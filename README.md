# goholesail

A peer-to-peer TCP tunnel over libp2p. Expose a local port through a self-hosted
hub without port forwarding; traffic goes P2P (relay fallback when hole-punching
fails). Not interoperable with JS holesail — see `docs/superpowers/specs/`.

## Quickstart (M2: public-mode TCP)

```bash
# 1. Hub (public VPS)
goholesail hub --listen /ip4/0.0.0.0/tcp/4001

# 2. Host (behind NAT), expose local :22
goholesail host --live 22 --seed <your-seed> --hub <hub-/p2p-addr>
#   prints a ghs://p1_... connection string

# 3. Client (anywhere)
goholesail connect 'ghs://p1_...' --port 2222
ssh -p 2222 user@127.0.0.1
```

## Status

M1 (foundation) + M2 (working public TCP tunnel) implemented. Private-mode auth,
the ws service registry, and resilience/fallback are tracked in the M3/M4 plans.
