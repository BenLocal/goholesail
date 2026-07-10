package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	installScript = "../../deploy/install.sh"
	wrapperFile   = "../../deploy/goholesail-host"
)

// fakeGoholesail writes a `goholesail` shim into dir/bin that prints each of its
// args on its own line, and returns the bin dir to prepend to PATH.
func fakeGoholesail(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	shim := "#!/bin/sh\nprintf '%s\\n' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "goholesail"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// (strings and the installer helpers are imported/added in Task 2.)

func TestWrapperBuildsArgv(t *testing.T) {
	// The wrapper sources /etc/goholesail/host.env if it exists; skip if a real
	// one is present so the test stays hermetic (uses env vars only).
	if _, err := os.Stat("/etc/goholesail/host.env"); err == nil {
		t.Skip("/etc/goholesail/host.env exists; skipping to stay hermetic")
	}
	bin := fakeGoholesail(t, t.TempDir())

	cases := []struct {
		name string
		env  []string
		want string
	}{
		{
			name: "minimal",
			env:  []string{"LIVE=22", "HUB=h"},
			want: "host\n--live\n22\n--hub\nh\n",
		},
		{
			name: "full",
			env:  []string{"LIVE=22", "HUB=h", "SEED=s", "PRIVATE=1", "SECRET=x", "NAME=n", "TAGS=ssh"},
			want: "host\n--live\n22\n--hub\nh\n--seed\ns\n--private\n--secret\nx\n--name\nn\n--tags\nssh\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("sh", wrapperFile)
			cmd.Env = append([]string{"PATH=" + bin + ":" + os.Getenv("PATH")}, tc.env...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("wrapper failed: %v\n%s", err, out)
			}
			if string(out) != tc.want {
				t.Fatalf("argv mismatch:\n got %q\nwant %q", out, tc.want)
			}
		})
	}
}

// runInstall runs deploy/install.sh with args and returns combined output + error.
func runInstall(t *testing.T, extraEnv []string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{installScript}, args...)...)
	cmd.Env = append(os.Environ(), extraEnv...)
	return cmd.CombinedOutput()
}

// writeFakeBinary writes a dummy goholesail binary and returns its path.
func writeFakeBinary(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "fake-goholesail")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho fake\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func mustPerm(t *testing.T, p string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat %s: %v", p, err)
	}
	if fi.Mode().Perm() != want {
		t.Fatalf("%s perm = %o, want %o", p, fi.Mode().Perm(), want)
	}
}

func TestInstallSystemd(t *testing.T) {
	root := t.TempDir()
	bin := writeFakeBinary(t, t.TempDir())
	out, err := runInstall(t, nil,
		"--root", root, "--binary", bin, "--service-manager", "systemd",
		"--hub", "H", "--live", "22", "--private", "--secret", "x", "--name", "n", "--tags", "ssh")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	goBin := filepath.Join(root, "usr/local/bin/goholesail")
	mustPerm(t, goBin, 0o755)
	wrap := filepath.Join(root, "usr/local/bin/goholesail-host")
	mustPerm(t, wrap, 0o755)

	env := filepath.Join(root, "etc/goholesail/host.env")
	mustPerm(t, env, 0o600)
	for _, want := range []string{"LIVE='22'", "HUB='H'", "PRIVATE='1'", "SECRET='x'", "NAME='n'", "TAGS='ssh'"} {
		if !strings.Contains(readFile(t, env), want) {
			t.Fatalf("host.env missing %q:\n%s", want, readFile(t, env))
		}
	}

	unit := readFile(t, filepath.Join(root, "etc/systemd/system/goholesail-host.service"))
	for _, want := range []string{"ExecStart=/usr/local/bin/goholesail-host", "Restart=always", "User=goholesail"} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit missing %q:\n%s", want, unit)
		}
	}
}

