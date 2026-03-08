package identity

import (
	"fmt"
	"os"
	"strings"
	"text/template"
)

// holonTemplate generates a holon.yaml identity scaffold.
var holonTemplate = `schema: {{ .Schema | quote }}
uuid: {{ .UUID | quote }}
given_name: {{ .GivenName | quote }}
family_name: {{ .FamilyName | quote }}
motto: {{ .Motto | quote }}
composer: {{ .Composer | quote }}
clade: {{ .Clade | quote }}
status: {{ .Status }}
born: {{ .Born | quote }}
parents: [{{ joinQuoted .Parents }}]
reproduction: {{ .Reproduction | quote }}
aliases: [{{ joinQuoted .Aliases }}]
generated_by: {{ .GeneratedBy | quote }}
lang: {{ .Lang | quote }}
proto_status: {{ .ProtoStatus }}
description: |
{{ .Description | indent }}
`

var tmplFuncs = template.FuncMap{
	"quote": func(s string) string {
		return fmt.Sprintf("%q", s)
	},
	"joinQuoted": func(ss []string) string {
		quoted := make([]string, len(ss))
		for i, s := range ss {
			quoted[i] = fmt.Sprintf("%q", s)
		}
		return strings.Join(quoted, ", ")
	},
	"indent": func(s string) string {
		if strings.TrimSpace(s) == "" {
			s = "<Describe what this holon does and complete the operational sections.>"
		}
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			lines[i] = "  " + line
		}
		return strings.Join(lines, "\n")
	},
}

// WriteHolonYAML renders an Identity to a holon.yaml file at the given path.
func WriteHolonYAML(id Identity, path string) error {
	if strings.TrimSpace(id.Schema) == "" {
		id.Schema = "holon/v0"
	}
	tmpl, err := template.New("holon").Funcs(tmplFuncs).Parse(holonTemplate)
	if err != nil {
		return fmt.Errorf("template error: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", path, err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, id); err != nil {
		return fmt.Errorf("template execution error: %w", err)
	}
	return nil
}
