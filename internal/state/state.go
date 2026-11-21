package state

// Root bundles the state layout with its backup service.
type Root struct {
	Paths   Paths
	Backups *BackupStore
}

// EnsureRoot prepares a state root (defaulting to ~/.aist when root is empty)
// and returns its services.
func EnsureRoot(root string) (*Root, error) {
	paths, err := ResolvePaths(root)
	if err != nil {
		return nil, err
	}
	if err := paths.Ensure(); err != nil {
		return nil, err
	}
	return NewFromPaths(paths), nil
}

// NewFromPaths wraps an existing layout.
func NewFromPaths(paths Paths) *Root {
	return &Root{
		Paths:   paths,
		Backups: NewBackupStore(paths),
	}
}
