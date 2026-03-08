package inspect

import (
	"path/filepath"
	"strings"

	"github.com/organic-programming/grace-op/internal/holons"
)

// LocalCatalog is a parsed local holon ready for inspect/tools/mcp reuse.
type LocalCatalog struct {
	*Catalog
	Dir  string
	Slug string
}

// LoadLocal resolves a slug/path/uuid selector, parses the holon's protos, and
// attaches identity metadata and skills from holon.yaml.
func LoadLocal(ref string) (*LocalCatalog, error) {
	target, err := holons.ResolveTarget(ref)
	if err != nil {
		return nil, err
	}

	catalog, err := ParseCatalog(filepath.Join(target.Dir, "protos"))
	if err != nil {
		return nil, err
	}

	slug := filepath.Base(target.Dir)
	catalog.Document.Slug = slug
	if target.Identity != nil && strings.TrimSpace(target.Identity.Motto) != "" {
		catalog.Document.Motto = strings.TrimSpace(target.Identity.Motto)
	}
	if target.Manifest != nil {
		catalog.Document.Skills = manifestSkills(target.Manifest.Manifest.Skills)
	}

	return &LocalCatalog{
		Catalog: catalog,
		Dir:     target.Dir,
		Slug:    slug,
	}, nil
}

func manifestSkills(skills []holons.Skill) []Skill {
	out := make([]Skill, 0, len(skills))
	for _, skill := range skills {
		out = append(out, Skill{
			Name:        strings.TrimSpace(skill.Name),
			Description: strings.TrimSpace(skill.Description),
			When:        strings.TrimSpace(skill.When),
			Steps:       append([]string(nil), skill.Steps...),
		})
	}
	return out
}
