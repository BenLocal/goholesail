package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/BenLocal/goholesail/internal/hub"
	"github.com/BenLocal/goholesail/internal/registry"
	"github.com/spf13/cobra"
)

func newHubCmd() *cobra.Command {
	var listen string
	var seed string
	var announce string
	var swarmKey string
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Run the relay/rendezvous hub",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// --announce flag takes precedence; fall back to HUB_ANNOUNCE_ADDR env.
			if announce == "" {
				announce = os.Getenv("HUB_ANNOUNCE_ADDR")
			}
			h, err := hub.New(listen, seed, announce, resolveSwarmKey(swarmKey))
			if err != nil {
				return err
			}
			defer h.Close()
			hub.AttachConnLogger(h, log.New(os.Stderr, "[hub] ", log.LstdFlags))
			// Registry is always on: it is just a stream protocol on the hub
			// host now (no extra port).
			srv := registry.NewServerWithLogger(registry.NewStore(), log.New(os.Stderr, "[registry] ", log.LstdFlags))
			h.SetStreamHandler(registry.RegistryProtocolID, srv.HandleStream)
			fmt.Println("hub listening; dial addresses:")
			for _, a := range hub.P2pAddrs(h) {
				fmt.Println("  " + a)
			}
			fmt.Println("registry: on (protocol " + string(registry.RegistryProtocolID) + ")")
			waitForSignal()
			return nil
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "/ip4/0.0.0.0/tcp/4001", "libp2p listen multiaddr")
	cmd.Flags().StringVar(&seed, "seed", "", "stable identity seed (empty = ephemeral; set it to keep --hub stable across restarts)")
	cmd.Flags().StringVar(&announce, "announce", "", "optional public multiaddr to announce (e.g. /ip4/203.0.113.10/tcp/4001); also settable via HUB_ANNOUNCE_ADDR env var. Needed when the hub runs behind NAT/Docker and cannot see its public IP")
	cmd.Flags().StringVar(&swarmKey, "swarm-key", "", "shared swarm passphrase for a private network (pnet); hub/host/client must all use the same value; also settable via SWARM_KEY env")
	return cmd
}

func waitForSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
