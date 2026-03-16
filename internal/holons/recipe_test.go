package holons

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func writeRecipeManifest(t *testing.T, dir, yaml string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeProtoRecipeManifest(t *testing.T, root, dir, body string) {
	t.Helper()

	writeSharedHolonManifestProto(t, root)
	if err := os.MkdirAll(filepath.Join(dir, "v1"), 0o755); err != nil {
		t.Fatal(err)
	}

	proto := fmt.Sprintf(`syntax = "proto3";

package test.v1;

import "holons/v1/manifest.proto";

option (holons.v1.manifest) = {
  identity: {
    schema: "holon/v1"
    uuid: "%s-uuid"
    given_name: "Proto"
    family_name: "Recipe"
    motto: "Proto-backed recipe test holon."
    composer: "test"
    clade: "deterministic/pure"
    status: "draft"
    born: "2026-03-15"
  }
  lineage: {
    reproduction: "manual"
    generated_by: "test"
  }
%s
};
`, filepath.Base(dir), strings.TrimSpace(body))

	if err := os.WriteFile(filepath.Join(dir, "v1", "holon.proto"), []byte(proto), 0o644); err != nil {
		t.Fatal(err)
	}
}

func withBundleSigningTestSeams(t *testing.T, hostPlatform string, runner func(dir, artifactRef string) (string, error)) {
	t.Helper()

	prevHostPlatform := bundleSigningHostPlatform
	prevRunner := runBundleCodesign
	bundleSigningHostPlatform = func() string { return hostPlatform }
	if runner != nil {
		runBundleCodesign = runner
	}
	t.Cleanup(func() {
		bundleSigningHostPlatform = prevHostPlatform
		runBundleCodesign = prevRunner
	})
}

func writeFakeCodesignCommand(t *testing.T, dir string) {
	t.Helper()

	path := filepath.Join(dir, "codesign")
	data := []byte("#!/bin/sh\nbundle=\"$5\"\nmkdir -p \"$bundle/Contents/_CodeSignature\"\nprintf 'signed\\n' > \"$bundle/Contents/_CodeSignature/CodeResources\"\n")
	mode := os.FileMode(0o755)
	if runtimePlatform() == "windows" {
		path += ".bat"
		data = []byte("@echo off\r\nset bundle=%~5\r\nmkdir \"%bundle%\\Contents\\_CodeSignature\" >nul 2>nul\r\necho signed> \"%bundle%\\Contents\\_CodeSignature\\CodeResources\"\r\n")
		mode = 0o644
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func writeBundleFixture(t *testing.T, root, rel string) {
	t.Helper()

	binaryPath := filepath.Join(root, filepath.FromSlash(rel), "Contents", "MacOS", "Demo")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func hasEntryContaining(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func TestLoadManifestAcceptsCompositeRecipe(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "child-a"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "child-a", ManifestFileName), []byte(`schema: holon/v0
kind: native
build:
  runner: go-module
requires:
  commands: [go]
  files: [go.mod]
artifacts:
  binary: child-a
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "child-b"), 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
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
  primary: child-b/my-app
`)

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
	if got := loaded.Manifest.Build.Defaults.Target; got != "macos" {
		t.Fatalf("defaults.target = %q, want %q", got, "macos")
	}
}

func TestLoadManifestNormalizesDarwinRecipeTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "work"), 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
kind: composite
build:
  runner: recipe
  defaults:
    target: darwin
    mode: DEBUG
  members:
    - id: work
      path: work
      type: component
  targets:
    darwin:
      steps:
        - exec:
            cwd: work
            argv: ["echo", "hello"]
artifacts:
  primary: work/app-debug
`)

	loaded, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if got := loaded.Manifest.Build.Defaults.Target; got != "macos" {
		t.Fatalf("defaults.target = %q, want %q", got, "macos")
	}
	if got := loaded.Manifest.Build.Defaults.Mode; got != "debug" {
		t.Fatalf("defaults.mode = %q, want %q", got, "debug")
	}
	if _, ok := loaded.Manifest.Build.Targets["macos"]; !ok {
		t.Fatalf("expected normalized macos target, got %v", loaded.Manifest.Build.Targets)
	}
	if got := loaded.Manifest.Artifacts.Primary; got != "work/app-debug" {
		t.Fatalf("artifacts.primary = %q, want %q", got, "work/app-debug")
	}
}

func TestLoadManifestAcceptsProtoBackedCompositeRecipe(t *testing.T) {
	root := t.TempDir()

	yamlDir := filepath.Join(root, "yaml-recipe")
	if err := os.MkdirAll(filepath.Join(yamlDir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(yamlDir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(yamlDir, "app", "source.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(yamlDir, "app", "output.txt"), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRecipeManifest(t, yamlDir, `schema: holon/v0
kind: composite
build:
  runner: recipe
  defaults:
    target: macos
    mode: debug
  members:
    - id: child
      path: child
      type: holon
    - id: app
      path: app
      type: component
  targets:
    macos:
      steps:
        - build_member: child
        - exec:
            cwd: app
            argv: ["echo", "hello"]
        - copy:
            from: app/source.txt
            to: app/copied.txt
        - assert_file:
            path: app/output.txt
artifacts:
  primary: app/output.txt
`)

	protoDir := filepath.Join(root, "proto-recipe")
	if err := os.MkdirAll(filepath.Join(protoDir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(protoDir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protoDir, "app", "source.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protoDir, "app", "output.txt"), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeProtoRecipeManifest(t, root, protoDir, `
  description: "Proto-backed composite recipe."
  kind: "composite"
  build: {
    runner: "recipe"
    defaults: {
      target: "macos"
      mode: "debug"
    }
    members: {
      id: "child"
      path: "child"
      type: "holon"
    }
    members: {
      id: "app"
      path: "app"
      type: "component"
    }
    targets: {
      key: "macos"
      value: {
        steps: { build_member: "child" }
        steps: { exec: { cwd: "app" argv: ["echo", "hello"] } }
        steps: { copy: { from: "app/source.txt" to: "app/copied.txt" } }
        steps: { assert_file: { path: "app/output.txt" } }
      }
    }
  }
  artifacts: {
    primary: "app/output.txt"
  }
`)

	yamlManifest, err := LoadManifest(yamlDir)
	if err != nil {
		t.Fatalf("LoadManifest(yaml) failed: %v", err)
	}
	protoManifest, err := LoadManifest(protoDir)
	if err != nil {
		t.Fatalf("LoadManifest(proto) failed: %v", err)
	}

	if !reflect.DeepEqual(protoManifest.Manifest.Build, yamlManifest.Manifest.Build) {
		t.Fatalf("proto build = %#v, want %#v", protoManifest.Manifest.Build, yamlManifest.Manifest.Build)
	}
}

func TestLoadManifestNormalizesProtoBackedDarwinRecipeTargets(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "proto-darwin-recipe")
	if err := os.MkdirAll(filepath.Join(dir, "work"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeProtoRecipeManifest(t, root, dir, `
  kind: "composite"
  build: {
    runner: "recipe"
    defaults: {
      target: "DARWIN"
      mode: "DEBUG"
    }
    members: {
      id: "work"
      path: "work"
      type: "component"
    }
    targets: {
      key: "darwin"
      value: {
        steps: {
          exec: {
            cwd: "work"
            argv: ["echo", "hello"]
          }
        }
      }
    }
  }
  artifacts: {
    primary: "work/app-debug"
  }
`)

	loaded, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if got := loaded.Manifest.Build.Defaults.Target; got != "macos" {
		t.Fatalf("defaults.target = %q, want %q", got, "macos")
	}
	if got := loaded.Manifest.Build.Defaults.Mode; got != "debug" {
		t.Fatalf("defaults.mode = %q, want %q", got, "debug")
	}
	if _, ok := loaded.Manifest.Build.Targets["macos"]; !ok {
		t.Fatalf("expected normalized macos target, got %v", loaded.Manifest.Build.Targets)
	}
}

func TestProtoRecipeValidationRejectsMissingMembers(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "proto-invalid-recipe")
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeProtoRecipeManifest(t, root, dir, `
  kind: "composite"
  build: {
    runner: "recipe"
    targets: {
      key: "macos"
      value: {
        steps: {
          assert_file: {
            path: "app/demo.app"
          }
        }
      }
    }
  }
  artifacts: {
    primary: "app/demo.app"
  }
`)

	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected error for missing proto recipe members")
	}
	if !strings.Contains(err.Error(), "recipe runner requires at least one member") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteLifecycleBuildProtoBackedRecipe(t *testing.T) {
	if _, err := execLookPath("go"); err != nil {
		t.Skip("go command not available")
	}

	root := t.TempDir()
	chdirForHolonTest(t, root)
	dir := filepath.Join(root, "proto-build-recipe")
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", "output.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	target := canonicalRuntimeTarget()
	writeProtoRecipeManifest(t, root, dir, fmt.Sprintf(`
  kind: "composite"
  build: {
    runner: "recipe"
    defaults: {
      target: %q
      mode: "debug"
    }
    members: {
      id: "app"
      path: "app"
      type: "component"
    }
    targets: {
      key: %q
      value: {
        steps: {
          exec: {
            cwd: "app"
            argv: ["go", "version"]
          }
        }
        steps: {
          assert_file: {
            path: "app/output.txt"
          }
        }
      }
    }
  }
  requires: {
    commands: ["go"]
  }
  artifacts: {
    primary: "app/output.txt"
  }
`, target, target))

	report, err := ExecuteLifecycle(OperationBuild, dir)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if report.Runner != RunnerRecipe {
		t.Fatalf("runner = %q, want %q", report.Runner, RunnerRecipe)
	}
	if !strings.HasSuffix(report.Manifest, filepath.ToSlash(filepath.Join("proto-build-recipe", "v1", "holon.proto"))) {
		t.Fatalf("manifest report = %q", report.Manifest)
	}
	if !hasEntryContaining(report.Commands, "go version") {
		t.Fatalf("expected go version command in report, got %v", report.Commands)
	}
	if !hasEntryContaining(report.Commands, "assert_file") {
		t.Fatalf("expected assert_file step in report, got %v", report.Commands)
	}
}

func TestRecipeValidationRejectsDuplicateNormalizedTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "work"), 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
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
            argv: ["echo", "macos"]
    darwin:
      steps:
        - exec:
            cwd: work
            argv: ["echo", "darwin"]
artifacts:
  primary: work/my-app
`)

	_, err := LoadManifest(root)
	if err == nil {
		t.Fatal("expected duplicate normalized target error")
	}
	if !strings.Contains(err.Error(), "duplicate recipe target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecipeValidationRejectsUnknownMemberRef(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "child"), 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
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
  primary: child/my-app
`)

	_, err := LoadManifest(root)
	if err == nil {
		t.Fatal("expected error for unknown member ref")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecipeValidationRejectsMultiActionStep(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "child"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "child", ManifestFileName), []byte(`schema: holon/v0
kind: native
build:
  runner: go-module
requires:
  commands: [go]
  files: [go.mod]
artifacts:
  binary: child
`), 0644); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: child
      path: child
      type: holon
  targets:
    macos:
      steps:
        - build_member: child
          exec:
            cwd: child
            argv: ["echo", "oops"]
artifacts:
  primary: child/my-app
`)

	_, err := LoadManifest(root)
	if err == nil {
		t.Fatal("expected multi-action step error")
	}
	if !strings.Contains(err.Error(), "exactly one action") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecipeBuildAllDryRunBuildsEachDeclaredTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app"), 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
kind: composite
platforms: [macos, ios-simulator]
build:
  runner: recipe
  defaults:
    mode: debug
  members:
    - id: app
      path: app
      type: component
  targets:
    macos:
      steps:
        - exec:
            cwd: app
            argv: ["echo", "macos"]
    ios-simulator:
      steps:
        - exec:
            cwd: app
            argv: ["echo", "ios-simulator"]
artifacts:
  primary: app/output.app
`)

	report, err := ExecuteLifecycle(OperationBuild, root, BuildOptions{
		Target: "all",
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("ExecuteLifecycle(build all) failed: %v", err)
	}

	if report.BuildTarget != "all" {
		t.Fatalf("report.BuildTarget = %q, want %q", report.BuildTarget, "all")
	}
	if report.Artifact != "" {
		t.Fatalf("report.Artifact = %q, want empty for aggregate builds", report.Artifact)
	}
	if len(report.Children) != 2 {
		t.Fatalf("len(report.Children) = %d, want 2", len(report.Children))
	}

	gotTargets := []string{report.Children[0].BuildTarget, report.Children[1].BuildTarget}
	wantTargets := []string{"macos", "ios-simulator"}
	if !slices.Equal(gotTargets, wantTargets) {
		t.Fatalf("child targets = %v, want %v", gotTargets, wantTargets)
	}
}

func TestRecipeValidationRejectsBuildMemberForComponent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "component"), 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: component
      path: component
      type: component
  targets:
    macos:
      steps:
        - build_member: component
artifacts:
  primary: component/my-app
`)

	_, err := LoadManifest(root)
	if err == nil {
		t.Fatal("expected component build_member error")
	}
	if !strings.Contains(err.Error(), "must reference a holon member") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecipeValidationRejectsExecWithoutArgv(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "work"), 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
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
            argv: []
artifacts:
  primary: work/my-app
`)

	_, err := LoadManifest(root)
	if err == nil {
		t.Fatal("expected empty argv error")
	}
	if !strings.Contains(err.Error(), "exec.argv") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecipeValidationAllowsParentRelativePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assemblies", "demo"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "shared"), 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, filepath.Join(root, "assemblies", "demo"), `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: shared
      path: ../../shared
      type: component
  targets:
    macos:
      steps:
        - exec:
            cwd: ../../shared
            argv: ["echo", "hello"]
        - copy:
            from: ../../shared/input.txt
            to: ../../shared/output.txt
        - assert_file:
            path: ../../shared/output.txt
artifacts:
  primary: ../../shared/output.txt
`)

	loaded, err := LoadManifest(filepath.Join(root, "assemblies", "demo"))
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if got := loaded.Manifest.Build.Members[0].Path; got != "../../shared" {
		t.Fatalf("member path = %q, want ../../shared", got)
	}
}

func TestRecipeRunnerExecStep(t *testing.T) {
	if runtimePlatform() == "windows" {
		t.Skip("touch not available on Windows test environment")
	}

	root := t.TempDir()
	chdirForHolonTest(t, root)
	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
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
artifacts:
  primary: work/proof.txt
`)

	report, err := ExecuteLifecycle(OperationBuild, root)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "proof.txt")); err != nil {
		t.Fatalf("exec step did not create proof file: %v", err)
	}
	if len(report.Commands) == 0 || !strings.Contains(report.Commands[0], "touch") {
		t.Fatalf("expected touch in commands, got %v", report.Commands)
	}
}

