package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "goholesail",
		Short: "Peer-to-peer TCP tunnel over libp2p",
		// Runtime errors are printed once by main(); don't also dump the usage
		// block, which would make network/runtime failures look like flag typos.
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newHubCmd(), newHostCmd(), newConnectCmd())
	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
