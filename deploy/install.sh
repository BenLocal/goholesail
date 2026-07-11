#!/bin/sh
# goholesail host installer.
#
# Installs the goholesail binary and registers the `host` role as a systemd
# service OR a supervisord program (you pick with --service-manager). It is
# self-contained (generates the wrapper, env-file, and unit/conf inline) so it
# can run via curl without a repo checkout:
#
#   sudo sh -c "$(curl -fsSL \
#     https://raw.githubusercontent.com/BenLocal/goholesail/main/deploy/install.sh)" -- \
#     --service-manager systemd --hub <addr> --live <port> [options]
set -eu

URL_BASE="https://github.com/BenLocal/goholesail/releases"

usage() {
	cat <<'USAGE'
Usage: install.sh --service-manager <systemd|supervisor> --hub <addr> --live <port> [options]

Required:
  --service-manager   systemd | supervisor
  --hub ADDR          hub /p2p multiaddr
  --live PORT         local TCP port to expose

Optional host flags:
  --seed S            stable identity seed
  --private           require a shared secret from clients
  --secret S          shared secret (with --private)
  --name N            registry name to publish under
  --tags T            comma-separated registry tags

Optional install flags:
  --version V         release tag to download (default: latest)
  --binary PATH       install this local binary instead of downloading
  --user NAME         system user to run as (default: goholesail)
  --root DIR          install prefix (default: /); non-/ renders files only
  --url-base URL      release base URL (default: GitHub releases)
USAGE
}

die() { echo "install.sh: $*" >&2; exit 1; }

fetch() {
	# fetch URL OUTFILE
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$2" "$1" || die "download failed: $1"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$2" "$1" || die "download failed: $1"
	else
		die "need curl or wget to download the binary"
	fi
}

# single-quote-escape $1 so the value is safe to source from host.env
sq() { printf "%s" "$1" | sed "s/'/'\\\\''/g"; }
write_kv() { printf "%s='%s'\n" "$1" "$(sq "$2")"; }

SVC="" HUB="" LIVE="" SEED="" PRIVATE="" SECRET="" NAME="" TAGS=""
VERSION="latest" BINARY="" USER_NAME="goholesail" ROOT="/"

while [ $# -gt 0 ]; do
	case "$1" in
		--service-manager) SVC="$2"; shift 2;;
		--hub) HUB="$2"; shift 2;;
		--live) LIVE="$2"; shift 2;;
		--seed) SEED="$2"; shift 2;;
		--private) PRIVATE=1; shift;;
		--secret) SECRET="$2"; shift 2;;
		--name) NAME="$2"; shift 2;;
		--tags) TAGS="$2"; shift 2;;
		--version) VERSION="$2"; shift 2;;
		--binary) BINARY="$2"; shift 2;;
		--user) USER_NAME="$2"; shift 2;;
		--root) ROOT="$2"; shift 2;;
		--url-base) URL_BASE="$2"; shift 2;;
		-h|--help) usage; exit 0;;
		*) die "unknown flag: $1 (try --help)";;
	esac
done

case "$SVC" in
	systemd|supervisor) ;;
	*) die "--service-manager must be 'systemd' or 'supervisor'";;
esac
[ -n "$HUB" ] || die "--hub is required"
[ -n "$LIVE" ] || die "--live is required"
if [ -n "$SECRET" ] && [ "$PRIVATE" != 1 ]; then
	echo "install.sh: warning: --secret without --private is ignored by the host" >&2
fi
[ -n "$ROOT" ] || die "--root must not be empty (use / for a real install)"
case "$USER_NAME" in
	*[!a-z0-9_-]*|'') die "--user must contain only [a-z0-9_-] (got '$USER_NAME')";;
esac

case "$ROOT" in
	/) RPFX="";;
	*) RPFX="${ROOT%/}";;
esac
BIN_DIR="$RPFX/usr/local/bin"
CFG_DIR="$RPFX/etc/goholesail"
mkdir -p "$BIN_DIR" "$CFG_DIR"

# ---- acquire the binary ----
need_download=1
if [ -n "$BINARY" ]; then
	echo "install.sh: using provided binary: $BINARY"
	install -m 0755 "$BINARY" "$BIN_DIR/goholesail"
	need_download=0
elif [ -x "$BIN_DIR/goholesail" ]; then
	echo "install.sh: goholesail already installed at $BIN_DIR/goholesail, skipping download"
	need_download=0
fi

