package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/organic-programming/sophia-who/pkg/identity"
)

func TestVersionCommand(t *testing.T) {
	code := Run([]string{"version"}, "0.1.0-test")
	if code != 0 {
		t.Errorf("version returned %d, want 0", code)
	}
}

func TestHelpCommand(t *testing.T) {
	for _, cmd := range []string{"help", "--help", "-h"} {
		code := Run([]string{cmd}, "0.1.0-test")
		if code != 0 {
			t.Errorf("%s returned %d, want 0", cmd, code)
		}
	}
}

func TestRunWhoListThroughTransportChain(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)

	seedTransportHolon(t, root, transportHolonSeed{
		dirName:    "sophia-who",
		givenName:  "Sophia",
		familyName: "Who?",
		aliases:    []string{"who", "sophia"},
		lang:       "go",
	})
	seedTransportHolon(t, root, transportHolonSeed{
		dirName:    "atlas",
		binaryName: "atlas",
		givenName:  "atlas",
		familyName: "Holon",
		aliases:    []string{"atlas"},
		lang:       "go",
	})

	code := Run([]string{"who", "list", "holons"}, "0.1.0-test")
	if code != 0 {
		t.Fatalf("who list returned %d, want 0", code)
	}
}

func TestRunPromotedListThroughSophiaWho(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)

	seedTransportHolon(t, root, transportHolonSeed{
		dirName:    "sophia-who",
		givenName:  "Sophia",
		familyName: "Who?",
		aliases:    []string{"who", "sophia"},
		lang:       "go",
	})

	code := Run([]string{"list", "holons"}, "0.1.0-test")
	if code != 0 {
		t.Fatalf("list returned %d, want 0", code)
	}
}

func TestRunNativeShowCommand(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)

	seedTransportHolon(t, root, transportHolonSeed{
		dirName:    "sophia-who",
		givenName:  "Sophia",
		familyName: "Who?",
		aliases:    []string{"who", "sophia"},
		lang:       "go",
	})

	output := captureStdout(t, func() {
		code := Run([]string{"show", "transport-test-sophia-who"}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("show returned %d, want 0", code)
		}
	})

	if !strings.Contains(output, "Sophia Who?") {
		t.Fatalf("show output missing identity name: %q", output)
	}
	if !strings.Contains(output, "holon.yaml") {
		t.Fatalf("show output missing manifest path: %q", output)
	}
}

func TestRunNativeNewCommandJSON(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)

	output := captureStdout(t, func() {
		code := Run([]string{
			"new",
			"--json",
			`{"given_name":"Alpha","family_name":"Builder","motto":"Builds holons.","composer":"test","lang":"go","aliases":["alpha"]}`,
		}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("new returned %d, want 0", code)
		}
	})

	createdPath := filepath.Join(root, "holons", "alpha-builder", identity.ManifestFileName)
	if _, err := os.Stat(createdPath); err != nil {
		t.Fatalf("created holon manifest missing: %v", err)
	}
	if !strings.Contains(output, "Identity created") {
		t.Fatalf("new output missing creation message: %q", output)
	}
	if !strings.Contains(output, "Alpha Builder") {
		t.Fatalf("new output missing identity name: %q", output)
	}
}

func TestMapHolonCommandToRPC(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantInput  string
		wantErr    bool
	}{
		{
			name:       "list default",
			args:       []string{"list"},
			wantMethod: "ListIdentities",
			wantInput:  "{}",
		},
		{
			name:       "list root dir",
			args:       []string{"list", "holons"},
			wantMethod: "ListIdentities",
			wantInput:  `{"rootDir":"holons"}`,
		},
		{
			name:       "show uuid",
			args:       []string{"show", "abc123"},
			wantMethod: "ShowIdentity",
			wantInput:  `{"uuid":"abc123"}`,
		},
		{
			name:       "new with json input",
			args:       []string{"new", `{"givenName":"Alpha"}`},
			wantMethod: "CreateIdentity",
			wantInput:  `{"givenName":"Alpha"}`,
		},
		{
			name:       "custom method passthrough",
			args:       []string{"ListIdentities"},
			wantMethod: "ListIdentities",
			wantInput:  "{}",
		},
		{
			name:    "show missing uuid",
			args:    []string{"show"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method, input, err := mapHolonCommandToRPC(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("mapHolonCommandToRPC returned error: %v", err)
			}
			if method != tc.wantMethod {
				t.Fatalf("method = %q, want %q", method, tc.wantMethod)
			}
			if input != tc.wantInput {
				t.Fatalf("input = %q, want %q", input, tc.wantInput)
			}
		})
	}
}

