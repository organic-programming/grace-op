package scaffold

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestListIncludesTemplatesAndCompositeAliases(t *testing.T) {
	entries, err := List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	names := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		names[entry.Name] = entry
	}

	for _, name := range []string{"go-daemon", "hostui-web", "wrapper-cli", "composition-direct-call", "composite-go-swiftui"} {
		if _, ok := names[name]; !ok {
			t.Fatalf("template %q missing from catalog", name)
		}
	}
}

func TestGenerateGoDaemonAppliesOverrides(t *testing.T) {
	root := t.TempDir()

	result, err := Generate("go-daemon", "alpha-builder", GenerateOptions{
		Dir: root,
		Overrides: map[string]string{
			"service": "EchoService",
		},
	})
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	if result.Template != "go-daemon" {
		t.Fatalf("result.Template = %q, want %q", result.Template, "go-daemon")
	}

	mainPath := filepath.Join(root, "alpha-builder", "cmd", "alpha-builder", "main.go")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) failed: %v", mainPath, err)
	}
	if !strings.Contains(string(data), "EchoService ready for alpha-builder") {
		t.Fatalf("main.go missing overridden service: %s", string(data))
	}

	manifestPath := filepath.Join(root, "alpha-builder", "holon.yaml")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) failed: %v", manifestPath, err)
	}
	uuidPattern := regexp.MustCompile(`uuid: "[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}"`)
	if !uuidPattern.Match(manifestData) {
		t.Fatalf("generated manifest missing UUIDv4: %s", string(manifestData))
	}
}

func TestGenerateCompositeAliasRendersKinds(t *testing.T) {
	root := t.TempDir()

	result, err := Generate("composite-go-swiftui", "orbit-console", GenerateOptions{Dir: root})
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	if result.Template != "composite-go-swiftui" {
		t.Fatalf("result.Template = %q, want %q", result.Template, "composite-go-swiftui")
	}

	manifestPath := filepath.Join(root, "orbit-console", "holon.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) failed: %v", manifestPath, err)
	}
	content := string(data)
	for _, expected := range []string{
		"motto: \"go + swiftui composite.\"",
		"primary: app/app.txt",
		"path: daemon",
		"path: app",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("manifest missing %q:\n%s", expected, content)
		}
	}
}
