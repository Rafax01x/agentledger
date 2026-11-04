package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"agentledger/internal/ui"
)

var uiPort int

var uiCmd = &cobra.Command{
	Use:   UiUse,
	Short: UiShort,
	Long:  UiLong,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, st, err := openStoreForRepo()
		if err != nil {
			return err
		}
		defer st.Close()
		srv := &ui.Server{
			Root:  root,
			Store: st,
			Undo: func(sessionID string, force bool, files []string) (int, error) {
				sess, err := resolveSession(st, sessionID)
				if err != nil {
					return 0, err
				}
				return runUndo(root, st, sess, files, false, force, io.Discard)
			},
		}
		// Loopback only: the inspector is a local view over the
		// repository's own database, never a network service.
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", uiPort))
		if err != nil {
			return fmt.Errorf("listen on port %d: %w", uiPort, err)
		}
		if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
			srv.Port = tcpAddr.Port
		}
		httpSrv := &http.Server{Handler: srv.Handler()}
		go func() {
			_ = httpSrv.Serve(ln)
		}()
		fmt.Fprintf(cmd.OutOrStdout(), UiServing+"\n", ln.Addr().String())
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(ctx)
	},
}

func init() {
	uiCmd.Flags().IntVar(&uiPort, "port", 4848, UiPortHelp)
}
