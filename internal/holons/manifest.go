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
	KindComposite    = "composite"
	RunnerGoModule   = "go-module"
	RunnerCMake      = "cmake"
	RunnerRecipe     = "recipe"
	ManifestFileName = "holon.yaml"
)

type Manifest struct {
	// Identity fields — present in holon.yaml but not used by lifecycle.
	Schema     string   `yaml:"schema"`
	UUID       string   `yaml:"uuid,omitempty"`
	GivenName  string   `yaml:"given_name,omitempty"`
	FamilyName string   `yaml:"family_name,omitempty"`
	Motto      string   `yaml:"motto,omitempty"`
	Composer   string   `yaml:"composer,omitempty"`
	Clade      string   `yaml:"clade,omitempty"`
	Status     string   `yaml:"status,omitempty"`
	Born       string   `yaml:"born,omitempty"`
	Lang       string   `yaml:"lang,omitempty"`
	Aliases    []string `yaml:"aliases,omitempty"`

	// Lineage fields.
	Parents      []string `yaml:"parents,omitempty"`
	Reproduction string   `yaml:"reproduction,omitempty"`
	GeneratedBy  string   `yaml:"generated_by,omitempty"`

	// Description.
	Description string `yaml:"description,omitempty"`

	// Operational fields — used by lifecycle.
	Kind      string        `yaml:"kind"`
	Platforms []string      `yaml:"platforms,omitempty"`
	Build     BuildConfig   `yaml:"build"`
	Requires  Requires      `yaml:"requires,omitempty"`
	Delegates Delegates     `yaml:"delegates,omitempty"`
	Artifacts ArtifactPaths `yaml:"artifacts"`

	// Contract fields — not used by lifecycle.
	Contract interface{} `yaml:"contract,omitempty"`
}

type BuildConfig struct {
	Runner   string                  `yaml:"runner"`
	Main     string                  `yaml:"main,omitempty"`
	Defaults *RecipeDefaults         `yaml:"defaults,omitempty"`
	Members  []RecipeMember          `yaml:"members,omitempty"`
	Targets  map[string]RecipeTarget `yaml:"targets,omitempty"`
}

// RecipeDefaults provides default target and mode for recipe builds.
type RecipeDefaults struct {
	Target string `yaml:"target,omitempty"`
	Mode   string `yaml:"mode,omitempty"`
}

// RecipeMember is a named build participant in a composite holon.
type RecipeMember struct {
	ID   string `yaml:"id"`
	Path string `yaml:"path"`
	Type string `yaml:"type"` // "holon" or "component"
}

// RecipeTarget defines the build steps for a specific platform.
type RecipeTarget struct {
	Steps []RecipeStep `yaml:"steps"`
}

// RecipeStep is one step in a recipe build plan.
// Exactly one field should be set.
type RecipeStep struct {
	BuildMember string          `yaml:"build_member,omitempty"`
	Exec        *RecipeStepExec `yaml:"exec,omitempty"`
	Copy        *RecipeStepCopy `yaml:"copy,omitempty"`
	AssertFile  *RecipeStepFile `yaml:"assert_file,omitempty"`
}

// RecipeStepExec runs a command with an explicit argv and working directory.
type RecipeStepExec struct {
	Cwd  string   `yaml:"cwd"`
	Argv []string `yaml:"argv"`
}

// RecipeStepCopy copies a file from one manifest-relative path to another.
type RecipeStepCopy struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// RecipeStepFile verifies a manifest-relative file exists.
type RecipeStepFile struct {
	Path string `yaml:"path"`
}

type Requires struct {
	Commands []string `yaml:"commands,omitempty"`
	Files    []string `yaml:"files,omitempty"`
}

type Delegates struct {
	Commands []string `yaml:"commands,omitempty"`
}

