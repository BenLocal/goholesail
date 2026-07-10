# goholesail

A peer-to-peer TCP tunnel over libp2p. Expose a local port through a self-hosted
hub without port forwarding; traffic goes P2P (relay fallback when hole-punching
fails). Not interoperable with JS holesail — see `docs/superpowers/specs/`.

## Quickstart

```bash
# 1. Hub (public VPS): relay + optional ws service registry
goholesail hub --listen /ip4/0.0.0.0/tcp/4001 --registry-listen :8080

# 2a. Host, public mode
goholesail host --live 22 --seed <seed> --hub <hub-/p2p-addr>
#     prints a ghs://p1_... connection string

# 2b. Host, private mode + published by name (secret is NOT sent to the registry)
goholesail host --live 22 --seed <seed> --hub <hub-/p2p-addr> \
  --private --secret <shared-secret> --name home-ssh --registry ws://<hub>:8080/reg

# 3a. Client, by pasted connection string
goholesail connect 'ghs://...' --port 2222

# 3b. Client, by name (supply the secret out-of-band for private services)
goholesail connect home-ssh --registry ws://<hub>:8080/reg --secret <shared-secret> --port 2222
ssh -p 2222 user@127.0.0.1
```

## Status

M1+M2 (public TCP tunnel) and M3 (private-mode HMAC auth + ws service registry
with `--name` resolution) implemented. Resilience/fallback (M4) and UDP (M5) are
tracked in the spec. Registry `subscribe`/live hot-switching is deferred.
