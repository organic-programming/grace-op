// Package identity defines the domain model for holon civil status.
// A holon's identity lives in holon.yaml: UUID, name, clade, and lineage.
package identity

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Identity holds the identity fields stored in holon.yaml.
type Identity struct {
	Schema string `yaml:"schema,omitempty"`

	// Required
	UUID       string `yaml:"uuid"`
	GivenName  string `yaml:"given_name"`
	FamilyName string `yaml:"family_name"`
	Motto      string `yaml:"motto"`
	Composer   string `yaml:"composer"`
	Clade      string `yaml:"clade"`
	Status     string `yaml:"status"`
	Born       string `yaml:"born"`

	// Lineage
	Parents      []string `yaml:"parents"`
	Reproduction string   `yaml:"reproduction"`

	// Optional
	Aliases []string `yaml:"aliases,omitempty"`

	// Metadata
	GeneratedBy string `yaml:"generated_by"`
	Lang        string `yaml:"lang"`
	ProtoStatus string `yaml:"proto_status"`

	// Optional descriptive text often scaffolded by Sophia.
	Description string `yaml:"description,omitempty"`
}

// Slug derives a normalized, lowercase-hyphenated identifier from the
// holon's given_name and family_name. This is the canonical name users
// pass to "op run" and "op build".
func (id Identity) Slug() string {
	parts := make([]string, 0, 2)
	if g := strings.TrimSpace(id.GivenName); g != "" {
		parts = append(parts, g)
	}
	if f := strings.TrimSpace(id.FamilyName); f != "" {
		parts = append(parts, f)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.ToLower(strings.Join(parts, "-"))
}

// Clades enumerates valid computational nature classifications.
var Clades = []string{
	"deterministic/pure",
	"deterministic/stateful",
	"deterministic/io_bound",
	"probabilistic/generative",
	"probabilistic/perceptual",
	"probabilistic/adaptive",
}

// Statuses enumerates valid lifecycle stages.
var Statuses = []string{"draft", "stable", "deprecated", "dead"}

// ReproductionModes enumerates how a holon can be created.
var ReproductionModes = []string{"manual", "assisted", "automatic", "autopoietic", "bred"}

// New creates a fresh identity with a generated UUID and today's date.
func New() Identity {
	return Identity{
		Schema:      "holon/v0",
		UUID:        uuid.New().String(),
		Status:      "draft",
		Born:        time.Now().Format("2006-01-02"),
		Parents:     []string{},
		GeneratedBy: "dummy-test",
		ProtoStatus: "draft",
	}
}
