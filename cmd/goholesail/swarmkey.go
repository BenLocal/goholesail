package main

import "os"

// resolveSwarmKey returns the --swarm-key flag value, falling back to the
// SWARM_KEY environment variable (handy for Docker/compose). Empty means no
// private network. All of hub/host/client must resolve to the same value.
func resolveSwarmKey(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv("SWARM_KEY")
}
