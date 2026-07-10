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

## Deploying a host

Run a long-lived `host` tunnel that exposes a service on the machine's loopback
(e.g. `sshd` on `127.0.0.1:22`) through a hub. Two options:

### Docker (Linux)

```bash
cp .env.host.example .env.host    # edit: at least LIVE and HUB
docker compose -f docker-compose.host.yaml up -d --build
docker compose -f docker-compose.host.yaml logs -f   # prints the ghs:// string
```

Uses `network_mode: host`, so `127.0.0.1:<LIVE>` inside the container is this
machine's loopback. All host flags come from `.env.host`
(`LIVE/HUB/SEED/PRIVATE/SECRET/NAME/TAGS`).

### Native (systemd or supervisord)

Install the released binary and register it as a service — you pick the manager:

```bash
sudo sh -c "$(curl -fsSL \
  https://raw.githubusercontent.com/BenLocal/goholesail/main/deploy/install.sh)" -- \
  --service-manager systemd \
  --hub /ip4/203.0.113.10/tcp/4001/p2p/12D3KooW... --live 22 \
  --private --secret s3cr3t --name home-ssh --tags ssh
```

- `--service-manager systemd|supervisor` chooses the service type.
- The binary is downloaded from the latest GitHub Release (or `--version vX.Y.Z`);
  use `--binary /path/to/goholesail` to install a local build instead.
- Config lives in `/etc/goholesail/host.env`; edit it and restart the service
  (`systemctl restart goholesail-host` or `supervisorctl restart goholesail-host`)
  to change flags.
- The shared secret is passed to `goholesail host` as a CLI flag, so it is
  visible to local users via the process list (`ps`); `host.env` itself is
  stored mode 0600.

Release binaries (Linux amd64/arm64) are published on every `v*` tag.

## Status

M1+M2 (public TCP tunnel), M3 (private-mode HMAC auth + service registry with
`--name` resolution), and the single-port rework (registry served over the
libp2p `/goholesail/registry/1.0.0` stream protocol on the hub — one open port,
Noise-encrypted + peer-authenticated) are implemented. Resilience (part of M4)
has landed too: the hub relay runs without the default 2 min / 128 KB
per-connection limit, the host renews its relay reservation and re-reserves
promptly when the hub link drops, and `connect` keeps its hub+host connections
warm, tolerates the host being down at startup, and holds a new connection
through a transient outage (bounded ~30 s re-dial). Remaining fallback work and
UDP (M5) are tracked in the specs.
