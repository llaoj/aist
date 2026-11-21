package tools

import (
	"strings"
	"testing"
)

func TestFileEditDefinitionAndArgs(t *testing.T) {
	def := FileEditDefinition()
	if def.Name != ToolFileEdit || len(def.InputSchema) == 0 {
		t.Fatalf("definition = %+v", def)
	}
	for _, want := range []string{
		"old_content must be an exact snippet",
		"remote SSH target",
		"Set sudo=true",
		"asks the user before writing",
		"Never claim a file was",
		"use shell_execute instead",
	} {
		if !strings.Contains(def.Description, want) {
			t.Fatalf("description missing %q:\n%s", want, def.Description)
		}
	}
	schema := string(def.InputSchema)
	for _, field := range []string{`"target"`, `"sudo"`} {
		if !strings.Contains(schema, field) {
			t.Fatalf("schema missing %s: %s", field, schema)
		}
	}

	args, err := ParseFileEditArgs(`{
		"path":" /etc/nginx/nginx.conf ",
		"old_content":"worker_processes 1;\n",
		"new_content":"worker_processes auto;\n",
		"reason":" tune workers ",
		"target":" web1 ",
		"sudo":true
	}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if args.Path != "/etc/nginx/nginx.conf" || args.Reason != "tune workers" || args.Target != "web1" || !args.UseSudo {
		t.Fatalf("args = %+v", args)
	}
	if args.OldContent != "worker_processes 1;\n" || args.NewContent != "worker_processes auto;\n" {
		t.Fatalf("content was trimmed: %+v", args)
	}

	if _, err := ParseFileEditArgs(`{"path":"x"}`); err == nil {
		t.Fatal("expected missing content error")
	}

	if _, err := ParseFileEditArgs(`{
		"path":"/etc/hosts",
		"new_content":"x",
		"target":"-oProxyCommand=evil"
	}`); err == nil {
		t.Fatal("expected invalid target error")
	}
}