if [ "$need_download" = 1 ]; then
	os="${GHS_OS:-$(uname -s | tr '[:upper:]' '[:lower:]')}"
	raw="${GHS_ARCH:-$(uname -m)}"
	case "$raw" in
		x86_64|amd64) arch=amd64;;
		aarch64|arm64) arch=arm64;;
		*) die "unsupported architecture: $raw";;
	esac
	asset="goholesail_${os}_${arch}.tar.gz"
	if [ "$VERSION" = latest ]; then
		base="$URL_BASE/latest/download"
	else
		base="$URL_BASE/download/$VERSION"
	fi
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT
	fetch "$base/$asset" "$tmp/$asset"
	fetch "$base/checksums.txt" "$tmp/checksums.txt"
	( cd "$tmp" && grep " $asset\$" checksums.txt | sha256sum -c - >/dev/null ) \
		|| die "checksum verification failed for $asset"
	tar -xzf "$tmp/$asset" -C "$tmp"
	[ -f "$tmp/goholesail" ] || die "archive $asset did not contain a goholesail binary"
	install -m 0755 "$tmp/goholesail" "$BIN_DIR/goholesail"
fi

# ---- install the shared wrapper (byte-identical to deploy/goholesail-host) ----
cat > "$BIN_DIR/goholesail-host" <<'WRAP'
#!/bin/sh
# goholesail host wrapper: build the `goholesail host` argv from config and exec it.
# Config comes from /etc/goholesail/host.env if present (native install), else the
# process environment (Docker env_file). LIVE and HUB are required.
set -eu
[ -f /etc/goholesail/host.env ] && . /etc/goholesail/host.env
: "${LIVE:?LIVE (local TCP port to expose) is required}"
: "${HUB:?HUB (hub /p2p multiaddr) is required}"
set -- host --live "$LIVE" --hub "$HUB"
[ -n "${SEED:-}" ] && set -- "$@" --seed "$SEED"
[ "${PRIVATE:-}" = 1 ] && set -- "$@" --private
[ -n "${SECRET:-}" ] && set -- "$@" --secret "$SECRET"
[ -n "${NAME:-}" ] && set -- "$@" --name "$NAME"
[ -n "${TAGS:-}" ] && set -- "$@" --tags "$TAGS"
exec goholesail "$@"
WRAP
chmod 0755 "$BIN_DIR/goholesail-host"

# ---- write the config env-file (0600; values single-quoted for safe sourcing) ----
(
	umask 077
	{
		write_kv LIVE "$LIVE"
		write_kv HUB "$HUB"
		write_kv SEED "$SEED"
		write_kv PRIVATE "$PRIVATE"
		write_kv SECRET "$SECRET"
		write_kv NAME "$NAME"
		write_kv TAGS "$TAGS"
	} > "$CFG_DIR/host.env"
)
chmod 0600 "$CFG_DIR/host.env"

# ---- render the service definition ----
case "$SVC" in
systemd)
	UNIT_DIR="$RPFX/etc/systemd/system"
	mkdir -p "$UNIT_DIR"
	cat > "$UNIT_DIR/goholesail-host.service" <<EOF
[Unit]
Description=goholesail host tunnel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$USER_NAME
ExecStart=/usr/local/bin/goholesail-host
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
	;;
supervisor)
	CONF_DIR="$RPFX/etc/supervisor/conf.d"
	mkdir -p "$CONF_DIR"
	cat > "$CONF_DIR/goholesail-host.conf" <<EOF
[program:goholesail-host]
command=/usr/local/bin/goholesail-host
user=$USER_NAME
autostart=true
autorestart=true
startsecs=5
startretries=3
stopasgroup=true
killasgroup=true
stdout_logfile=/var/log/goholesail-host.log
stderr_logfile=/var/log/goholesail-host.err.log
EOF
	;;
esac

# ---- create user + activate (real install only) ----
if [ "$ROOT" = "/" ]; then
	if ! id "$USER_NAME" >/dev/null 2>&1; then
		useradd --system --no-create-home --shell /usr/sbin/nologin "$USER_NAME" \
			|| die "failed to create user $USER_NAME"
	fi
	chown "$USER_NAME" "$CFG_DIR/host.env" || die "failed to chown host.env to $USER_NAME"
	case "$SVC" in
	systemd)
		systemctl daemon-reload
		systemctl enable goholesail-host
		systemctl restart --no-block goholesail-host
		;;
	supervisor)
		supervisorctl reread
		supervisorctl update
		;;
	esac
	echo "install.sh: goholesail-host installed and started via $SVC"
else
	echo "install.sh: rendered goholesail-host files under $ROOT (user/activation skipped)"
fi
