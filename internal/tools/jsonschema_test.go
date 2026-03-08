package tools

import (
	"path/filepath"
	"runtime"
	"testing"

	inspectpkg "github.com/organic-programming/grace-op/internal/inspect"
)

func TestJSONSchemaForMethod(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	protoDir := filepath.Join(filepath.Dir(file), "..", "..", "internal", "cli", "testsupport", "echoholon", "protos")
	catalog, err := inspectpkg.ParseCatalog(protoDir)
	if err != nil {
		t.Fatalf("ParseCatalog returned error: %v", err)
	}
	if len(catalog.Methods) != 1 {
		t.Fatalf("methods = %d, want 1", len(catalog.Methods))
	}

	schema := JSONSchemaForMethod(catalog.Methods[0].Method)
	if schema["type"] != "object" {
		t.Fatalf("schema type = %#v, want object", schema["type"])
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v, want map", schema["properties"])
	}
	message, ok := properties["message"].(map[string]any)
	if !ok {
		t.Fatalf("message schema = %#v, want map", properties["message"])
	}
	if message["type"] != "string" {
		t.Fatalf("message type = %#v, want string", message["type"])
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required = %#v, want []string", schema["required"])
	}
	if len(required) != 1 || required[0] != "message" {
		t.Fatalf("required = %#v, want [message]", required)
	}

	tags, ok := properties["tags"].(map[string]any)
	if !ok {
		t.Fatalf("tags schema = %#v, want map", properties["tags"])
	}
	if tags["type"] != "array" {
		t.Fatalf("tags type = %#v, want array", tags["type"])
	}
	items, ok := tags["items"].(map[string]any)
	if !ok || items["type"] != "string" {
		t.Fatalf("tags items = %#v, want string items", tags["items"])
	}

	mode, ok := properties["mode"].(map[string]any)
	if !ok {
		t.Fatalf("mode schema = %#v, want map", properties["mode"])
	}
	enums, ok := mode["enum"].([]any)
	if !ok {
		t.Fatalf("mode enum = %#v, want []any", mode["enum"])
	}
	if len(enums) != 3 || enums[1] != "ECHO_MODE_UPPER" {
		t.Fatalf("mode enum = %#v, want enum values", enums)
	}

	examples, ok := schema["examples"].([]any)
	if !ok || len(examples) != 1 {
		t.Fatalf("schema examples = %#v, want one example", schema["examples"])
	}
}