func TestInstallSupervisor(t *testing.T) {
	root := t.TempDir()
	bin := writeFakeBinary(t, t.TempDir())
	out, err := runInstall(t, nil,
		"--root", root, "--binary", bin, "--service-manager", "supervisor",
		"--hub", "H", "--live", "22")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	conf := readFile(t, filepath.Join(root, "etc/supervisor/conf.d/goholesail-host.conf"))
	for _, want := range []string{"command=/usr/local/bin/goholesail-host", "autorestart=true", "user=goholesail"} {
		if !strings.Contains(conf, want) {
			t.Fatalf("conf missing %q:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "environment=") {
		t.Fatalf("supervisor conf must not use inline environment=:\n%s", conf)
	}
}

func TestInstallValidation(t *testing.T) {
	root := t.TempDir()
	bin := writeFakeBinary(t, t.TempDir())
	cases := [][]string{
		{"--root", root, "--binary", bin, "--hub", "H", "--live", "22"},                                                       // missing --service-manager
		{"--root", root, "--binary", bin, "--service-manager", "bogus", "--hub", "H", "--live", "22"},                         // bad manager
		{"--root", root, "--binary", bin, "--service-manager", "systemd", "--live", "22"},                                     // missing --hub
		{"--root", root, "--binary", bin, "--service-manager", "systemd", "--hub", "H"},                                       // missing --live
		{"--root", "", "--binary", bin, "--service-manager", "systemd", "--hub", "H", "--live", "22"},                         // empty --root must be rejected
		{"--root", root, "--binary", bin, "--service-manager", "systemd", "--hub", "H", "--live", "22", "--user", "bad user"}, // invalid --user (space) must be rejected
	}
	for i, args := range cases {
		if _, err := runInstall(t, nil, args...); err == nil {
			t.Fatalf("case %d: expected non-zero exit, got success", i)
		}
	}
}

func TestWrapperDriftGuard(t *testing.T) {
	root := t.TempDir()
	bin := writeFakeBinary(t, t.TempDir())
	if _, err := runInstall(t, nil,
		"--root", root, "--binary", bin, "--service-manager", "systemd",
		"--hub", "H", "--live", "22"); err != nil {
		t.Fatal(err)
	}
	installed := readFile(t, filepath.Join(root, "usr/local/bin/goholesail-host"))
	committed := readFile(t, wrapperFile)
	if installed != committed {
		t.Fatalf("wrapper drift: install.sh-generated wrapper != committed deploy/goholesail-host\n--- installed ---\n%s\n--- committed ---\n%s", installed, committed)
	}
}

// makeReleaseServer serves a fake release at routePrefix (e.g. "/latest/download/"
// or "/download/v1.2.3/"): a tar.gz named `asset` containing a goholesail binary,
// plus checksums.txt. If corruptSum is true the checksum is wrong.
func makeReleaseServer(t *testing.T, corruptSum bool, asset, routePrefix string) (*httptest.Server, string) {
	t.Helper()
	content := "#!/bin/sh\necho downloaded\n"

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "goholesail", Mode: 0o755, Size: int64(len(content))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	tgz := buf.Bytes()

	sum := sha256.Sum256(tgz)
	hexSum := hex.EncodeToString(sum[:])
	if corruptSum {
		hexSum = strings.Repeat("0", 64)
	}
	checks := fmt.Sprintf("%s  %s\n", hexSum, asset)

	mux := http.NewServeMux()
	mux.HandleFunc(routePrefix+asset, func(w http.ResponseWriter, _ *http.Request) {
		w.Write(tgz)
	})
	mux.HandleFunc(routePrefix+"checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(checks))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, content
}

func TestInstallDownload(t *testing.T) {
	srv, content := makeReleaseServer(t, false, "goholesail_linux_amd64.tar.gz", "/latest/download/")
	root := t.TempDir()
	out, err := runInstall(t, []string{"GHS_OS=linux", "GHS_ARCH=amd64"},
		"--root", root, "--url-base", srv.URL, "--service-manager", "systemd",
		"--hub", "H", "--live", "22")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	got := readFile(t, filepath.Join(root, "usr/local/bin/goholesail"))
	if got != content {
		t.Fatalf("downloaded binary content mismatch:\n got %q\nwant %q", got, content)
	}
}

func TestInstallDownloadBadChecksum(t *testing.T) {
	srv, _ := makeReleaseServer(t, true, "goholesail_linux_amd64.tar.gz", "/latest/download/")
	root := t.TempDir()
	if _, err := runInstall(t, []string{"GHS_OS=linux", "GHS_ARCH=amd64"},
		"--root", root, "--url-base", srv.URL, "--service-manager", "systemd",
		"--hub", "H", "--live", "22"); err == nil {
		t.Fatal("expected non-zero exit on bad checksum")
	}
	if _, err := os.Stat(filepath.Join(root, "usr/local/bin/goholesail")); err == nil {
		t.Fatal("binary should not be installed when checksum fails")
	}
}

func TestInstallDownloadVersioned(t *testing.T) {
	srv, content := makeReleaseServer(t, false, "goholesail_linux_amd64.tar.gz", "/download/v1.2.3/")
	root := t.TempDir()
	out, err := runInstall(t, []string{"GHS_OS=linux", "GHS_ARCH=amd64"},
		"--root", root, "--url-base", srv.URL, "--version", "v1.2.3",
		"--service-manager", "systemd", "--hub", "H", "--live", "22")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(root, "usr/local/bin/goholesail")); got != content {
		t.Fatalf("versioned download mismatch:\n got %q\nwant %q", got, content)
	}
}

func TestInstallDownloadArm64(t *testing.T) {
	// GHS_ARCH=aarch64 also exercises the aarch64->arm64 mapping.
	srv, content := makeReleaseServer(t, false, "goholesail_linux_arm64.tar.gz", "/latest/download/")
	root := t.TempDir()
	out, err := runInstall(t, []string{"GHS_OS=linux", "GHS_ARCH=aarch64"},
		"--root", root, "--url-base", srv.URL, "--service-manager", "systemd",
		"--hub", "H", "--live", "22")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(root, "usr/local/bin/goholesail")); got != content {
		t.Fatalf("arm64 download mismatch:\n got %q\nwant %q", got, content)
	}
}
