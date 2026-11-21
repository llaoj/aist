package token

import (
	"os"
	"testing"

	"github.com/llaoj/aist/internal/state"
)

func testState(dir string) *state.Root {
	return state.NewFromPaths(state.NewPaths(dir))
}

func TestSaveAndLoad(t *testing.T) {
	stateRoot := testState(t.TempDir())
	store := NewStore(StoreOptions{State: stateRoot, LookupEnv: func(string) (string, bool) { return "", false }})

	creds := Credentials{ServerURL: "https://api.example.com", Token: "tok-123"}
	if err := store.Save(creds); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o, want 0600", info.Mode().Perm())
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Token != "tok-123" || loaded.ServerURL != "https://api.example.com" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	store := NewStore(StoreOptions{State: testState(t.TempDir()), LookupEnv: func(string) (string, bool) { return "", false }})
	creds, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if creds.Token != "" {
		t.Fatalf("expected empty token, got %q", creds.Token)
	}
}

func TestEnvOverridesServer(t *testing.T) {
	stateRoot := testState(t.TempDir())
	store := NewStore(StoreOptions{State: stateRoot, LookupEnv: func(string) (string, bool) { return "", false }})
	if err := store.Save(Credentials{ServerURL: "https://stored.example.com", Token: "stored"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	env := map[string]string{EnvServer: "https://env.example.com"}
	store = NewStore(StoreOptions{State: stateRoot, LookupEnv: func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}})
	creds, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if creds.ServerURL != "https://env.example.com" || creds.Token != "stored" {
		t.Fatalf("creds = %+v", creds)
	}
}

func TestClear(t *testing.T) {
	stateRoot := testState(t.TempDir())
	store := NewStore(StoreOptions{State: stateRoot, LookupEnv: func(string) (string, bool) { return "", false }})
	if err := store.Save(Credentials{ServerURL: "s", Token: "t"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(stateRoot.Paths.TokenFile); !os.IsNotExist(err) {
		t.Fatalf("token file still present: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear missing: %v", err)
	}
}

func TestSaveRequiresToken(t *testing.T) {
	store := NewStore(StoreOptions{State: testState(t.TempDir()), LookupEnv: func(string) (string, bool) { return "", false }})
	if err := store.Save(Credentials{ServerURL: "s"}); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestStoreRequiresStateRoot(t *testing.T) {
	store := NewStore(StoreOptions{})
	if _, err := store.Load(); err == nil {
		t.Fatal("expected Load error without state root")
	}
	if err := store.Save(Credentials{ServerURL: "s", Token: "t"}); err == nil {
		t.Fatal("expected Save error without state root")
	}
	if err := store.Clear(); err == nil {
		t.Fatal("expected Clear error without state root")
	}
}

func TestSaveServerURLWithoutLogin(t *testing.T) {
	store := NewStore(StoreOptions{State: testState(t.TempDir()), LookupEnv: func(string) (string, bool) { return "", false }})

	if err := store.SaveServerURL(" https://aist.example.com "); err != nil {
		t.Fatalf("SaveServerURL: %v", err)
	}
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o, want 0600", info.Mode().Perm())
	}
	creds, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if creds.ServerURL != "https://aist.example.com" || creds.Token != "" {
		t.Fatalf("creds = %+v", creds)
	}

	if err := store.SaveServerURL("https://other.example.com"); err != nil {
		t.Fatalf("SaveServerURL again: %v", err)
	}
	if creds, _ := store.Load(); creds.ServerURL != "https://other.example.com" {
		t.Fatalf("server = %q, want the latest attempt", creds.ServerURL)
	}
}

func TestSaveServerURLKeepsExistingLogin(t *testing.T) {
	store := NewStore(StoreOptions{State: testState(t.TempDir()), LookupEnv: func(string) (string, bool) { return "", false }})
	existing := Credentials{ServerURL: "https://a.example.com", Token: "tok-a", Email: "alice@example.com"}
	if err := store.Save(existing); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.SaveServerURL("https://b.example.com"); err != nil {
		t.Fatalf("SaveServerURL: %v", err)
	}
	creds, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if creds != existing {
		t.Fatalf("creds = %+v, want %+v", creds, existing)
	}
}

func TestSaveServerURLIgnoresEmpty(t *testing.T) {
	store := NewStore(StoreOptions{State: testState(t.TempDir()), LookupEnv: func(string) (string, bool) { return "", false }})
	if err := store.SaveServerURL("  "); err != nil {
		t.Fatalf("SaveServerURL: %v", err)
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Fatalf("token file created for empty server: %v", err)
	}
}
