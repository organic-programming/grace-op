package holons

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/organic-programming/grace-op/internal/progress"
)

func TestCargoRunnerDryRunBuild(t *testing.T) {
	root := t.TempDir()
	writeRunnerManifest(t, root, "schema: holon/v0\nkind: native\nbuild:\n  runner: cargo\nartifacts:\n  binary: cargo-demo\n")

	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	var report Report
	err = (cargoRunner{}).build(manifest, BuildContext{
		Target:   canonicalRuntimeTarget(),
		Mode:     buildModeDebug,
		DryRun:   true,
		Progress: progress.Silence(),
	}, &report)
	if err != nil {
		t.Fatalf("cargo dry-run build failed: %v", err)
	}
	if len(report.Commands) != 1 || !strings.Contains(report.Commands[0], "cargo build --target-dir") {
		t.Fatalf("unexpected cargo commands: %v", report.Commands)
	}
}

func TestSwiftPackageRunnerDryRunBuild(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Package.swift"), []byte("// swift-tools-version: 6.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRunnerManifest(t, root, "schema: holon/v0\nkind: native\nbuild:\n  runner: swift-package\nartifacts:\n  binary: swift-demo\n")

	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	var report Report
	err = (swiftPackageRunner{}).build(manifest, BuildContext{
		Target:   canonicalRuntimeTarget(),
		Mode:     buildModeRelease,
		DryRun:   true,
		Progress: progress.Silence(),
	}, &report)
	if err != nil {
		t.Fatalf("swift-package dry-run build failed: %v", err)
	}
	if len(report.Commands) != 1 || !strings.Contains(report.Commands[0], "swift build --build-path") {
		t.Fatalf("unexpected swift commands: %v", report.Commands)
	}
}

func TestFlutterRunnerDryRunBuild(t *testing.T) {
	root := t.TempDir()
	writeRunnerManifest(t, root, "schema: holon/v0\nkind: composite\nbuild:\n  runner: flutter\nartifacts:\n  primary: build/macos/Build/Products/Debug/flutter-demo.app\n")

	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	var report Report
	err = (flutterRunner{}).build(manifest, BuildContext{
		Target:   flutterTargetForTest(),
		Mode:     buildModeProfile,
		DryRun:   true,
		Progress: progress.Silence(),
	}, &report)
	if err != nil {
		t.Fatalf("flutter dry-run build failed: %v", err)
	}
	if len(report.Commands) != 1 || !strings.Contains(report.Commands[0], "flutter build") {
		t.Fatalf("unexpected flutter commands: %v", report.Commands)
	}
}

func TestNPMRunnerDryRunBuild(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{\"name\":\"demo\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRunnerManifest(t, root, "schema: holon/v0\nkind: native\nbuild:\n  runner: npm\nartifacts:\n  binary: npm-demo\n")

	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	var report Report
	err = (npmRunner{}).build(manifest, BuildContext{
		Target:   canonicalRuntimeTarget(),
		Mode:     buildModeDebug,
		DryRun:   true,
		Progress: progress.Silence(),
	}, &report)
	if err != nil {
		t.Fatalf("npm dry-run build failed: %v", err)
	}
	if len(report.Commands) != 2 || !strings.Contains(report.Commands[0], "npm ci") || !strings.Contains(report.Commands[1], "npm run build") {
		t.Fatalf("unexpected npm commands: %v", report.Commands)
	}
}

func TestGradleRunnerDryRunBuildUsesWrapper(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "gradlew"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRunnerManifest(t, root, "schema: holon/v0\nkind: native\nbuild:\n  runner: gradle\nartifacts:\n  binary: gradle-demo\n")

	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	var report Report
	err = (gradleRunner{}).build(manifest, BuildContext{
		Target:   canonicalRuntimeTarget(),
		Mode:     buildModeDebug,
		DryRun:   true,
		Progress: progress.Silence(),
	}, &report)
	if err != nil {
		t.Fatalf("gradle dry-run build failed: %v", err)
	}
	if len(report.Commands) != 1 || !strings.Contains(report.Commands[0], "./gradlew build") {
		t.Fatalf("unexpected gradle commands: %v", report.Commands)
	}
}

func TestDotnetRunnerDryRunBuild(t *testing.T) {
	root := t.TempDir()
	writeRunnerManifest(t, root, "schema: holon/v0\nkind: native\nbuild:\n  runner: dotnet\nartifacts:\n  binary: dotnet-demo\n")

	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	var report Report
	err = (dotnetRunner{}).build(manifest, BuildContext{
		Target:   canonicalRuntimeTarget(),
		Mode:     buildModeRelease,
		DryRun:   true,
		Progress: progress.Silence(),
	}, &report)
	if err != nil {
		t.Fatalf("dotnet dry-run build failed: %v", err)
	}
	if len(report.Commands) != 1 || !strings.Contains(report.Commands[0], "dotnet build -c Release -o") {
		t.Fatalf("unexpected dotnet commands: %v", report.Commands)
	}
}

func TestDotnetProjectFileAndWorkloadDetection(t *testing.T) {
	root := t.TempDir()
	projectFile := filepath.Join(root, "demo.csproj")
	if err := os.WriteFile(projectFile, []byte("<Project><PropertyGroup><UseMaui>true</UseMaui></PropertyGroup></Project>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRunnerManifest(t, root, "schema: holon/v0\nkind: composite\nbuild:\n  runner: dotnet\nartifacts:\n  primary: bin/Debug/net8.0/Demo.app\n")

	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	gotProject, err := dotnetProjectFile(manifest)
	if err != nil {
		t.Fatalf("dotnetProjectFile() failed: %v", err)
	}
	if gotProject != projectFile {
		t.Fatalf("project file = %q, want %q", gotProject, projectFile)
	}
	if workload := requiredDotnetWorkload(projectFile, "macos"); workload != "maui-maccatalyst" {
		t.Fatalf("requiredDotnetWorkload() = %q, want %q", workload, "maui-maccatalyst")
	}
}

func TestQtCMakeRunnerDryRunBuildUsesQt6Dir(t *testing.T) {
	root := t.TempDir()
	qtDir := filepath.Join(root, "qt", "lib", "cmake", "Qt6")
	if err := os.MkdirAll(qtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("Qt6_DIR", qtDir)
	if err := os.WriteFile(filepath.Join(root, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.20)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRunnerManifest(t, root, "schema: holon/v0\nkind: composite\nbuild:\n  runner: qt-cmake\nartifacts:\n  primary: build/qt-demo.app\n")

	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	var report Report
	err = (qtCMakeRunner{}).build(manifest, BuildContext{
		Target:   canonicalRuntimeTarget(),
		Mode:     buildModeDebug,
		DryRun:   true,
		Progress: progress.Silence(),
	}, &report)
	if err != nil {
		t.Fatalf("qt-cmake dry-run build failed: %v", err)
	}
	if len(report.Commands) != 2 || !strings.Contains(report.Commands[0], "-DCMAKE_PREFIX_PATH="+qtDir) {
		t.Fatalf("unexpected qt-cmake commands: %v", report.Commands)
	}
}

func writeRunnerManifest(t *testing.T, dir, yaml string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func flutterTargetForTest() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "linux":
		return "linux"
	case "windows":
		return "windows"
	default:
		return canonicalRuntimeTarget()
	}
}
