package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrSessionNotFound is returned when a session id is unknown.
var ErrSessionNotFound = errors.New("session not found")

// Schema creates the sessions, actions, and events tables exactly as
// specified. Times are stored as RFC3339Nano text; nullable columns
// (ended_at, exit_code, undone_at) are NULL until set.
const schema = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	agent TEXT NOT NULL DEFAULT '',
	command TEXT NOT NULL DEFAULT '',
	cwd TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL,
	ended_at TEXT,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	exit_code INTEGER,
	start_ref TEXT NOT NULL DEFAULT '',
	end_ref TEXT NOT NULL DEFAULT '',
	files_changed INTEGER NOT NULL DEFAULT 0,
	undone_at TEXT,
	lines_added INTEGER,
	lines_deleted INTEGER,
	input_tokens INTEGER,
	output_tokens INTEGER,
	cost_usd REAL
);
CREATE TABLE IF NOT EXISTS actions (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	type TEXT NOT NULL DEFAULT '',
	path TEXT NOT NULL DEFAULT '',
	diff TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	FOREIGN KEY (session_id) REFERENCES sessions(id)
);
CREATE INDEX IF NOT EXISTS idx_actions_session ON actions(session_id);
CREATE TABLE IF NOT EXISTS events (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	stream TEXT NOT NULL DEFAULT '',
	line TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	FOREIGN KEY (session_id) REFERENCES sessions(id)
);
CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id);
`

// Session is one wrapped agent run. LinesAdded, LinesDeleted,
// InputTokens, OutputTokens, and CostUSD are nil for sessions
// recorded before v0.2 or by agents that report no metrics: nil
// means "not reported", never zero.
type Session struct {
	ID           string
	Agent        string
	Command      string
	Cwd          string
	StartedAt    time.Time
	EndedAt      *time.Time
	DurationMs   int64
	ExitCode     *int
	StartRef     string
	EndRef       string
	FilesChanged int
	UndoneAt     *time.Time
	LinesAdded   *int
	LinesDeleted *int
	InputTokens  *int64
	OutputTokens *int64
	CostUSD      *float64
}

// Action is one file change recorded for a session.
type Action struct {
	ID        string
	SessionID string
	Type      string
	Path      string
	Diff      string
	CreatedAt time.Time
}

// Event is one captured output line for a session.
type Event struct {
	ID        string
	SessionID string
	Stream    string // "stdout" or "stderr"
	Line      string
	CreatedAt time.Time
}

// Store is the SQLite store. It is the ONLY place that calls SQL.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and
// applies the schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema to %s: %w (check that the file is a valid agentledger database and writable)", path, err)
	}
	// Migrations for databases created before v0.2: add the
	// telemetry columns. "duplicate column name" means the column
	// is already there and is not an error; any other error is.
	for _, alter := range []string{
		`ALTER TABLE sessions ADD COLUMN lines_added INTEGER`,
		`ALTER TABLE sessions ADD COLUMN lines_deleted INTEGER`,
		`ALTER TABLE sessions ADD COLUMN input_tokens INTEGER`,
		`ALTER TABLE sessions ADD COLUMN output_tokens INTEGER`,
		`ALTER TABLE sessions ADD COLUMN cost_usd REAL`,
	} {
		if _, err := db.Exec(alter); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			db.Close()
			return nil, fmt.Errorf("migrate database %s: %w", path, err)
		}
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

func nullableTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := parseTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func nullableInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

func nullableInt64(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}

func nullableFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

// CreateSession inserts a new session row.
func (s *Store) CreateSession(sess *Session) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, agent, command, cwd, started_at, ended_at, duration_ms, exit_code, start_ref, end_ref, files_changed, undone_at, lines_added, lines_deleted, input_tokens, output_tokens, cost_usd)
		 VALUES (?, ?, ?, ?, ?, NULL, ?, NULL, ?, ?, ?, NULL, ?, ?, ?, ?, ?)`,
		sess.ID, sess.Agent, sess.Command, sess.Cwd, formatTime(sess.StartedAt),
		sess.DurationMs, sess.StartRef, sess.EndRef, sess.FilesChanged,
		sess.LinesAdded, sess.LinesDeleted, sess.InputTokens, sess.OutputTokens, sess.CostUSD)
	if err != nil {
		return fmt.Errorf("create session %s: %w", sess.ID, err)
	}
	return nil
}