func TestRecipeRunnerBuildTargetOverride(t *testing.T) {
	if runtimePlatform() == "windows" {
		t.Skip("touch not available on Windows test environment")
	}

	root := t.TempDir()
	chdirForHolonTest(t, root)
	if err := os.MkdirAll(filepath.Join(root, "work"), 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
kind: composite
build:
  runner: recipe
  defaults:
    target: macos
  members:
    - id: work
      path: work
      type: component
  targets:
    macos:
      steps:
        - exec:
            cwd: work
            argv: ["touch", "macos.txt"]
    linux:
      steps:
        - exec:
            cwd: work
            argv: ["touch", "linux.txt"]
artifacts:
  primary: work/linux.txt
`)

	report, err := ExecuteLifecycle(OperationBuild, root, BuildOptions{Target: "linux"})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "work", "linux.txt")); err != nil {
		t.Fatalf("linux target file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "work", "macos.txt")); err == nil {
		t.Fatal("macos target file should not have been created")
	}
	if report.BuildTarget != "linux" {
		t.Fatalf("build target = %q, want linux", report.BuildTarget)
	}
	if !strings.HasSuffix(report.Artifact, "work/linux.txt") {
		t.Fatalf("artifact = %q, want linux artifact", report.Artifact)
	}
}

func TestRecipeRunnerCopyStep(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)
	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "data.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
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
artifacts:
  primary: dst/data.txt
`)

	report, err := ExecuteLifecycle(OperationBuild, root)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "dst", "data.txt"))
	if err != nil {
		t.Fatalf("copy step did not produce file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("copied content = %q, want hello", string(data))
	}
	if len(report.Commands) == 0 || !strings.Contains(report.Commands[0], "copy") {
		t.Fatalf("expected copy command, got %v", report.Commands)
	}
}