func TestDiscoverCommand(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)
	seedTransportHolon(t, root, transportHolonSeed{
		dirName:    "who",
		binaryName: "who",
		givenName:  "who",
		familyName: "Holon",
		aliases:    []string{"who"},
		lang:       "go",
	})
	seedTransportHolon(t, root, transportHolonSeed{
		dirName:    "atlas",
		binaryName: "atlas",
		givenName:  "atlas",
		familyName: "Holon",
		aliases:    []string{"atlas"},
		lang:       "rust",
	})

	output := captureStdout(t, func() {
		code := Run([]string{"discover"}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("discover returned %d, want 0", code)
		}
	})

	if !strings.Contains(output, "LANG") {
		t.Fatalf("discover output missing LANG column: %q", output)
	}
	if !strings.Contains(output, "who Holon") {
		t.Fatalf("discover output missing who holon row: %q", output)
	}
	if !strings.Contains(output, "atlas Holon") {
		t.Fatalf("discover output missing atlas holon row: %q", output)
	}
	// Verify relative path appears in the who row (tabwriter converts tabs to spaces)
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "who Holon") {
			if !strings.Contains(line, "who") {
				t.Fatalf("who row missing relative path: %q", line)
			}
			break
		}
	}
	if !strings.Contains(output, "local") {
		t.Fatalf("discover output missing origin: %q", output)
	}
}

