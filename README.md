# goholesail

A peer-to-peer TCP tunnel over libp2p. Expose a local port through a self-hosted
hub without port forwarding; traffic goes P2P (relay fallback when hole-punching
fails). Not interoperable with JS holesail — see `docs/superpowers/specs/`.

## Quickstart

```bash
# 1. Hub (public VPS): relay + built-in service registry, one libp2p port.
#    Pass a stable --seed so the hub's peer id (and every --hub / ghs:// string
#    that embeds it) survives restarts; omit it for a throwaway ephemeral id.
goholesail hub --listen /ip4/0.0.0.0/tcp/4001 --seed <hub-seed>

# 2a. Host, public mode
goholesail host --live 22 --seed <seed> --hub <hub-/p2p-addr>
#     prints a ghs://p1_... connection string

# 2b. Host, private mode + published by name (secret is NOT sent to the registry)
goholesail host --live 22 --seed <seed> --hub <hub-/p2p-addr> \
  --private --secret <shared-secret> --name home-ssh --tags ssh

# 3a. Client, by pasted connection string
goholesail connect 'ghs://...' --port 2222

# 3b. Client, by name (resolve via the hub; supply the secret out-of-band for private services)
goholesail connect home-ssh --hub <hub-/p2p-addr> --secret <shared-secret> --port 2222
ssh -p 2222 user@127.0.0.1

# 4. Inspect a hub's directory
goholesail list --hub <hub-/p2p-addr> [--tag ssh]
```

## Status

M1+M2 (public TCP tunnel), M3 (private-mode HMAC auth + service registry with
`--name` resolution), and the single-port rework (registry served over the
libp2p `/goholesail/registry/1.0.0` stream protocol on the hub — one open port,
Noise-encrypted + peer-authenticated) are implemented. Resilience/fallback (M4)
and UDP (M5) are tracked in the specs.
