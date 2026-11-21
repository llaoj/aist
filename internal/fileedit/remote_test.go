package fileedit

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llaoj/aist/internal/scriptexec"
)

// localShFakeRemoteRunner executes the real POSIX sh helper scripts locally
// with a dedicated HOME directory, exercising both RunPipe and RunInteractive
// paths (including marker extraction) end-to-end.
type localShFakeRemoteRunner struct {
	remoteHome      string
	pipeCalls       int
	interactiveSudo int
	forcePermOnPipe bool
}

func (r *localShFakeRemoteRunner) RunPipe(ctx context.Context, req scriptexec.PipeRequest) ([]byte, string, error) {
	r.pipeCalls++
	if r.forcePermOnPipe {
		return nil, "AIST_ERR:PERM:" + req.Args[0], errors.New("exit status 22")
	}
	argv := append([]string{"-c", req.Script, "_"}, req.Args...)
	cmd := exec.CommandContext(ctx, "sh", argv...)
	cmd.Env = append(os.Environ(), "HOME="+r.remoteHome)
	if len(req.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.Bytes(), errBuf.String(), err
}

func (r *localShFakeRemoteRunner) RunInteractive(ctx context.Context, req scriptexec.InteractiveRequest) (string, error) {
	if req.UseSudo {
		r.interactiveSudo++
	}
	argv := append([]string{"-c", req.Script, "_"}, req.Args...)
	cmd := exec.CommandContext(ctx, "sh", argv...)
	cmd.Env = append(os.Environ(), "HOME="+r.remoteHome)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	raw := buf.String()
	if req.BeginMarker != "" && req.EndMarker != "" {
		start := strings.Index(raw, req.BeginMarker)
		end := strings.Index(raw, req.EndMarker)
		if start >= 0 && end > start {
			body := strings.TrimPrefix(raw[start+len(req.BeginMarker):end], "\n")
			if err != nil {
				return body, errors.New(raw)
			}
			return body, nil
		}
	}
	if err != nil {
		return "", errors.New(raw)
	}
	return raw, nil
}

func TestRemotePrepareApplyAndBackup(t *testing.T) {
	localHome := t.TempDir()
	remoteHome := t.TempDir()
	remoteFile := filepath.Join(remoteHome, ".ssh", "config")
	writeTestFile(t, remoteFile, "Host db\n  HostName 10.0.1.10\n", 0o640)

	runner := &localShFakeRemoteRunner{remoteHome: remoteHome}
	editor := NewEditor(EditorOptions{HomeDir: localHome, Runner: runner})

	change, err := editor.Prepare(context.Background(), EditRequest{
		Target:     "web1",
		Path:       "~/.ssh/config",
		OldContent: "  HostName 10.0.1.10\n",
		NewContent: "  HostName 10.0.1.20\n",
		Reason:     "Update db IP on web1",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if change.Target.String() != "web1" || change.Path != remoteFile {
		t.Fatalf("change target=%q path=%q", change.Target.String(), change.Path)
	}
	if change.UseSudo {
		t.Fatal("user-owned remote file should not require sudo")
	}
	if !strings.Contains(change.Diff, "--- web1:"+remoteFile) {
		t.Fatalf("diff missing remote target header:\n%s", change.Diff)
	}
	if change.LocationLabel() != "[web1] "+remoteFile {
		t.Fatalf("LocationLabel = %q", change.LocationLabel())
	}

	if err := editor.Apply(context.Background(), change); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	updated, err := os.ReadFile(remoteFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(updated), "10.0.1.20") {
		t.Fatalf("updated = %q", updated)
	}
	info, _ := os.Stat(remoteFile)
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("perm = %o, want 0640", info.Mode().Perm())
	}

	backups := 0
	backupContent := ""
	_ = filepath.WalkDir(filepath.Join(localHome, ".aist", "fileedit", "backups"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".bak") {
			return nil
		}
		backups++
		data, _ := os.ReadFile(path)
		backupContent += string(data)
		return nil
	})
	if backups != 1 {
		t.Fatalf("backups = %d, want 1", backups)
	}
	if !strings.Contains(backupContent, "10.0.1.10") {
		t.Fatalf("backup = %q", backupContent)
	}
}

func TestRemoteSudoPrepareAndStagedApply(t *testing.T) {
	localHome := t.TempDir()
	remoteHome := t.TempDir()
	etcFile := filepath.Join(remoteHome, "nginx.conf")
	writeTestFile(t, etcFile, "listen 80;\n", 0o644)

	runner := &localShFakeRemoteRunner{
		remoteHome:      remoteHome,
		forcePermOnPipe: true,
	}
	editor := NewEditor(EditorOptions{HomeDir: localHome, Runner: runner})

	// Without sudo=true, unreadable file returns actionable hint.
	if _, err := editor.Prepare(context.Background(), EditRequest{
		Target:     "web1",
		Path:       etcFile,
		OldContent: "listen 80;\n",
		NewContent: "listen 8080;\n",
	}); err == nil || !strings.Contains(err.Error(), "sudo=true") {
		t.Fatalf("expected sudo=true hint, got: %v", err)
	}

	// With sudo=true, Prepare reads via interactive sudo script and Apply stages + installs via sudo.
	change, err := editor.Prepare(context.Background(), EditRequest{
		Target:     "web1",
		Path:       etcFile,
		OldContent: "listen 80;\n",
		NewContent: "listen 8080;\n",
		UseSudo:    true,
	})
	if err != nil {
		t.Fatalf("Prepare with sudo: %v", err)
	}
	if !change.UseSudo || change.LocationLabel() != "[web1] "+etcFile+" (sudo)" {
		t.Fatalf("change = %+v label=%q", change, change.LocationLabel())
	}

	// Allow staging pipe call during Apply.
	runner.forcePermOnPipe = false
	if err := editor.Apply(context.Background(), change); err != nil {
		t.Fatalf("Apply with sudo: %v", err)
	}
	if runner.interactiveSudo < 2 {
		t.Fatalf("expected interactive sudo for both unreadable Prepare and Apply, got %d", runner.interactiveSudo)
	}
	updated, _ := os.ReadFile(etcFile)
	if string(updated) != "listen 8080;\n" {
		t.Fatalf("updated = %q", updated)
	}
}

func TestRemoteCASDetectsConcurrentModification(t *testing.T) {
	localHome := t.TempDir()
	remoteHome := t.TempDir()
	remoteFile := filepath.Join(remoteHome, "app.conf")
	writeTestFile(t, remoteFile, "workers=2\n", 0o600)

	runner := &localShFakeRemoteRunner{remoteHome: remoteHome}
	editor := NewEditor(EditorOptions{HomeDir: localHome, Runner: runner})

	change, err := editor.Prepare(context.Background(), EditRequest{
		Target:     "web1",
		Path:       remoteFile,
		OldContent: "workers=2\n",
		NewContent: "workers=4\n",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	writeTestFile(t, remoteFile, "workers=8\n", 0o600)
	if err := editor.Apply(context.Background(), change); err == nil || !strings.Contains(err.Error(), "file changed after the diff was prepared") {
		t.Fatalf("expected CAS conflict, got: %v", err)
	}
	after, _ := os.ReadFile(remoteFile)
	if string(after) != "workers=8\n" {
		t.Fatalf("concurrent file was overwritten: %q", after)
	}
}

func TestRemoteRejectsInvalidTargetAndPrivateKeys(t *testing.T) {
	localHome := t.TempDir()
	runner := &localShFakeRemoteRunner{remoteHome: t.TempDir()}
	editor := NewEditor(EditorOptions{HomeDir: localHome, Runner: runner})

	if _, err := editor.Prepare(context.Background(), EditRequest{
		Target:     "-oProxyCommand=evil",
		Path:       "/etc/hosts",
		OldContent: "a",
		NewContent: "b",
	}); err == nil {
		t.Fatal("expected invalid target error")
	}

	if _, err := editor.Prepare(context.Background(), EditRequest{
		Target:     "web1",
		Path:       "~/.ssh/id_ed25519",
		OldContent: "a",
		NewContent: "b",
	}); !errors.Is(err, ErrPrivateKey) {
		t.Fatalf("err = %v, want ErrPrivateKey", err)
	}
	if runner.pipeCalls != 0 {
		t.Fatalf("private key path must be rejected before SSH call, pipeCalls=%d", runner.pipeCalls)
	}
}
