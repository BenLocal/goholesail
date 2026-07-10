package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
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
