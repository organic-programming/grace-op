package holons

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRecipeManifest creates a holon.yaml for a composite recipe holon.
func writeRecipeManifest(t *testing.T, dir, yaml string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadManifestAcceptsCompositeRecipe(t *testing.T) {
	root := t.TempDir()

	// Create member directories.
	if err := os.MkdirAll(filepath.Join(root, "child-a"), 0755); err != nil {
		t.Fatal(err)
	}
	// child-a is a holon member — needs its own holon.yaml.
	childManifest := `schema: holon/v0
kind: native
build:
  runner: go-module
requires:
  commands: [go]
  files: [go.mod]
artifacts:
  binary: .op/build/bin/child-a
`
	if err := os.WriteFile(filepath.Join(root, "child-a", ManifestFileName), []byte(childManifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "child-b"), 0755); err != nil {
		t.Fatal(err)
	}

	manifest := `schema: holon/v0
kind: composite
build:
  runner: recipe
  defaults:
    target: macos
    mode: debug
  members:
    - id: a
      path: child-a
      type: holon
    - id: b
      path: child-b
      type: component
  targets:
    macos:
      steps:
        - build_member: a
        - exec:
            cwd: child-b
            argv: ["echo", "hello"]
artifacts:
  binary: my-app
`
	writeRecipeManifest(t, root, manifest)

	loaded, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if loaded.Manifest.Kind != KindComposite {
		t.Fatalf("kind = %q, want %q", loaded.Manifest.Kind, KindComposite)
	}
	if loaded.Manifest.Build.Runner != RunnerRecipe {
		t.Fatalf("runner = %q, want %q", loaded.Manifest.Build.Runner, RunnerRecipe)
	}
	if len(loaded.Manifest.Build.Members) != 2 {
		t.Fatalf("members = %d, want 2", len(loaded.Manifest.Build.Members))
	}
	if len(loaded.Manifest.Build.Targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(loaded.Manifest.Build.Targets))
	}
}

func TestRecipeValidationRejectsUnknownMemberRef(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "child"), 0755); err != nil {
		t.Fatal(err)
	}

	manifest := `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: child
      path: child
      type: component
  targets:
    macos:
      steps:
        - build_member: nonexistent
artifacts:
  binary: my-app
`
	writeRecipeManifest(t, root, manifest)

	_, err := LoadManifest(root)
	if err == nil {
		t.Fatal("expected error for unknown member ref")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecipeRunnerExecStep(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)

	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a proof file via exec step.
	proof := filepath.Join(workDir, "proof.txt")

	manifest := `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: work
      path: work
      type: component
  targets:
    macos:
      steps:
        - exec:
            cwd: work
            argv: ["touch", "proof.txt"]
    linux:
      steps:
        - exec:
            cwd: work
            argv: ["touch", "proof.txt"]
    darwin:
      steps:
        - exec:
            cwd: work
            argv: ["touch", "proof.txt"]
artifacts:
  primary: work/proof.txt
`
	writeRecipeManifest(t, root, manifest)

	report, err := ExecuteLifecycle(OperationBuild, root)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	if _, err := os.Stat(proof); err != nil {
		t.Fatalf("exec step did not create proof file: %v", err)
	}

	if len(report.Commands) == 0 {
		t.Fatal("expected commands in report")
	}
	if !strings.Contains(report.Commands[0], "touch") {
		t.Fatalf("expected touch in commands, got %s", report.Commands[0])
	}
}

func TestRecipeRunnerCopyStep(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)

	// Create source file.
	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "data.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	manifest := `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: src
      path: src
      type: component
  targets:
    macos:
      steps:
        - copy:
            from: src/data.txt
            to: dst/data.txt
    linux:
      steps:
        - copy:
            from: src/data.txt
            to: dst/data.txt
    darwin:
      steps:
        - copy:
            from: src/data.txt
            to: dst/data.txt
artifacts:
  primary: dst/data.txt
`
	writeRecipeManifest(t, root, manifest)

	report, err := ExecuteLifecycle(OperationBuild, root)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	dst := filepath.Join(root, "dst", "data.txt")
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("copy step did not produce file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("copied content = %q, want %q", string(data), "hello")
	}

	found := false
	for _, note := range report.Notes {
		if strings.Contains(note, "copied") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected copy note in report, got %v", report.Notes)
	}
}

func TestRecipeRunnerAssertFilePass(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)

	targetDir := filepath.Join(root, "out")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Pre-create the file to assert.
	if err := os.WriteFile(filepath.Join(targetDir, "result.bin"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	manifest := `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: out
      path: out
      type: component
  targets:
    macos:
      steps:
        - assert_file:
            path: out/result.bin
    linux:
      steps:
        - assert_file:
            path: out/result.bin
    darwin:
      steps:
        - assert_file:
            path: out/result.bin
artifacts:
  primary: out/result.bin
`
	writeRecipeManifest(t, root, manifest)

	_, err := ExecuteLifecycle(OperationBuild, root)
	if err != nil {
		t.Fatalf("assert_file pass case failed: %v", err)
	}
}

func TestRecipeRunnerAssertFileFail(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)

	if err := os.MkdirAll(filepath.Join(root, "out"), 0755); err != nil {
		t.Fatal(err)
	}

	manifest := `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: out
      path: out
      type: component
  targets:
    macos:
      steps:
        - assert_file:
            path: out/missing.bin
    linux:
      steps:
        - assert_file:
            path: out/missing.bin
    darwin:
      steps:
        - assert_file:
            path: out/missing.bin
artifacts:
  primary: out/result.bin
`
	writeRecipeManifest(t, root, manifest)

	_, err := ExecuteLifecycle(OperationBuild, root)
	if err == nil {
		t.Fatal("expected error for missing assert_file")
	}
	if !strings.Contains(err.Error(), "assert_file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecipeRunnerMissingTarget(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)

	if err := os.MkdirAll(filepath.Join(root, "child"), 0755); err != nil {
		t.Fatal(err)
	}

	manifest := `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: child
      path: child
      type: component
  targets:
    windows:
      steps:
        - exec:
            cwd: child
            argv: ["echo", "hello"]
artifacts:
  binary: my-app
`
	writeRecipeManifest(t, root, manifest)

	// On macOS/Linux, "windows" target won't match runtimePlatform().
	_, err := ExecuteLifecycle(OperationBuild, root)
	if err == nil {
		t.Fatal("expected error for missing target")
	}
	if !strings.Contains(err.Error(), "no recipe target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDryRunDoesNotExecute(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)

	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifest := `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: work
      path: work
      type: component
  targets:
    macos:
      steps:
        - exec:
            cwd: work
            argv: ["touch", "should-not-exist.txt"]
    linux:
      steps:
        - exec:
            cwd: work
            argv: ["touch", "should-not-exist.txt"]
    darwin:
      steps:
        - exec:
            cwd: work
            argv: ["touch", "should-not-exist.txt"]
artifacts:
  binary: my-app
`
	writeRecipeManifest(t, root, manifest)

	opts := BuildOptions{DryRun: true}
	report, err := ExecuteLifecycle(OperationBuild, root, opts)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	// Verify the exec step was NOT actually run.
	if _, err := os.Stat(filepath.Join(workDir, "should-not-exist.txt")); err == nil {
		t.Fatal("dry run should not have created the file")
	}

	// Verify the report contains a dry run note.
	found := false
	for _, note := range report.Notes {
		if strings.Contains(note, "dry run") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected dry run note, got %v", report.Notes)
	}
}