func TestDiscoverCommandJSONFormat(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)
	seedTransportHolon(t, root, transportHolonSeed{
		dirName:    "who",
		binaryName: "who",
		givenName:  "who",
		familyName: "Holon",
		aliases:    []string{"who"},
		lang:       "go",
	})
	seedTransportHolon(t, root, transportHolonSeed{
		dirName:    "atlas",
		binaryName: "atlas",
		givenName:  "atlas",
		familyName: "Holon",
		aliases:    []string{"atlas"},
		lang:       "rust",
	})

	output := captureStdout(t, func() {
		code := Run([]string{"--format", "json", "discover"}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("discover --format json returned %d, want 0", code)
		}
	})

	var payload struct {
		Entries []struct {
			UUID         string `json:"uuid"`
			GivenName    string `json:"given_name"`
			FamilyName   string `json:"family_name"`
			Lang         string `json:"lang"`
			Clade        string `json:"clade"`
			Status       string `json:"status"`
			RelativePath string `json:"relative_path"`
			Origin       string `json:"origin"`
		} `json:"entries"`
		PathBinaries []string `json:"path_binaries"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("discover json output is invalid: %v\noutput=%s", err, output)
	}
	if len(payload.Entries) < 2 {
		t.Fatalf("entries = %d, want at least 2", len(payload.Entries))
	}

	foundWho := false
	for _, entry := range payload.Entries {
		if entry.GivenName != "who" {
			continue
		}
		foundWho = true
		if entry.Lang != "go" {
			t.Fatalf("who lang = %q, want %q", entry.Lang, "go")
		}
		if entry.Origin != "local" {
			t.Fatalf("who origin = %q, want %q", entry.Origin, "local")
		}
		if entry.RelativePath != "who" {
			t.Fatalf("who relative_path = %q, want %q", entry.RelativePath, "who")
		}
	}
	if !foundWho {
		t.Fatalf("who entry not found in json output: %s", output)
	}
}

func TestDiscoverCommandIncludesCachedAndInstalledHolons(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)

	seedTransportHolon(t, root, transportHolonSeed{
		dirName:    "who",
		binaryName: "who",
		givenName:  "who",
		familyName: "Holon",
		aliases:    []string{"who"},
		lang:       "go",
	})

	runtimeHome := filepath.Join(root, "runtime")
	t.Setenv("OPPATH", runtimeHome)
	t.Setenv("OPBIN", filepath.Join(runtimeHome, "bin"))

	cacheDir := filepath.Join(runtimeHome, "cache", "github.com", "example", "cached-holon@v1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cachedID := identity.Identity{
		UUID:        "cached-test-holon",
		GivenName:   "Cached",
		FamilyName:  "Holon",
		Motto:       "Cached test.",
		Composer:    "test",
		Clade:       "deterministic/pure",
		Status:      "draft",
		Born:        "2026-03-07",
		Aliases:     []string{"cached"},
		GeneratedBy: "test",
		Lang:        "go",
	}
	cachedManifest := fmt.Sprintf("%s\nkind: native\nbuild:\n  runner: go-module\nartifacts:\n  binary: .op/build/bin/cached-holon\n", manifestIdentityPrefix(cachedID))
	if err := os.WriteFile(filepath.Join(cacheDir, identity.ManifestFileName), []byte(cachedManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(runtimeHome, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	installedPath := filepath.Join(runtimeHome, "bin", "installed-holon")
	if err := os.WriteFile(installedPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		code := Run([]string{"discover"}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("discover returned %d, want 0", code)
		}
	})

	if !strings.Contains(output, "cached") {
		t.Fatalf("discover output missing cached holon: %q", output)
	}
	if !strings.Contains(output, "In $OPBIN:") {
		t.Fatalf("discover output missing $OPBIN section: %q", output)
	}
	if !strings.Contains(output, "installed-holon") {
		t.Fatalf("discover output missing installed binary: %q", output)
	}
}

func TestEnvCommand(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)

	for _, dir := range []string{"holons", "examples", "recipes"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("OPPATH", filepath.Join(root, ".op-home"))
	t.Setenv("OPBIN", filepath.Join(root, ".op-home", "bin"))

	output := captureStdout(t, func() {
		code := Run([]string{"env"}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("env returned %d, want 0", code)
		}
	})

	if !strings.Contains(output, "OPPATH="+filepath.Join(root, ".op-home")) {
		t.Fatalf("env output missing OPPATH: %q", output)
	}
	if !strings.Contains(output, "OPBIN="+filepath.Join(root, ".op-home", "bin")) {
		t.Fatalf("env output missing OPBIN: %q", output)
	}
	if !strings.Contains(output, "ROOTS=holons, examples, recipes") {
		t.Fatalf("env output missing roots: %q", output)
	}
}

func TestEnvCommandInitAndShell(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)

	t.Setenv("OPPATH", filepath.Join(root, ".runtime"))
	t.Setenv("OPBIN", filepath.Join(root, ".runtime", "bin"))

	output := captureStdout(t, func() {
		code := Run([]string{"env", "--init", "--shell"}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("env --init --shell returned %d, want 0", code)
		}
	})

	if _, err := os.Stat(filepath.Join(root, ".runtime")); err != nil {
		t.Fatalf("runtime home missing after init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".runtime", "bin")); err != nil {
		t.Fatalf("opbin missing after init: %v", err)
	}
	if !strings.Contains(output, `export OPPATH="${OPPATH:-$HOME/.op}"`) {
		t.Fatalf("env --shell output missing OPPATH export: %q", output)
	}
	if !strings.Contains(output, `export PATH="$OPBIN:$PATH"`) {
		t.Fatalf("env --shell output missing PATH export: %q", output)
	}
}

func TestInstallCommand(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not available")
	}

	root := t.TempDir()
	chdirForTest(t, root)
	t.Setenv("OPPATH", filepath.Join(root, ".runtime"))
	t.Setenv("OPBIN", filepath.Join(root, ".runtime", "bin"))

	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n\ngo 1.24.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", "demo", "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "holon.yaml"), []byte("schema: holon/v0\nkind: native\nbuild:\n  runner: go-module\nrequires:\n  commands: [go]\n  files: [go.mod]\nartifacts:\n  binary: .op/build/bin/demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		code := Run([]string{"install", dir}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("install returned %d, want 0", code)
		}
	})

	installed := filepath.Join(root, ".runtime", "bin", "demo")
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("installed binary missing: %v", err)
	}
	if !strings.Contains(output, "Installed: "+installed) {
		t.Fatalf("install output missing installed path: %q", output)
	}
}

func TestInstallCommandNoBuildFailsWhenArtifactMissing(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)
	t.Setenv("OPPATH", filepath.Join(root, ".runtime"))
	t.Setenv("OPBIN", filepath.Join(root, ".runtime", "bin"))

	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n\ngo 1.24.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "holon.yaml"), []byte("schema: holon/v0\nkind: native\nbuild:\n  runner: go-module\nrequires:\n  commands: [go]\n  files: [go.mod]\nartifacts:\n  binary: .op/build/bin/demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stderr := captureStderr(t, func() {
		code := Run([]string{"install", "--no-build", dir}, "0.1.0-test")
		if code != 1 {
			t.Fatalf("install --no-build returned %d, want 1", code)
		}
	})

	if !strings.Contains(stderr, "artifact missing") {
		t.Fatalf("stderr missing missing-artifact error: %q", stderr)
	}
}

func TestUninstallCommand(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)
	t.Setenv("OPPATH", filepath.Join(root, ".runtime"))
	t.Setenv("OPBIN", filepath.Join(root, ".runtime", "bin"))

	if err := os.MkdirAll(filepath.Join(root, ".runtime", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(root, ".runtime", "bin", "demo")
	if err := os.WriteFile(installed, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	code := Run([]string{"uninstall", "demo"}, "0.1.0-test")
	if code != 0 {
		t.Fatalf("uninstall returned %d, want 0", code)
	}
	if _, err := os.Stat(installed); !os.IsNotExist(err) {
		t.Fatalf("installed binary still exists: %v", err)
	}
}

func TestModInitCommandInfersSlugFromHolonYAML(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)

	id := identity.New()
	id.GivenName = "Alpha"
	id.FamilyName = "Builder"
	id.Motto = "Builds holons."
	id.Composer = "test"
	id.Clade = "deterministic/pure"
	id.Status = "draft"
	id.Lang = "go"
	if err := identity.WriteHolonYAML(id, filepath.Join(root, identity.ManifestFileName)); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		code := Run([]string{"mod", "init"}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("mod init returned %d, want 0", code)
		}
	})

	data, err := os.ReadFile(filepath.Join(root, "holon.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "holon alpha-builder") {
		t.Fatalf("holon.mod missing inferred slug: %s", string(data))
	}
	if !strings.Contains(output, "created") {
		t.Fatalf("mod init output missing create message: %q", output)
	}
}

func TestModCommandsUseOPPATHCache(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)
	runtimeHome := filepath.Join(root, ".runtime")
	t.Setenv("OPPATH", runtimeHome)
	t.Setenv("OPBIN", filepath.Join(runtimeHome, "bin"))

	if err := os.WriteFile(filepath.Join(root, "holon.mod"), []byte("holon alpha-builder\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	depPath := "github.com/example/dep"
	version := "v1.0.0"
	cacheDir := filepath.Join(runtimeHome, "cache", depPath+"@"+version)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cachedID := identity.New()
	cachedID.GivenName = "Cached"
	cachedID.FamilyName = "Dep"
	cachedID.Motto = "Cached dependency."
	cachedID.Composer = "test"
	cachedID.Clade = "deterministic/pure"
	cachedID.Status = "draft"
	cachedID.Lang = "go"
	if err := identity.WriteHolonYAML(cachedID, filepath.Join(cacheDir, identity.ManifestFileName)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "holon.mod"), []byte("holon github.com/example/dep\n\nrequire (\n    github.com/example/subdep v0.2.0\n)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	addOutput := captureStdout(t, func() {
		code := Run([]string{"mod", "add", depPath, version}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("mod add returned %d, want 0", code)
		}
	})
	if !strings.Contains(addOutput, depPath+"@"+version) {
		t.Fatalf("mod add output missing dependency: %q", addOutput)
	}

	listOutput := captureStdout(t, func() {
		code := Run([]string{"mod", "list"}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("mod list returned %d, want 0", code)
		}
	})
	if !strings.Contains(listOutput, depPath) {
		t.Fatalf("mod list output missing dependency: %q", listOutput)
	}

	graphOutput := captureStdout(t, func() {
		code := Run([]string{"mod", "graph"}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("mod graph returned %d, want 0", code)
		}
	})
	if !strings.Contains(graphOutput, "github.com/example/subdep@v0.2.0") {
		t.Fatalf("mod graph output missing transitive dependency: %q", graphOutput)
	}

	pullOutput := captureStdout(t, func() {
		code := Run([]string{"mod", "pull"}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("mod pull returned %d, want 0", code)
		}
	})
	if !strings.Contains(pullOutput, depPath+"@"+version) {
		t.Fatalf("mod pull output missing fetched dependency: %q", pullOutput)
	}

	if err := os.WriteFile(filepath.Join(root, "holon.sum"), []byte(depPath+" "+version+" h1:keep\n"+"github.com/example/stale v9.9.9 h1:drop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tidyOutput := captureStdout(t, func() {
		code := Run([]string{"mod", "tidy"}, "0.1.0-test")
		if code != 0 {
			t.Fatalf("mod tidy returned %d, want 0", code)
		}
	})
	if !strings.Contains(tidyOutput, "updated") {
		t.Fatalf("mod tidy output missing update message: %q", tidyOutput)
	}

	sumData, err := os.ReadFile(filepath.Join(root, "holon.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sumData), "github.com/example/stale") {
		t.Fatalf("holon.sum still contains stale dependency: %s", string(sumData))
	}
	if !strings.Contains(string(sumData), depPath+" "+version) {
		t.Fatalf("holon.sum missing kept dependency: %s", string(sumData))
	}
}

func TestDispatchUnknownHolon(t *testing.T) {
	code := Run([]string{"nonexistent-holon", "some-command"}, "0.1.0-test")
	if code != 1 {
		t.Errorf("dispatch (unknown) returned %d, want 1", code)
	}
}

func TestFlagValue(t *testing.T) {
	args := []string{"--name", "Test", "--lang", "rust", "--verbose"}

	if v := flagValue(args, "--name"); v != "Test" {
		t.Errorf("flagValue(--name) = %q, want %q", v, "Test")
	}
	if v := flagValue(args, "--lang"); v != "rust" {
		t.Errorf("flagValue(--lang) = %q, want %q", v, "rust")
	}
	if v := flagValue(args, "--missing"); v != "" {
		t.Errorf("flagValue(--missing) = %q, want empty", v)
	}
	// --verbose has no value after it
	if v := flagValue(args, "--verbose"); v != "" {
		t.Errorf("flagValue(--verbose) = %q, want empty", v)
	}
}

func TestFlagOrDefault(t *testing.T) {
	args := []string{"--name", "Test"}

	if v := flagOrDefault(args, "--name", "fallback"); v != "Test" {
		t.Errorf("flagOrDefault(--name) = %q, want %q", v, "Test")
	}
	if v := flagOrDefault(args, "--missing", "fallback"); v != "fallback" {
		t.Errorf("flagOrDefault(--missing) = %q, want %q", v, "fallback")
	}
}

func TestParseGlobalFormat(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantFormat Format
		wantArgs   []string
		wantErr    bool
	}{
		{
			name:       "default format",
			args:       []string{"who", "list"},
			wantFormat: FormatText,
			wantArgs:   []string{"who", "list"},
		},
		{
			name:       "long flag",
			args:       []string{"--format", "json", "who", "list"},
			wantFormat: FormatJSON,
			wantArgs:   []string{"who", "list"},
		},
		{
			name:       "short flag",
			args:       []string{"-f", "json", "who", "list"},
			wantFormat: FormatJSON,
			wantArgs:   []string{"who", "list"},
		},
		{
			name:       "inline long flag",
			args:       []string{"--format=text", "who", "list"},
			wantFormat: FormatText,
			wantArgs:   []string{"who", "list"},
		},
		{
			name:       "inline short flag",
			args:       []string{"-f=text", "who", "list"},
			wantFormat: FormatText,
			wantArgs:   []string{"who", "list"},
		},
		{
			name:       "flag after command is not global",
			args:       []string{"who", "-f", "json", "list"},
			wantFormat: FormatText,
			wantArgs:   []string{"who", "-f", "json", "list"},
		},
		{
			name:    "invalid format",
			args:    []string{"--format", "yaml", "who", "list"},
			wantErr: true,
		},
		{
			name:    "missing format value",
			args:    []string{"-f"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotFormat, gotArgs, err := parseGlobalFormat(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGlobalFormat returned error: %v", err)
			}
			if gotFormat != tc.wantFormat {
				t.Fatalf("format = %q, want %q", gotFormat, tc.wantFormat)
			}
			if len(gotArgs) != len(tc.wantArgs) {
				t.Fatalf("args length = %d, want %d", len(gotArgs), len(tc.wantArgs))
			}
			for i := range gotArgs {
				if gotArgs[i] != tc.wantArgs[i] {
					t.Fatalf("args[%d] = %q, want %q", i, gotArgs[i], tc.wantArgs[i])
				}
			}
		})
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
		_ = w.Close()
		_ = r.Close()
	}()

	fn()

	_ = w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	return buf.String()
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = origStderr
		_ = w.Close()
		_ = r.Close()
	}()

	fn()

	_ = w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return buf.String()
}
