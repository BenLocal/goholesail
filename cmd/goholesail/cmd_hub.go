package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/BenLocal/goholesail/internal/hub"
	"github.com/BenLocal/goholesail/internal/registry"
	"github.com/spf13/cobra"
)

func newHubCmd() *cobra.Command {
	var listen string
	var registryListen string
	var seed string
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Run the relay/rendezvous hub",
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := hub.New(listen, seed)
			if err != nil {
				return err
			}
			defer h.Close()
			hub.AttachConnLogger(h, log.New(os.Stderr, "[hub] ", log.LstdFlags))
			fmt.Println("hub listening; dial addresses:")
			for _, a := range hub.P2pAddrs(h) {
				fmt.Println("  " + a)
			}
			if registryListen != "" {
				regLog := log.New(os.Stderr, "[registry] ", log.LstdFlags)
				srv := &http.Server{Addr: registryListen, Handler: registry.NewServerWithLogger(registry.NewStore(), regLog)}
				go func() {
					if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						fmt.Fprintln(cmd.ErrOrStderr(), "registry server:", err)
					}
				}()
				defer srv.Close()
				fmt.Println("registry ws on " + registryListen + " (path /reg, GET /services)")
			}
			waitForSignal()
			return nil
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "/ip4/0.0.0.0/tcp/4001", "libp2p listen multiaddr")
	cmd.Flags().StringVar(&registryListen, "registry-listen", "", "registry HTTP listen addr, e.g. :8080 (empty = relay only)")
	cmd.Flags().StringVar(&seed, "seed", "", "stable identity seed (empty = ephemeral; set it to keep --hub stable across restarts)")
	return cmd
}

func waitForSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