func TestRecipeRunnerAssertFilePass(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)
	if err := os.MkdirAll(filepath.Join(root, "out"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "result.bin"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
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
artifacts:
  primary: out/result.bin
`)

	if _, err := ExecuteLifecycle(OperationBuild, root); err != nil {
		t.Fatalf("assert_file pass case failed: %v", err)
	}
}

func TestRecipeRunnerAssertFileFail(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)
	if err := os.MkdirAll(filepath.Join(root, "out"), 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
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
artifacts:
  primary: out/result.bin
`)

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

	writeRecipeManifest(t, root, `schema: holon/v0
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
  primary: child/my-app
`)

	_, err := ExecuteLifecycle(OperationBuild, root)
	if err == nil {
		t.Fatal("expected error for missing target")
	}
	if !strings.Contains(err.Error(), "no recipe target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDryRunReportsPlanAndArtifact(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)
	if err := os.MkdirAll(filepath.Join(root, "work"), 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, root, `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: work
      path: work
      type: component
  targets:
    linux:
      steps:
        - exec:
            cwd: work
            argv: ["touch", "linux-release.txt"]
artifacts:
  primary: work/linux-release.txt
`)

	report, err := ExecuteLifecycle(OperationBuild, root, BuildOptions{
		Target: "linux",
		Mode:   "release",
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "work", "linux-release.txt")); err == nil {
		t.Fatal("dry run should not have created the file")
	}
	if len(report.Commands) == 0 || !strings.Contains(report.Commands[0], "touch") {
		t.Fatalf("expected planned command, got %v", report.Commands)
	}
	if report.Artifact == "" || !strings.HasSuffix(report.Artifact, "work/linux-release.txt") {
		t.Fatalf("artifact = %q, want linux release artifact", report.Artifact)
	}
	foundDryRun := false
	for _, note := range report.Notes {
		if strings.Contains(note, "dry run") {
			foundDryRun = true
		}
	}
	if !foundDryRun {
		t.Fatalf("expected dry run note, got %v", report.Notes)
	}
}

func TestRecipeRunnerPropagatesBuildContextToChildren(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)
	childDir := filepath.Join(root, "child")
	if err := os.MkdirAll(filepath.Join(childDir, "work"), 0755); err != nil {
		t.Fatal(err)
	}

	writeRecipeManifest(t, childDir, `schema: holon/v0
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
            argv: ["touch", "macos-release.txt"]
    linux:
      steps:
        - exec:
            cwd: work
            argv: ["touch", "linux-release.txt"]
artifacts:
  primary: work/linux-release.txt
`)

	writeRecipeManifest(t, root, `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: child
      path: child
      type: holon
  targets:
    linux:
      steps:
        - build_member: child
artifacts:
  primary: child/work/linux-release.txt
`)

	report, err := ExecuteLifecycle(OperationBuild, root, BuildOptions{
		Target: "linux",
		Mode:   "release",
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if len(report.Children) != 1 {
		t.Fatalf("children = %d, want 1", len(report.Children))
	}
	child := report.Children[0]
	if child.BuildTarget != "linux" {
		t.Fatalf("child build target = %q, want linux", child.BuildTarget)
	}
	if child.BuildMode != "release" {
		t.Fatalf("child build mode = %q, want release", child.BuildMode)
	}
	if !strings.HasSuffix(child.Artifact, "child/work/linux-release.txt") {
		t.Fatalf("child artifact = %q, want linux release artifact", child.Artifact)
	}
	foundLinuxCommand := false
	for _, command := range child.Commands {
		if strings.Contains(command, "linux-release.txt") {
			foundLinuxCommand = true
		}
	}
	if !foundLinuxCommand {
		t.Fatalf("expected child commands to reflect propagated target/mode, got %v", child.Commands)
	}
}

func TestRecipeRunnerAutoSignsBundleBeforeAssertFile(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)
	writeBundleFixture(t, root, "app/Demo.app")

	toolDir := t.TempDir()
	writeFakeCodesignCommand(t, toolDir)
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	withBundleSigningTestSeams(t, "darwin", nil)

	writeRecipeManifest(t, root, `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: app
      path: app
      type: component
  targets:
    macos:
      steps:
        - assert_file:
            path: app/Demo.app/Contents/_CodeSignature/CodeResources
artifacts:
  primary: app/Demo.app
`)

	report, err := ExecuteLifecycle(OperationBuild, root, BuildOptions{Target: "macos"})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "app", "Demo.app", "Contents", "_CodeSignature", "CodeResources")); err != nil {
		t.Fatalf("signed bundle missing CodeResources: %v", err)
	}
	if len(report.Commands) == 0 || report.Commands[0] != "codesign --force --deep --sign - app/Demo.app" {
		t.Fatalf("expected first command to be codesign, got %v", report.Commands)
	}
	if !hasEntryContaining(report.Notes, "signed (ad-hoc): app/Demo.app") {
		t.Fatalf("expected signing note, got %v", report.Notes)
	}
}

