package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	storage "github.com/llaoj/aist/internal/storage"
	"github.com/llaoj/aist/internal/util"
)

// DefaultMaxBackups is the number of versions kept for each original file.
const DefaultMaxBackups = 10

// BackupStore keeps a small, browsable history of file versions below
// ~/.aist/<domain>/backups/<mirrored-path>/.
type BackupStore struct {
	Paths       Paths
	MaxVersions int
	Now         func() time.Time
}

// NewBackupStore creates a backup store.
func NewBackupStore(paths Paths) *BackupStore {
	return &BackupStore{Paths: paths, MaxVersions: DefaultMaxBackups, Now: time.Now}
}

type backupMeta struct {
	SchemaVersion int            `json:"schema_version"`
	OriginalPath  string         `json:"original_path"`
	Mode          string         `json:"mode"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	VersionCount  int            `json:"version_count"`
	Versions      []backupRecord `json:"versions"`
}

type backupRecord struct {
	Name string    `json:"name"`
	Time time.Time `json:"time"`
	Hash string    `json:"hash"`
}

// Backup writes data as a new version for originalPath. If the newest version
// already has identical content it is reused. A backup failure must be
// reported to the caller so the target file is not modified.
func (b *BackupStore) Backup(category, originalPath string, data []byte, mode os.FileMode) error {
	if b == nil {
		return nil
	}
	if b.MaxVersions <= 0 {
		b.MaxVersions = DefaultMaxBackups
	}
	if b.Now == nil {
		b.Now = time.Now
	}

	dir := b.Paths.BackupDir(category, originalPath)
	if err := ensureDir(dir); err != nil {
		return fmt.Errorf("failed to prepare backup directory %s: %w", dir, err)
	}

	hash := util.SHA256Hex(data)
	meta, err := readBackupMeta(filepath.Join(dir, "meta.json"))
	if err != nil {
		return err
	}
	if len(meta.Versions) > 0 && meta.Versions[len(meta.Versions)-1].Hash == hash {
		return nil
	}

	now := b.Now().UTC()
	name := now.Format("20060102T150405.000000000Z") + "-" + hash[:8] + ".bak"
	if err := storage.ReplaceFile(filepath.Join(dir, name), data, fileMode); err != nil {
		return fmt.Errorf("failed to write backup %s: %w", name, err)
	}

	meta.SchemaVersion = 1
	meta.OriginalPath = originalPath
	meta.Mode = fmt.Sprintf("%04o", mode.Perm())
	meta.UpdatedAt = now
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.Versions = append(meta.Versions, backupRecord{Name: name, Time: now, Hash: hash})
	kept, removed := pruneVersions(meta.Versions, b.MaxVersions)
	meta.Versions = kept
	meta.VersionCount = len(meta.Versions)

	if err := writeBackupMeta(filepath.Join(dir, "meta.json"), meta); err != nil {
		return err
	}
	for _, record := range removed {
		_ = os.Remove(filepath.Join(dir, record.Name))
	}
	return nil
}

func readBackupMeta(path string) (backupMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return backupMeta{}, nil
		}
		return backupMeta{}, fmt.Errorf("failed to read backup metadata %s: %w", path, err)
	}
	var meta backupMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return backupMeta{}, fmt.Errorf("failed to parse backup metadata %s: %w", path, err)
	}
	sort.Slice(meta.Versions, func(i, j int) bool {
		return meta.Versions[i].Name < meta.Versions[j].Name
	})
	return meta, nil
}

func writeBackupMeta(path string, meta backupMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode backup metadata: %w", err)
	}
	data = append(data, '\n')
	if err := storage.ReplaceFile(path, data, fileMode); err != nil {
		return fmt.Errorf("failed to write backup metadata: %w", err)
	}
	return nil
}

func pruneVersions(versions []backupRecord, keep int) (kept []backupRecord, removed []backupRecord) {
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Name < versions[j].Name
	})
	if keep > 0 && len(versions) > keep {
		removed = append(removed, versions[:len(versions)-keep]...)
		versions = versions[len(versions)-keep:]
	}
	return versions, removed
}
