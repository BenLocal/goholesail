package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/BenLocal/goholesail/internal/identity"
	"github.com/BenLocal/goholesail/internal/registry"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var hubAddr string
	var tag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List services published on a hub's registry",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			hubInfo, err := peer.AddrInfoFromString(hubAddr)
			if err != nil {
				return fmt.Errorf("list: parse hub addr: %w", err)
			}
			priv, err := identity.Random()
			if err != nil {
				return err
			}
			h, err := libp2p.New(libp2p.Identity(priv))
			if err != nil {
				return fmt.Errorf("list: new: %w", err)
			}
			defer h.Close()
			if err := h.Connect(ctx, *hubInfo); err != nil {
				return fmt.Errorf("list: connect hub: %w", err)
			}
			svcs, err := registry.NewClient(h, hubInfo.ID).List(ctx, tag)
			if err != nil {
				return err
			}
			for _, s := range svcs {
				fmt.Printf("%s\t%s\tprivate=%v\ttags=%v\thub=%s\n", s.Name, s.PeerID, s.Private, s.Tags, s.Hub)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&hubAddr, "hub", "", "hub /p2p multiaddr (required)")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag")
	_ = cmd.MarkFlagRequired("hub")
	return cmd
}
