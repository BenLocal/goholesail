package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/BenLocal/goholesail/internal/host"
	"github.com/spf13/cobra"
)

func newHostCmd() *cobra.Command {
	var (
		live    int
		seed    string
		hubAddr string
		private bool
		secret  string
		name    string
		tags    []string
	)
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Expose a local TCP port",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if live < 1 || live > 65535 {
				return fmt.Errorf("--live must be a valid TCP port (1-65535), got %d", live)
			}
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			h, cs, err := host.Run(ctx, host.Options{
				Seed: seed, LocalPort: live, HubAddr: hubAddr,
				Private: private, Secret: secret,
				Name: name, Tags: tags,
			})
			if err != nil {
				return err
			}
			defer h.Close()
			fmt.Println("connection string:")
			fmt.Println("  " + cs)
			<-ctx.Done()
			return nil
		},
	}
	cmd.Flags().IntVar(&live, "live", 0, "local TCP port to expose (required)")
	cmd.Flags().StringVar(&seed, "seed", "", "stable identity seed (empty = ephemeral)")
	cmd.Flags().StringVar(&hubAddr, "hub", "", "hub /p2p multiaddr (required)")
	cmd.Flags().BoolVar(&private, "private", false, "require a shared secret from clients")
	cmd.Flags().StringVar(&secret, "secret", "", "shared secret (with --private; empty => generated)")
	cmd.Flags().StringVar(&name, "name", "", "registry name to publish under")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "registry tags (comma-separated)")
	_ = cmd.MarkFlagRequired("live")
	_ = cmd.MarkFlagRequired("hub")
	return cmd
}
