package fileedit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/llaoj/aist/internal/target"
)

func TestNormalizeLocalPath(t *testing.T) {
	home := t.TempDir()
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{"home", "~/config", filepath.Join(home, "config"), nil},
		{"absolute", "/etc/hosts", "/etc/hosts", nil},
		{"relative", "config", "", ErrUnsafePath},
		{"control", "/tmp/a\nb", "", ErrUnsafePath},
		{"proc", "/proc/self/environ", "", ErrUnsafePath},
		{"dev root", "/dev", "", ErrUnsafePath},
		{"private key", "~/.ssh/id_ed25519", "", ErrPrivateKey},
		{"public key allowed", "~/.ssh/id_ed25519.pub", filepath.Join(home, ".ssh/id_ed25519.pub"), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeLocalPath(tc.raw, home)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeRemotePath(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{"home", "~/.ssh/config", "~/.ssh/config", nil},
		{"absolute", "/etc/nginx/nginx.conf", "/etc/nginx/nginx.conf", nil},
		{"home cannot escape", "~/../etc/passwd", "~/etc/passwd", nil},
		{"home root", "~/", "", ErrUnsafePath},
		{"relative", "etc/passwd", "", ErrUnsafePath},
		{"control", "/tmp/a\rb", "", ErrUnsafePath},
		{"proc", "/proc/1/environ", "", ErrUnsafePath},
		{"private key", "/root/.ssh/id_rsa", "", ErrPrivateKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeRemotePath(tc.raw)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStateKey(t *testing.T) {
	if got := stateKey(target.Local(), "/etc/hosts"); got != "/etc/hosts" {
		t.Fatalf("local key = %q", got)
	}
	remote, err := target.New("web1")
	if err != nil {
		t.Fatalf("target.New: %v", err)
	}
	if got := stateKey(remote, "/etc/nginx/nginx.conf"); got != "/@remote/web1/etc/nginx/nginx.conf" {
		t.Fatalf("remote key = %q", got)
	}
}

func TestCanWriteLocalDir(t *testing.T) {
	dir := t.TempDir()
	if !canWriteLocalDir(dir) {
		t.Fatal("writable directory reported as unwritable")
	}

	readOnly := filepath.Join(dir, "readonly")
	if err := os.Mkdir(readOnly, 0o500); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	defer func() { _ = os.Chmod(readOnly, 0o700) }()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	if canWriteLocalDir(readOnly) {
		t.Fatal("read-only directory reported as writable")
	}
}

func TestCanReplaceLocalFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !canReplaceLocalFile(file, info) {
		t.Fatal("user-owned file in writable directory reported as unwritable")
	}
}