func TestRecipeRunnerSkipsBundleSigningWhenNoSignSet(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)
	writeBundleFixture(t, root, "app/Demo.app")

	withBundleSigningTestSeams(t, "darwin", func(dir, artifactRef string) (string, error) {
		t.Fatalf("runBundleCodesign should not be called for --no-sign")
		return "", nil
	})

	writeRecipeManifest(t, root, `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: app
      path: app
      type: component
  targets:
    macos:
      steps:
        - assert_file:
            path: app/Demo.app
artifacts:
  primary: app/Demo.app
`)

	report, err := ExecuteLifecycle(OperationBuild, root, BuildOptions{Target: "macos", NoSign: true})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if hasEntryContaining(report.Commands, "codesign --force --deep --sign - app/Demo.app") {
		t.Fatalf("did not expect codesign command, got %v", report.Commands)
	}
	if !hasEntryContaining(report.Notes, "skip signing (--no-sign): app/Demo.app") {
		t.Fatalf("expected no-sign note, got %v", report.Notes)
	}
}

func TestRecipeRunnerSkipsBundleSigningOffDarwin(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)
	writeBundleFixture(t, root, "app/Demo.app")

	withBundleSigningTestSeams(t, "linux", func(dir, artifactRef string) (string, error) {
		t.Fatalf("runBundleCodesign should not be called off Darwin")
		return "", nil
	})

	writeRecipeManifest(t, root, `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: app
      path: app
      type: component
  targets:
    macos:
      steps:
        - assert_file:
            path: app/Demo.app
artifacts:
  primary: app/Demo.app
`)

	report, err := ExecuteLifecycle(OperationBuild, root, BuildOptions{Target: "macos"})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if !hasEntryContaining(report.Notes, "skip signing: not on macOS: app/Demo.app") {
		t.Fatalf("expected not-on-macOS note, got %v", report.Notes)
	}
}

