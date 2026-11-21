package fileedit

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/llaoj/aist/internal/target"
	"github.com/llaoj/aist/internal/util"
)

func normalizeLocalPath(raw, home string) (string, error) {
	p := strings.TrimSpace(raw)
	if err := rejectUnsafePathCharacters(p); err != nil {
		return "", err
	}
	p = util.ExpandHome(p, home)
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%w: path must be absolute or start with ~/", ErrUnsafePath)
	}
	p = filepath.Clean(p)
	if err := rejectSpecialFilesystemPath(p); err != nil {
		return "", err
	}
	if isPrivateKeyPath(p) {
		return "", ErrPrivateKey
	}
	return p, nil
}

func normalizeRemotePath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if err := rejectUnsafePathCharacters(p); err != nil {
		return "", err
	}
	if p == "~" {
		return "", fmt.Errorf("%w: %s is a directory", ErrUnsafePath, p)
	}
	if strings.HasPrefix(p, "~/") {
		sub := path.Clean("/" + strings.TrimPrefix(p, "~/"))
		if sub == "/" {
			return "", fmt.Errorf("%w: %s is a directory", ErrUnsafePath, p)
		}
		p = "~" + sub
		if isPrivateKeyPath(p) {
			return "", ErrPrivateKey
		}
		return p, nil
	}
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("%w: path must be absolute or start with ~/", ErrUnsafePath)
	}
	p = path.Clean(p)
	if err := rejectSpecialFilesystemPath(p); err != nil {
		return "", err
	}
	if isPrivateKeyPath(p) {
		return "", ErrPrivateKey
	}
	return p, nil
}

func rejectUnsafePathCharacters(p string) error {
	if p == "" {
		return fmt.Errorf("%w: path is empty", ErrUnsafePath)
	}
	for _, r := range p {
		if r == '\n' || r == '\r' || r == 0 {
			return fmt.Errorf("%w: path contains control characters", ErrUnsafePath)
		}
	}
	return nil
}

func rejectSpecialFilesystemPath(p string) error {
	for _, prefix := range []string{"/proc", "/sys", "/dev"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return fmt.Errorf("%w: special filesystem path %s", ErrUnsafePath, p)
		}
	}
	return nil
}

func stateKey(tgt target.Target, filePath string) string {
	if tgt.IsLocal() {
		return filePath
	}
	return "/@remote/" + tgt.Host() + "/" + strings.TrimLeft(filePath, "/")
}

func canReplaceLocalFile(filePath string, info os.FileInfo) bool {
	if os.Geteuid() == 0 {
		return true
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid != uint32(os.Geteuid()) {
		return false
	}
	return canWriteLocalDir(filepath.Dir(filePath))
}

func canWriteLocalDir(dir string) bool {
	file, err := os.CreateTemp(dir, ".aist-probe-*")
	if err != nil {
		return false
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
	return true
}

func isPrivateKeyPath(p string) bool {
	base := strings.ToLower(filepath.Base(p))
	if strings.HasSuffix(base, ".pub") {
		return false
	}
	switch base {
	case "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519":
		return true
	}
	if strings.HasPrefix(base, "ssh_host_") && strings.HasSuffix(base, "_key") {
		return true
	}
	for _, suffix := range []string{".pem", ".key", ".p12", ".pfx"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return strings.Contains(base, "private_key") || strings.Contains(base, "privatekey")
}
