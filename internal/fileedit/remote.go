package fileedit

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/llaoj/aist/internal/scriptexec"
	"github.com/llaoj/aist/internal/target"
)

type inspectedFile struct {
	Path     string
	Exists   bool
	Writable bool
	Perm     os.FileMode
	Content  []byte
}

func (e *Editor) inspectViaRunner(ctx context.Context, tgt target.Target, reqPath string, allowSudo bool) (inspectedFile, bool, error) {
	if e.opts.Runner == nil {
		return inspectedFile{}, false, errors.New("remote or sudo file_edit is unavailable: system runner is not configured")
	}
	maxStr := strconv.FormatInt(e.opts.MaxBytes, 10)

	if !tgt.IsLocal() {
		stdout, stderr, err := e.opts.Runner.RunPipe(ctx, scriptexec.PipeRequest{
			Target: tgt,
			Script: inspectScript,
			Args:   []string{reqPath, maxStr, "0"},
		})
		if err == nil {
			parsed, parseErr := parseInspectedOutput(string(stdout))
			if parseErr != nil {
				return inspectedFile{}, false, parseErr
			}
			return parsed, allowSudo || !parsed.Writable, nil
		}
		if !isScriptPermError(stderr) {
			return inspectedFile{}, false, mapScriptError(err, stderr)
		}
		if !allowSudo {
			return inspectedFile{}, false, fmt.Errorf("permission denied accessing %s on %s; retry file_edit with sudo=true", reqPath, tgt.String())
		}
	} else if !allowSudo {
		return inspectedFile{}, false, errors.New("local sudo inspection requires sudo=true")
	}

	payload, err := e.opts.Runner.RunInteractive(ctx, scriptexec.InteractiveRequest{
		Target:      tgt,
		UseSudo:     true,
		Script:      inspectScript,
		Args:        []string{reqPath, maxStr, "1"},
		BeginMarker: scriptMarkerBegin,
		EndMarker:   scriptMarkerEnd,
	})
	if err != nil {
		return inspectedFile{}, true, mapScriptError(err, err.Error())
	}
	parsed, err := parseInspectedOutput(payload)
	if err != nil {
		return inspectedFile{}, true, err
	}
	return parsed, true, nil
}

func parseInspectedOutput(raw string) (inspectedFile, error) {
	var out inspectedFile
	out.Perm = defaultFileMode

	var inData bool
	var b64 strings.Builder
	var status string

	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if inData {
			if line == "DATA_END" {
				inData = false
				continue
			}
			b64.WriteString(strings.TrimSpace(line))
			continue
		}
		switch {
		case line == "DATA_BEGIN":
			inData = true
		case strings.HasPrefix(line, "STATUS="):
			status = strings.TrimPrefix(line, "STATUS=")
		case strings.HasPrefix(line, "PATH="):
			out.Path = strings.TrimPrefix(line, "PATH=")
		case strings.HasPrefix(line, "WRITABLE="):
			out.Writable = strings.TrimPrefix(line, "WRITABLE=") == "1"
		case strings.HasPrefix(line, "MODE="):
			modeStr := strings.TrimSpace(strings.TrimPrefix(line, "MODE="))
			if parsed, err := strconv.ParseUint(modeStr, 8, 32); err == nil {
				out.Perm = os.FileMode(parsed) & os.ModePerm
			}
		}
	}

	if out.Path == "" || (status != "EXISTS" && status != "NEW") {
		return inspectedFile{}, errors.New("failed to inspect target file: unexpected helper output")
	}
	if !path.IsAbs(out.Path) {
		return inspectedFile{}, fmt.Errorf("%w: resolved path %s is not absolute", ErrUnsafePath, out.Path)
	}
	if err := rejectSpecialFilesystemPath(out.Path); err != nil {
		return inspectedFile{}, err
	}
	if isPrivateKeyPath(out.Path) {
		return inspectedFile{}, ErrPrivateKey
	}

	if status == "EXISTS" {
		decoded, err := base64.StdEncoding.DecodeString(b64.String())
		if err != nil {
			return inspectedFile{}, fmt.Errorf("failed to decode remote file content: %w", err)
		}
		out.Exists = true
		out.Content = decoded
	}
	return out, nil
}