func scanSession(row interface {
	Scan(...any) error
}) (*Session, error) {
	var sess Session
	var started, ended, undone sql.NullString
	var exitCode, linesAdded, linesDeleted, inputTokens, outputTokens sql.NullInt64
	var costUSD sql.NullFloat64
	if err := row.Scan(&sess.ID, &sess.Agent, &sess.Command, &sess.Cwd, &started,
		&ended, &sess.DurationMs, &exitCode, &sess.StartRef, &sess.EndRef,
		&sess.FilesChanged, &undone, &linesAdded, &linesDeleted, &inputTokens, &outputTokens, &costUSD); err != nil {
		return nil, err
	}
	t, err := parseTime(started.String)
	if err != nil {
		return nil, err
	}
	sess.StartedAt = t
	if sess.EndedAt, err = nullableTime(ended); err != nil {
		return nil, err
	}
	sess.ExitCode = nullableInt(exitCode)
	if sess.UndoneAt, err = nullableTime(undone); err != nil {
		return nil, err
	}
	sess.LinesAdded = nullableInt(linesAdded)
	sess.LinesDeleted = nullableInt(linesDeleted)
	sess.InputTokens = nullableInt64(inputTokens)
	sess.OutputTokens = nullableInt64(outputTokens)
	sess.CostUSD = nullableFloat(costUSD)
	return &sess, nil
}

// GetSession returns one session by id.
func (s *Store) GetSession(id string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT id, agent, command, cwd, started_at, ended_at, duration_ms, exit_code, start_ref, end_ref, files_changed, undone_at, lines_added, lines_deleted, input_tokens, output_tokens, cost_usd
		 FROM sessions WHERE id = ?`, id)
	sess, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get session %s: %w", id, err)
	}
	return sess, nil
}

// ListSessions returns sessions newest first. A limit <= 0 means no limit.
func (s *Store) ListSessions(limit int) ([]*Session, error) {
	query := `SELECT id, agent, command, cwd, started_at, ended_at, duration_ms, exit_code, start_ref, end_ref, files_changed, undone_at, lines_added, lines_deleted, input_tokens, output_tokens, cost_usd
		FROM sessions ORDER BY started_at DESC, id DESC`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.Query(query+" LIMIT ?", limit)
	} else {
		rows, err = s.db.Query(query)
	}
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var out []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// FinishSession fills in a session's end fields after a run completes.
// Telemetry is nil: use FinishSessionWithMetrics when line counts or
// token/cost figures are known.
func (s *Store) FinishSession(id string, endedAt time.Time, durationMs int64, exitCode int, endRef string, filesChanged int) error {
	return s.FinishSessionWithMetrics(id, endedAt, durationMs, exitCode, endRef, filesChanged, nil, nil, nil, nil, nil)
}

// FinishSessionWithMetrics is FinishSession plus the v0.2 telemetry
// fields. Nil metric pointers are stored as NULL (not reported).
func (s *Store) FinishSessionWithMetrics(id string, endedAt time.Time, durationMs int64, exitCode int, endRef string, filesChanged int, linesAdded, linesDeleted *int, inputTokens, outputTokens *int64, costUSD *float64) error {
	res, err := s.db.Exec(
		`UPDATE sessions SET ended_at = ?, duration_ms = ?, exit_code = ?, end_ref = ?, files_changed = ?, lines_added = ?, lines_deleted = ?, input_tokens = ?, output_tokens = ?, cost_usd = ? WHERE id = ?`,
		formatTime(endedAt), durationMs, exitCode, endRef, filesChanged, linesAdded, linesDeleted, inputTokens, outputTokens, costUSD, id)
	if err != nil {
		return fmt.Errorf("finish session %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, id)
	}
	return nil
}

// MarkUndone records that a session's changes were undone.
func (s *Store) MarkUndone(id string, undoneAt time.Time) error {
	res, err := s.db.Exec(`UPDATE sessions SET undone_at = ? WHERE id = ?`, formatTime(undoneAt), id)
	if err != nil {
		return fmt.Errorf("mark session %s undone: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, id)
	}
	return nil
}

// DeleteSession removes a session and its actions and events, in
// one transaction so a failure cannot leave orphaned rows. It
// returns ErrSessionNotFound when the session id is unknown.
func (s *Store) DeleteSession(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete session %s: %w", id, err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM events WHERE session_id = ?`, id); err != nil {
		return fmt.Errorf("delete events for session %s: %w", id, err)
	}
	if _, err := tx.Exec(`DELETE FROM actions WHERE session_id = ?`, id); err != nil {
		return fmt.Errorf("delete actions for session %s: %w", id, err)
	}
	res, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete session %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, id)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete session %s: %w", id, err)
	}
	return nil
}

