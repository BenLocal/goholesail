package main

import (
	"context"
	"fmt"

	"github.com/BenLocal/goholesail/internal/host"
	"github.com/spf13/cobra"
)

func newHostCmd() *cobra.Command {
	var (
		live int
		seed string
		hub  string
	)
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Expose a local TCP port",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if live < 1 || live > 65535 {
				return fmt.Errorf("--live must be a valid TCP port (1-65535), got %d", live)
			}
			ctx := context.Background()
			h, cs, err := host.Run(ctx, host.Options{Seed: seed, LocalPort: live, HubAddr: hub})
			if err != nil {
				return err
			}
			defer h.Close()
			fmt.Println("connection string:")
			fmt.Println("  " + cs)
			waitForSignal()
			return nil
		},
	}
	cmd.Flags().IntVar(&live, "live", 0, "local TCP port to expose (required)")
	cmd.Flags().StringVar(&seed, "seed", "", "stable identity seed (empty = ephemeral)")
	cmd.Flags().StringVar(&hub, "hub", "", "hub /p2p multiaddr (required)")
	_ = cmd.MarkFlagRequired("live")
	_ = cmd.MarkFlagRequired("hub")
	return cmd
}
