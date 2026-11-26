package frontend

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedFiles(t *testing.T) {
	for _, name := range []string{
		"index.html",
		"register.html",
		"logo.svg",
		"run.sh",
		"uninstall.sh",
		"css/style.css",
		"js/i18n.js",
		"js/main.js",
		"js/register.js",
		"js/terminal.js",
		"js/chat.js",
	} {
		if _, err := fs.Stat(FS, name); err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
	}
}

func checkPOSIXScript(t *testing.T, name string) string {
	t.Helper()

	raw, err := fs.ReadFile(FS, name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	syntaxCmd := exec.Command("/bin/sh", "-n")
	syntaxCmd.Stdin = strings.NewReader(string(raw))
	if out, err := syntaxCmd.CombinedOutput(); err != nil {
		t.Fatalf("/bin/sh -n %s failed: %v\n%s", name, err, out)
	}

	return string(raw)
}

func TestUninstallScriptSyntax(t *testing.T) {
	checkPOSIXScript(t, "uninstall.sh")
}

func TestUninstallScriptRemovesClientStateAndBinary(t *testing.T) {
	raw := checkPOSIXScript(t, "uninstall.sh")

	home := t.TempDir()
	stateDir := filepath.Join(home, ".aist")
	binDir := filepath.Join(home, ".local", "bin")
	binaryPath := filepath.Join(binDir, "aist")
	unrelatedPath := filepath.Join(binDir, "other")

	if err := os.MkdirAll(filepath.Join(stateDir, "memory"), 0o700); err != nil {
		t.Fatalf("create state dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "token.json"), []byte("token"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	if err := os.WriteFile(unrelatedPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}

	cmd := exec.Command("/bin/sh", "-s", "--", "--yes")
	cmd.Stdin = strings.NewReader(raw)
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("uninstall.sh failed: %v\n%s", err, out)
	}

	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("state dir still exists after uninstall")
	}
	if _, err := os.Stat(binaryPath); !os.IsNotExist(err) {
		t.Fatalf("binary still exists after uninstall")
	}
	if _, err := os.Stat(unrelatedPath); err != nil {
		t.Fatalf("unrelated file removed: %v", err)
	}
}

func TestRunScriptPOSIXAndPlatformDetection(t *testing.T) {
	raw := checkPOSIXScript(t, "run.sh")

	libScript := strings.TrimSuffix(strings.TrimSpace(raw), "\nmain")

	tests := []struct {
		name    string
		unameS  string
		unameM  string
		want    string
		wantErr bool
	}{
		{name: "linux x86_64", unameS: "Linux", unameM: "x86_64", want: "linux/amd64"},
		{name: "linux aarch64", unameS: "Linux", unameM: "aarch64", want: "linux/arm64"},
		{name: "raspberry pi zero armv6l", unameS: "Linux", unameM: "armv6l", want: "linux/arm"},
		{name: "raspberry pi 32bit armv7l", unameS: "Linux", unameM: "armv7l", want: "linux/arm"},
		{name: "arm64 32bit compat armv8l", unameS: "Linux", unameM: "armv8l", want: "linux/arm"},
		{name: "linux 32bit x86 i686", unameS: "Linux", unameM: "i686", want: "linux/386"},
		{name: "linux riscv64", unameS: "Linux", unameM: "riscv64", want: "linux/riscv64"},
		{name: "darwin x86_64", unameS: "Darwin", unameM: "x86_64", want: "darwin/amd64"},
		{name: "darwin arm64", unameS: "Darwin", unameM: "arm64", want: "darwin/arm64"},
		{name: "unsupported armv5tel", unameS: "Linux", unameM: "armv5tel", wantErr: true},
		{name: "unsupported darwin 386", unameS: "Darwin", unameM: "i686", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness := libScript + "\n" +
				"uname() { if [ \"$1\" = \"-s\" ]; then printf '%s\\n' '" + tt.unameS + "'; else printf '%s\\n' '" + tt.unameM + "'; fi; }\n" +
				"os=$(detect_os) && arch=$(detect_arch) && validate_platform \"$os\" \"$arch\" && printf '%s/%s' \"$os\" \"$arch\"\n"

			cmd := exec.Command("/bin/sh", "-e", "-c", harness)
			out, err := cmd.Output()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", string(out))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := string(out); got != tt.want {
				t.Fatalf("platform = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunScriptFetchRetry(t *testing.T) {
	raw := checkPOSIXScript(t, "run.sh")
	libScript := strings.TrimSuffix(strings.TrimSpace(raw), "\nmain")

	t.Run("retries transient curl failure under set -e", func(t *testing.T) {
		dir := t.TempDir()
		countFile := filepath.Join(dir, "count")
		outFile := filepath.Join(dir, "out.txt")

		harness := libScript + "\n" +
			"sleep() { :; }\n" +
			"curl() {\n" +
			"  out=''\n" +
			"  while [ $# -gt 0 ]; do\n" +
			"    if [ \"$1\" = '-o' ]; then out=\"$2\"; shift 2; continue; fi\n" +
			"    shift\n" +
			"  done\n" +
			"  n=0\n" +
			"  [ -f '" + countFile + "' ] && n=$(cat '" + countFile + "')\n" +
			"  n=$((n + 1))\n" +
			"  printf '%s' \"$n\" > '" + countFile + "'\n" +
			"  if [ \"$n\" -lt 3 ]; then\n" +
			"    printf 'partial' > \"$out\"\n" +
			"    return 35\n" +
			"  fi\n" +
			"  printf 'verified-body' > \"$out\"\n" +
			"  return 0\n" +
			"}\n" +
			"fetch_url https://example.com/checksums.txt '" + outFile + "'\n"

		cmd := exec.Command("/bin/sh", "-e", "-c", harness)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fetch_url failed: %v\n%s", err, out)
		}
		gotBody, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("read output: %v", err)
		}
		if string(gotBody) != "verified-body" {
			t.Fatalf("output = %q, want %q", string(gotBody), "verified-body")
		}
		gotCount, err := os.ReadFile(countFile)
		if err != nil {
			t.Fatalf("read count: %v", err)
		}
		if string(gotCount) != "3" {
			t.Fatalf("attempt count = %q, want %q", string(gotCount), "3")
		}
	})

	t.Run("stops after max attempts on persistent failure", func(t *testing.T) {
		dir := t.TempDir()
		countFile := filepath.Join(dir, "count")
		outFile := filepath.Join(dir, "out.txt")

		harness := libScript + "\n" +
			"sleep() { :; }\n" +
			"curl() {\n" +
			"  n=0\n" +
			"  [ -f '" + countFile + "' ] && n=$(cat '" + countFile + "')\n" +
			"  n=$((n + 1))\n" +
			"  printf '%s' \"$n\" > '" + countFile + "'\n" +
			"  return 35\n" +
			"}\n" +
			"if fetch_url_progress https://example.com/aist.tar.gz '" + outFile + "'; then\n" +
			"  exit 0\n" +
			"else\n" +
			"  exit 42\n" +
			"fi\n"

		cmd := exec.Command("/bin/sh", "-e", "-c", harness)
		err := cmd.Run()
		if err == nil {
			t.Fatal("expected failure after max attempts")
		}
		if !strings.Contains(err.Error(), "exit status 42") {
			t.Fatalf("err = %v, want exit status 42", err)
		}
		gotCount, err := os.ReadFile(countFile)
		if err != nil {
			t.Fatalf("read count: %v", err)
		}
		if string(gotCount) != "3" {
			t.Fatalf("attempt count = %q, want %q", string(gotCount), "3")
		}
	})
}