func (e *Editor) applyViaRunner(ctx context.Context, change *Change) error {
	if e.opts.Runner == nil {
		return errors.New("remote or sudo file_edit is unavailable: system runner is not configured")
	}

	existsFlag := "0"
	expectedHash := "NEW"
	if change.Exists {
		existsFlag = "1"
		expectedHash = change.OriginalHash
	}
	modeStr := fmt.Sprintf("%04o", change.Perm.Perm())
	if change.Perm.Perm() == 0 {
		modeStr = fmt.Sprintf("%04o", defaultFileMode)
	}
	lenStr := strconv.Itoa(len(change.Updated))

	if !change.Target.IsLocal() && !change.UseSudo {
		_, stderr, err := e.opts.Runner.RunPipe(ctx, scriptexec.PipeRequest{
			Target: change.Target,
			Script: applyScript,
			Args:   []string{change.Path, "-", existsFlag, expectedHash, modeStr, lenStr},
			Stdin:  change.Updated,
		})
		if err != nil {
			if isScriptPermError(stderr) || strings.Contains(strings.ToLower(stderr), "permission denied") {
				return fmt.Errorf("permission denied writing %s on %s; retry file_edit with sudo=true", change.Path, change.Target.String())
			}
			return mapScriptError(err, stderr)
		}
		return nil
	}

	if change.Target.IsLocal() {
		return e.applyLocalSudo(ctx, change, existsFlag, expectedHash, modeStr, lenStr)
	}
	return e.applyRemoteSudo(ctx, change, existsFlag, expectedHash, modeStr, lenStr)
}

func (e *Editor) applyLocalSudo(ctx context.Context, change *Change, existsFlag, expectedHash, modeStr, lenStr string) error {
	tmpFile, err := os.CreateTemp("", "aist-edit-*")
	if err != nil {
		return fmt.Errorf("failed to stage temporary file: %w", err)
	}
	stagePath := tmpFile.Name()
	defer func() { _ = os.Remove(stagePath) }()

	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to tighten temporary file permissions: %w", err)
	}
	if _, err := tmpFile.Write(change.Updated); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write temporary file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	_, err = e.opts.Runner.RunInteractive(ctx, scriptexec.InteractiveRequest{
		Target:  target.Local(),
		UseSudo: true,
		Script:  applyScript,
		Args:    []string{change.Path, stagePath, existsFlag, expectedHash, modeStr, lenStr},
	})
	if err != nil {
		return mapScriptError(err, err.Error())
	}
	return nil
}

func (e *Editor) applyRemoteSudo(ctx context.Context, change *Change, existsFlag, expectedHash, modeStr, lenStr string) error {
	stdout, stderr, err := e.opts.Runner.RunPipe(ctx, scriptexec.PipeRequest{
		Target: change.Target,
		Script: stageScript,
		Args:   []string{lenStr},
		Stdin:  change.Updated,
	})
	if err != nil {
		return mapScriptError(err, stderr)
	}
	remoteStage := strings.TrimSpace(string(stdout))
	if remoteStage == "" {
		return errors.New("failed to stage remote temporary file")
	}

	_, err = e.opts.Runner.RunInteractive(ctx, scriptexec.InteractiveRequest{
		Target:  change.Target,
		UseSudo: true,
		Script:  applyScript,
		Args:    []string{change.Path, remoteStage, existsFlag, expectedHash, modeStr, lenStr},
	})
	if err != nil {
		// Ensure the staged file is removed even if sudo authentication failed
		// before the apply script ran.
		_, _, _ = e.opts.Runner.RunPipe(context.Background(), scriptexec.PipeRequest{
			Target: change.Target,
			Script: `rm -f -- "$1"`,
			Args:   []string{remoteStage},
		})
		return mapScriptError(err, err.Error())
	}
	return nil
}

func isScriptPermError(detail string) bool {
	return strings.Contains(detail, "AIST_ERR:PERM:")
}

func mapScriptError(err error, detail string) error {
	for _, line := range strings.Split(strings.ReplaceAll(detail, "\r\n", "\n"), "\n") {
		idx := strings.Index(line, "AIST_ERR:")
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(line[idx+len("AIST_ERR:"):])
		parts := strings.SplitN(rest, ":", 2)
		code := parts[0]
		msg := ""
		if len(parts) == 2 {
			msg = strings.TrimSpace(parts[1])
		}
		switch code {
		case "UNSAFE_PATH", "SYMLINK", "NOT_REGULAR", "NOT_DIR_PARENT":
			return fmt.Errorf("%w: %s", ErrUnsafePath, msg)
		case "TOO_LARGE":
			return fmt.Errorf("%w: %s", ErrTooLarge, msg)
		case "NO_PARENT":
			return errors.New(msg)
		case "CONCURRENT":
			return errors.New("file changed after the diff was prepared; no changes were written")
		case "ALREADY_EXISTS":
			return errors.New("file already exists; no changes were written")
		case "PERM":
			return fmt.Errorf("permission denied: %s", msg)
		case "TRUNCATED":
			return fmt.Errorf("incomplete write: %s", msg)
		}
	}
	return err
}