type ArtifactPaths struct {
	Binary  string `yaml:"binary"`
	Primary string `yaml:"primary,omitempty"`
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

// PrimaryArtifactPath returns the primary artifact path (success contract).
// If artifacts.primary is set, it takes precedence over artifacts.binary.
func (m *LoadedManifest) PrimaryArtifactPath() string {
	if p := m.Manifest.Artifacts.Primary; strings.TrimSpace(p) != "" {
		return filepath.Join(m.Dir, filepath.FromSlash(p))
	}
	return m.BinaryPath()
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
	case KindNative, KindWrapper, KindComposite:
	default:
		return fmt.Errorf("%s: kind must be %q, %q, or %q", m.Path, KindNative, KindWrapper, KindComposite)
	}

	switch m.Manifest.Build.Runner {
	case RunnerGoModule, RunnerCMake, RunnerRecipe:
	default:
		return fmt.Errorf("%s: build.runner must be %q, %q, or %q", m.Path, RunnerGoModule, RunnerCMake, RunnerRecipe)
	}

	// Artifact validation: binary required for leaf runners, primary or binary for recipe.
	hasBinary := strings.TrimSpace(m.Manifest.Artifacts.Binary) != ""
	hasPrimary := strings.TrimSpace(m.Manifest.Artifacts.Primary) != ""
	if !hasBinary && !hasPrimary {
		return fmt.Errorf("%s: artifacts.binary or artifacts.primary is required", m.Path)
	}
	if hasBinary && filepath.IsAbs(m.Manifest.Artifacts.Binary) {
		return fmt.Errorf("%s: artifacts.binary must be relative to the manifest directory", m.Path)
	}
	if hasPrimary && filepath.IsAbs(m.Manifest.Artifacts.Primary) {
		return fmt.Errorf("%s: artifacts.primary must be relative to the manifest directory", m.Path)
	}

	if m.Manifest.Build.Runner != RunnerGoModule && strings.TrimSpace(m.Manifest.Build.Main) != "" {
		return fmt.Errorf("%s: build.main is only valid for %q", m.Path, RunnerGoModule)
	}

	if m.Manifest.Kind != KindWrapper && len(m.Manifest.Delegates.Commands) > 0 {
		return fmt.Errorf("%s: delegates.commands is only valid for wrapper holons", m.Path)
	}

	// Recipe-specific validation.
	if m.Manifest.Build.Runner == RunnerRecipe {
		if err := validateRecipe(m); err != nil {
			return err
		}
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

// validateRecipe checks recipe-specific manifest constraints.
func validateRecipe(m *LoadedManifest) error {
	if len(m.Manifest.Build.Members) == 0 {
		return fmt.Errorf("%s: recipe runner requires at least one member", m.Path)
	}
	if len(m.Manifest.Build.Targets) == 0 {
		return fmt.Errorf("%s: recipe runner requires at least one target", m.Path)
	}

	memberIDs := make(map[string]bool, len(m.Manifest.Build.Members))
	for _, member := range m.Manifest.Build.Members {
		if strings.TrimSpace(member.ID) == "" {
			return fmt.Errorf("%s: recipe member must have an id", m.Path)
		}
		if memberIDs[member.ID] {
			return fmt.Errorf("%s: duplicate recipe member id %q", m.Path, member.ID)
		}
		memberIDs[member.ID] = true

		if strings.TrimSpace(member.Path) == "" {
			return fmt.Errorf("%s: recipe member %q must have a path", m.Path, member.ID)
		}
		switch member.Type {
		case "holon", "component":
		default:
			return fmt.Errorf("%s: recipe member %q type must be \"holon\" or \"component\"", m.Path, member.ID)
		}
	}

	// Validate that build_member references exist in members.
	for targetName, target := range m.Manifest.Build.Targets {
		for i, step := range target.Steps {
			if step.BuildMember != "" && !memberIDs[step.BuildMember] {
				return fmt.Errorf("%s: target %q step %d references unknown member %q", m.Path, targetName, i+1, step.BuildMember)
			}
		}
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
