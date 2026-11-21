package adminui

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestBind(t *testing.T) {
	e := &core.ServeEvent{}

	Bind(e)

	if len(e.UIExtensions) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(e.UIExtensions))
	}
	ext := e.UIExtensions[0]
	if ext.Name != Name {
		t.Fatalf("expected extension name %q, got %q", Name, ext.Name)
	}
	if ext.FS == nil {
		t.Fatal("expected a non-nil extension file system")
	}
	for _, name := range []string{"main.js", "logo.svg", "logo-bw.svg"} {
		if _, err := ext.FS.Open(name); err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
	}
}