func TestRecipeRunnerDoesNotAttemptSigningForNonBundleArtifacts(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)
	if err := os.MkdirAll(filepath.Join(root, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "result.bin"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	withBundleSigningTestSeams(t, "darwin", func(dir, artifactRef string) (string, error) {
		t.Fatalf("runBundleCodesign should not be called for non-bundle artifacts")
		return "", nil
	})

	writeRecipeManifest(t, root, `schema: holon/v0
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
artifacts:
  primary: out/result.bin
`)

	report, err := ExecuteLifecycle(OperationBuild, root, BuildOptions{Target: "macos"})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if hasEntryContaining(report.Commands, "codesign") {
		t.Fatalf("did not expect codesign command, got %v", report.Commands)
	}
	if hasEntryContaining(report.Notes, "sign") {
		t.Fatalf("did not expect signing notes, got %v", report.Notes)
	}
}

func TestRecipeRunnerDryRunPlansBundleSigningWithoutExecuting(t *testing.T) {
	root := t.TempDir()
	chdirForHolonTest(t, root)
	writeBundleFixture(t, root, "app/Demo.app")

	called := false
	withBundleSigningTestSeams(t, "darwin", func(dir, artifactRef string) (string, error) {
		called = true
		return "", fmt.Errorf("unexpected bundle signing call for %s", artifactRef)
	})

	writeRecipeManifest(t, root, `schema: holon/v0
kind: composite
build:
  runner: recipe
  members:
    - id: app
      path: app
      type: component
  targets:
    macos:
      steps:
        - assert_file:
            path: app/Demo.app/Contents/_CodeSignature/CodeResources
artifacts:
  primary: app/Demo.app
`)

	report, err := ExecuteLifecycle(OperationBuild, root, BuildOptions{Target: "macos", DryRun: true})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if called {
		t.Fatal("runBundleCodesign was called during dry run")
	}
	if hasEntryContaining(report.Notes, "signed (ad-hoc):") {
		t.Fatalf("did not expect success note in dry run, got %v", report.Notes)
	}
	if !hasEntryContaining(report.Commands, "codesign --force --deep --sign - app/Demo.app") {
		t.Fatalf("expected planned codesign command, got %v", report.Commands)
	}
	if _, err := os.Stat(filepath.Join(root, "app", "Demo.app", "Contents", "_CodeSignature", "CodeResources")); !os.IsNotExist(err) {
		t.Fatalf("dry run should not have created CodeResources: %v", err)
	}
}
