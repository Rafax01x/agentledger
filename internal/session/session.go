package session

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"agentledger/internal/config"
	"agentledger/internal/gitx"
	"agentledger/internal/redact"
	"agentledger/internal/store"
)

// captureWriter forwards every write to dst immediately (so output
// streams live) and also accumulates it for later event recording.
// Writes are serialized through a mutex shared by a run's stdout and
// stderr writers: exec runs those streams on separate goroutines, and
// the two destinations may be the same writer (for example when a
// caller redirects both streams to one buffer), so unsynchronized
// writes are a data race.
type captureWriter struct {
	dst io.Writer
	buf bytes.Buffer
	mu  *sync.Mutex
}

func (w *captureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.dst.Write(p)
	if n > 0 {
		w.buf.Write(p[:n])
	}
	return n, err
}

func (w *captureWriter) lines() []string {
	s := strings.TrimSuffix(w.buf.String(), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// Config describes one wrapped run.
type Config struct {
	Root    string    // git repository root (snapshots happen here)
	Cwd     string    // working directory the child runs in
	Agent   string    // agent name recorded for the session
	Command []string  // child command, argv[0] first; must be non-empty
	Env     []string  // extra environment for the child (nil = inherit)
	Stdout  io.Writer // live stdout destination
	Stderr  io.Writer // live stderr destination
	Store   *store.Store
}

// Result is the recorded outcome of a run.
type Result struct {
	Session  *store.Session
	ExitCode int
}

// refFor builds refs/agentledger/<id>/<name>.
func refFor(id, name string) string {
	return "refs/agentledger/" + id + "/" + name
}

// Run snapshots the working tree, runs the child command with its
// output streamed live and captured, snapshots again, and records
// the session, its file changes (actions), and its output lines
// (events) in the store. It never swallows the child's exit code:
// the code is in Result.ExitCode and in the stored session.
//
// Every string is redacted before it is persisted (session fields,
// action diffs, event lines). Live output streamed to the terminal
// is NOT redacted: it is the user's own terminal, and redacting it
// would hide what their agent actually did from them.
//
// A child that cannot be started at all is recorded with exit code
// 127 and its start error written to the stderr stream, so the user
// still sees why it failed.
func Run(cfg Config) (*Result, error) {
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("run requires a command")
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("run requires a store (run agentledger init first)")
	}
	id := ulid.Make().String()
	started := time.Now()

	// Project configuration is loaded before anything is recorded:
	// a malformed config fails the run rather than silently
	// recording unredacted output, and ignore patterns apply to
	// both snapshots and the recorded diffs.
	cfgFile, err := config.Load(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("load project config: %w", err)
	}
	red, err := redact.NewFromConfig(cfgFile, cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("build redactor: %w", err)
	}

	startCommit, err := gitx.SnapshotWithIgnores(cfg.Root, id, "start", cfgFile.IgnorePaths)
	if err != nil {
		return nil, fmt.Errorf("start snapshot failed: %w", err)
	}
	_ = startCommit
	sess := &store.Session{
		ID:        id,
		Agent:     red.Redact(cfg.Agent),
		Command:   red.Redact(strings.Join(cfg.Command, " ")),
		Cwd:       red.Redact(cfg.Cwd),
		StartedAt: started,
		StartRef:  refFor(id, "start"),
	}
	if err := cfg.Store.CreateSession(sess); err != nil {
		return nil, err
	}

	var writeMu sync.Mutex
	stdoutCap := &captureWriter{dst: cfg.Stdout, mu: &writeMu}
	stderrCap := &captureWriter{dst: cfg.Stderr, mu: &writeMu}
	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Dir = cfg.Cwd
	cmd.Env = cfg.Env
	cmd.Stdout = stdoutCap
	cmd.Stderr = stderrCap
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Cannot start (binary missing, permission, ...).
			fmt.Fprintln(stderrCap, runErr.Error())
			exitCode = 127
		}
	}

	ended := time.Now()
	endCommit, err := gitx.SnapshotWithIgnores(cfg.Root, id, "end", cfgFile.IgnorePaths)
	if err != nil {
		return nil, fmt.Errorf("end snapshot failed: %w", err)
	}
	_ = endCommit

	changes, err := gitx.ChangedFilesStatus(cfg.Root, sess.StartRef, refFor(id, "end"))
	if err != nil {
		return nil, fmt.Errorf("compute file changes: %w", err)
	}
	// Ignore patterns are enforced at the recorded-action level even
	// when a git pathspec could not exclude a file from the snapshot
	// tree, so ignored files never appear in diffs or undo.
	var kept []gitx.FileChange
	for _, ch := range changes {
		if !cfgFile.MatchIgnore(ch.Path) {
			kept = append(kept, ch)
		}
	}
	changes = kept
	var linesAdded, linesDeleted int
	for _, ch := range changes {
		diff, err := gitx.DiffFile(cfg.Root, sess.StartRef, refFor(id, "end"), ch.Path)
		if err != nil {
			return nil, fmt.Errorf("diff for %s: %w", ch.Path, err)
		}
		// Count on the raw diff: redaction can collapse a
		// multi-line private-key block into one placeholder line.
		added, deleted := gitx.DiffStat(diff)
		linesAdded += added
		linesDeleted += deleted
		if err := cfg.Store.CreateAction(&store.Action{
			ID:        ulid.Make().String(),
			SessionID: id,
			Type:      ch.Type,
			Path:      ch.Path,
			Diff:      red.Redact(diff),
			CreatedAt: ended,
		}); err != nil {
			return nil, err
		}
	}
	for _, line := range stdoutCap.lines() {
		if err := cfg.Store.CreateEvent(&store.Event{
			ID: ulid.Make().String(), SessionID: id,
			Stream: "stdout", Line: red.Redact(line), CreatedAt: ended,
		}); err != nil {
			return nil, err
		}
	}
	for _, line := range stderrCap.lines() {
		if err := cfg.Store.CreateEvent(&store.Event{
			ID: ulid.Make().String(), SessionID: id,
			Stream: "stderr", Line: red.Redact(line), CreatedAt: ended,
		}); err != nil {
			return nil, err
		}
	}

	durationMs := ended.Sub(started).Milliseconds()
	// Token/cost metrics are parsed from the raw output lines: the
	// figures are not secrets, and parsing after redaction would
	// risk a redacted placeholder hiding a metric line.
	metrics := ParseMetrics(append(append([]string{}, stdoutCap.lines()...), stderrCap.lines()...))
	if err := cfg.Store.FinishSessionWithMetrics(id, ended, durationMs, exitCode, refFor(id, "end"), len(changes), &linesAdded, &linesDeleted, metrics.InputTokens, metrics.OutputTokens, metrics.CostUSD); err != nil {
		return nil, err
	}
	finished, err := cfg.Store.GetSession(id)
	if err != nil {
		return nil, err
	}
	return &Result{Session: finished, ExitCode: exitCode}, nil
}
