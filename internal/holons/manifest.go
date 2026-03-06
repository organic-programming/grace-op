package holons

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	SchemaV0         = "holon/v0"
	KindNative       = "native"
	KindWrapper      = "wrapper"
	RunnerGoModule   = "go-module"
	RunnerCMake      = "cmake"
	ManifestFileName = "holon.yaml"
)

type Manifest struct {
	Schema    string        `yaml:"schema"`
	Kind      string        `yaml:"kind"`
	Platforms []string      `yaml:"platforms,omitempty"`
	Build     BuildConfig   `yaml:"build"`
	Requires  Requires      `yaml:"requires,omitempty"`
	Delegates Delegates     `yaml:"delegates,omitempty"`
	Artifacts ArtifactPaths `yaml:"artifacts"`
}

type BuildConfig struct {
	Runner string `yaml:"runner"`
	Main   string `yaml:"main,omitempty"`
}

type Requires struct {
	Commands []string `yaml:"commands,omitempty"`
	Files    []string `yaml:"files,omitempty"`
}

type Delegates struct {
	Commands []string `yaml:"commands,omitempty"`
}

type ArtifactPaths struct {
	Binary string `yaml:"binary"`
}

type LoadedManifest struct {
	Manifest Manifest
	Dir      string
	Path     string
	Name     string
}

func LoadManifest(dir string) (*LoadedManifest, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", dir, err)
	}

	manifestPath := filepath.Join(absDir, ManifestFileName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", manifestPath, err)
	}

	var manifest Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("parse %s: %w", manifestPath, err)
	}

	loaded := &LoadedManifest{
		Manifest: manifest,
		Dir:      absDir,
		Path:     manifestPath,
		Name:     filepath.Base(absDir),
	}

	if err := validateManifest(loaded); err != nil {
		return nil, err
	}

	return loaded, nil
}

func (m *LoadedManifest) SupportsCurrentPlatform() bool {
	if m == nil || len(m.Manifest.Platforms) == 0 {
		return true
	}
	return slices.Contains(m.Manifest.Platforms, runtime.GOOS)
}

func (m *LoadedManifest) BinaryPath() string {
	return filepath.Join(m.Dir, filepath.FromSlash(m.Manifest.Artifacts.Binary))
}

func (m *LoadedManifest) OpRoot() string {
	return filepath.Join(m.Dir, ".op")
}

func (m *LoadedManifest) CMakeBuildDir() string {
	return filepath.Join(m.Dir, ".op", "build", "cmake")
}

func (m *LoadedManifest) GoMainPackage() string {
	if strings.TrimSpace(m.Manifest.Build.Main) != "" {
		return m.Manifest.Build.Main
	}
	return "./cmd/" + m.Name
}

func validateManifest(m *LoadedManifest) error {
	if m.Manifest.Schema != SchemaV0 {
		return fmt.Errorf("%s: schema must be %q", m.Path, SchemaV0)
	}

	switch m.Manifest.Kind {
	case KindNative, KindWrapper:
	default:
		return fmt.Errorf("%s: kind must be %q or %q", m.Path, KindNative, KindWrapper)
	}

	switch m.Manifest.Build.Runner {
	case RunnerGoModule, RunnerCMake:
	default:
		return fmt.Errorf("%s: build.runner must be %q or %q", m.Path, RunnerGoModule, RunnerCMake)
	}

	if strings.TrimSpace(m.Manifest.Artifacts.Binary) == "" {
		return fmt.Errorf("%s: artifacts.binary is required", m.Path)
	}
	if filepath.IsAbs(m.Manifest.Artifacts.Binary) {
		return fmt.Errorf("%s: artifacts.binary must be relative to the manifest directory", m.Path)
	}

	if m.Manifest.Build.Runner != RunnerGoModule && strings.TrimSpace(m.Manifest.Build.Main) != "" {
		return fmt.Errorf("%s: build.main is only valid for %q", m.Path, RunnerGoModule)
	}

	if m.Manifest.Kind != KindWrapper && len(m.Manifest.Delegates.Commands) > 0 {
		return fmt.Errorf("%s: delegates.commands is only valid for wrapper holons", m.Path)
	}

	for _, platform := range m.Manifest.Platforms {
		if !isValidPlatform(platform) {
			return fmt.Errorf("%s: unsupported platform %q", m.Path, platform)
		}
	}

	if err := validateList("requires.commands", m.Manifest.Requires.Commands); err != nil {
		return fmt.Errorf("%s: %w", m.Path, err)
	}
	if err := validateList("requires.files", m.Manifest.Requires.Files); err != nil {
		return fmt.Errorf("%s: %w", m.Path, err)
	}
	if err := validateList("delegates.commands", m.Manifest.Delegates.Commands); err != nil {
		return fmt.Errorf("%s: %w", m.Path, err)
	}

	return nil
}

func validateList(field string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fmt.Errorf("%s cannot contain empty values", field)
		}
		if _, ok := seen[trimmed]; ok {
			return fmt.Errorf("%s contains duplicate value %q", field, trimmed)
		}
		seen[trimmed] = struct{}{}
	}
	return nil
}

func isValidPlatform(platform string) bool {
	switch platform {
	case "aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios",
		"js", "linux", "netbsd", "openbsd", "plan9", "solaris", "wasip1",
		"windows":
		return true
	default:
		return false
	}
}