// SessionStoredBytes estimates the bytes a session occupies in the
// database: the sum of its action diffs and event lines. It is an
// estimate (index and page overhead are excluded), used by clean to
// report what pruning removes.
func (s *Store) SessionStoredBytes(id string) (int64, error) {
	var total int64
	err := s.db.QueryRow(
		`SELECT COALESCE((SELECT SUM(LENGTH(diff)) FROM actions WHERE session_id = ?), 0)
		 + COALESCE((SELECT SUM(LENGTH(line)) FROM events WHERE session_id = ?), 0)`,
		id, id).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("stored bytes for session %s: %w", id, err)
	}
	return total, nil
}

// Vacuum rewrites the database file, returning freed pages to the
// operating system so clean's reclaimed-space figure is real.
func (s *Store) Vacuum() error {
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("vacuum database: %w", err)
	}
	return nil
}

// CreateAction inserts one action row.
func (s *Store) CreateAction(a *Action) error {
	_, err := s.db.Exec(
		`INSERT INTO actions (id, session_id, type, path, diff, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.SessionID, a.Type, a.Path, a.Diff, formatTime(a.CreatedAt))
	if err != nil {
		return fmt.Errorf("create action %s: %w", a.ID, err)
	}
	return nil
}

// ListActions returns a session's actions in insertion order.
func (s *Store) ListActions(sessionID string) ([]*Action, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, type, path, diff, created_at FROM actions WHERE session_id = ? ORDER BY rowid`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list actions for %s: %w", sessionID, err)
	}
	defer rows.Close()
	var out []*Action
	for rows.Next() {
		var a Action
		var created string
		if err := rows.Scan(&a.ID, &a.SessionID, &a.Type, &a.Path, &a.Diff, &created); err != nil {
			return nil, err
		}
		if a.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// CreateEvent inserts one output-line row.
func (s *Store) CreateEvent(e *Event) error {
	_, err := s.db.Exec(
		`INSERT INTO events (id, session_id, stream, line, created_at) VALUES (?, ?, ?, ?, ?)`,
		e.ID, e.SessionID, e.Stream, e.Line, formatTime(e.CreatedAt))
	if err != nil {
		return fmt.Errorf("create event %s: %w", e.ID, err)
	}
	return nil
}

// ListEvents returns a session's events in insertion order.
func (s *Store) ListEvents(sessionID string) ([]*Event, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, stream, line, created_at FROM events WHERE session_id = ? ORDER BY rowid`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list events for %s: %w", sessionID, err)
	}
	defer rows.Close()
	var out []*Event
	for rows.Next() {
		var e Event
		var created string
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Stream, &e.Line, &created); err != nil {
			return nil, err
		}
		if e.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}
