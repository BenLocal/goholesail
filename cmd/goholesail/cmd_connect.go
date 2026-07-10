package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/BenLocal/goholesail/internal/client"
	"github.com/spf13/cobra"
)

func newConnectCmd() *cobra.Command {
	var (
		port    int
		hubAddr string
		secret  string
	)
	cmd := &cobra.Command{
		Use:   "connect <connection-string|name>",
		Short: "Bind a local port tunneling to a remote host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			h, ln, err := client.Run(ctx, client.Options{
				ConnString: args[0], LocalPort: port,
				Hub: hubAddr, Secret: secret,
			})
			if err != nil {
				return err
			}
			defer h.Close()
			defer ln.Close()
			fmt.Printf("listening on %s\n", ln.Addr().String())
			<-ctx.Done()
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "local TCP port to bind (0 = random)")
	cmd.Flags().StringVar(&hubAddr, "hub", "", "hub /p2p multiaddr (required when passing a name)")
	cmd.Flags().StringVar(&secret, "secret", "", "shared secret for a private host")
	return cmd
}
