package main

import (
	"context"
	"fmt"

	"github.com/BenLocal/goholesail/internal/client"
	"github.com/spf13/cobra"
)

func newConnectCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "connect <connection-string>",
		Short: "Bind a local port tunneling to a remote host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			h, ln, err := client.Run(ctx, client.Options{ConnString: args[0], LocalPort: port})
			if err != nil {
				return err
			}
			defer h.Close()
			defer ln.Close()
			fmt.Printf("listening on %s\n", ln.Addr().String())
			waitForSignal()
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "local TCP port to bind (0 = random)")
	return cmd
}
