package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/BenLocal/goholesail/internal/hub"
	"github.com/spf13/cobra"
)

func newHubCmd() *cobra.Command {
	var listen string
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Run the relay/rendezvous hub",
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := hub.New(listen)
			if err != nil {
				return err
			}
			defer h.Close()
			fmt.Println("hub listening; dial addresses:")
			for _, a := range hub.P2pAddrs(h) {
				fmt.Println("  " + a)
			}
			waitForSignal()
			return nil
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "/ip4/0.0.0.0/tcp/4001", "libp2p listen multiaddr")
	return cmd
}

func waitForSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
