package holons

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	holonsv1 "github.com/organic-programming/go-holons/gen/go/holons/v1"
	godescribe "github.com/organic-programming/go-holons/pkg/describe"
	"github.com/organic-programming/grace-op/internal/progress"
)

const (
	describeTemplatePrefix = "describe."
	describeTemplateSuffix = ".tmpl"
	describeOutputPrefix   = "describe_generated."
)

func generateDescribeSource(manifest *LoadedManifest, reporter progress.Reporter) (restore func(), err error) {
	restore = func() {}

	if manifest == nil {
		return restore, nil
	}

	lang := strings.TrimSpace(manifest.Manifest.Lang)
	if lang == "" {
		return restore, nil
	}

	templatePath, ext, err := findDescribeTemplate(manifest.Dir, lang)
	if err != nil {
		return restore, err
	}
	if templatePath == "" {
		return restore, nil
	}

	response, err := godescribe.BuildResponse(describeProtoDir(manifest), manifest.Path)
	if err != nil {
		return restore, fmt.Errorf("build describe response: %w", err)
	}

	outputPath := filepath.Join(manifest.Dir, "gen", describeOutputPrefix+ext)
	restore, err = writeDescribeSource(templatePath, outputPath, response)
	if err != nil {
		return func() {}, err
	}

	reporter.Step(fmt.Sprintf("incode description: %s", workspaceRelativePath(outputPath)))
	return restore, nil
}

func describeProtoDir(manifest *LoadedManifest) string {
	if manifest == nil {
		return ""
	}

	candidate := filepath.Join(manifest.Dir, "proto")
	info, err := os.Stat(candidate)
	if err == nil && info.IsDir() {
		return candidate
	}

	return manifest.Dir
}

func findDescribeTemplate(holonDir, lang string) (path string, ext string, err error) {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return "", "", nil
	}

	for current := filepath.Clean(holonDir); ; {
		templateDir := filepath.Join(current, "sdk", lang+"-holons", "templates")
		path, ext, err = findDescribeTemplateInDir(templateDir)
		if err != nil {
			return "", "", err
		}
		if path != "" {
			return path, ext, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", "", nil
		}
		current = parent
	}
}

func findDescribeTemplateInDir(templateDir string) (path string, ext string, err error) {
	entries, err := os.ReadDir(templateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("read template dir %s: %w", templateDir, err)
	}

	matches := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, describeTemplatePrefix) || !strings.HasSuffix(name, describeTemplateSuffix) {
			continue
		}
		matches = append(matches, name)
	}

	if len(matches) == 0 {
		return "", "", nil
	}

	sort.Strings(matches)
	if len(matches) > 1 {
		return "", "", fmt.Errorf("multiple describe templates found in %s: %s", templateDir, strings.Join(matches, ", "))
	}

	name := matches[0]
	ext = strings.TrimSuffix(strings.TrimPrefix(name, describeTemplatePrefix), describeTemplateSuffix)
	if ext == "" {
		return "", "", fmt.Errorf("describe template %s has empty extension", filepath.Join(templateDir, name))
	}
	return filepath.Join(templateDir, name), ext, nil
}

func writeDescribeSource(templatePath, outputPath string, response *holonsv1.DescribeResponse) (restore func(), err error) {
	restore = func() {}

	originalExists := false
	var original []byte
	mode := os.FileMode(0o644)

	if info, statErr := os.Stat(outputPath); statErr == nil {
		originalExists = true
		mode = info.Mode()
		original, err = os.ReadFile(outputPath)
		if err != nil {
			return func() {}, fmt.Errorf("read existing output %s: %w", outputPath, err)
		}
	} else if !os.IsNotExist(statErr) {
		return func() {}, fmt.Errorf("stat output %s: %w", outputPath, statErr)
	}

	restore = func() {
		if originalExists {
			_ = os.WriteFile(outputPath, original, mode)
			return
		}
		_ = os.Remove(outputPath)
	}

	rendered, err := renderDescribeTemplate(templatePath, outputPath, response)
	if err != nil {
		restore()
		return func() {}, err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		restore()
		return func() {}, fmt.Errorf("create output dir for %s: %w", outputPath, err)
	}
	if err := os.WriteFile(outputPath, rendered, mode); err != nil {
		restore()
		return func() {}, fmt.Errorf("write %s: %w", outputPath, err)
	}

	return restore, nil
}

func renderDescribeTemplate(templatePath, outputPath string, response *holonsv1.DescribeResponse) ([]byte, error) {
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", templatePath, err)
	}

	ext := strings.TrimPrefix(filepath.Ext(outputPath), ".")
	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(describeTemplateFuncs(ext)).Parse(string(templateBytes))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", templatePath, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, response); err != nil {
		return nil, fmt.Errorf("execute template %s: %w", templatePath, err)
	}

	rendered := buf.Bytes()
	if ext == "go" {
		rendered, err = format.Source(rendered)
		if err != nil {
			return nil, fmt.Errorf("format generated Go source for %s: %w", outputPath, err)
		}
	}

	return rendered, nil
}

func describeTemplateFuncs(ext string) template.FuncMap {
	funcs := template.FuncMap{}
	if ext == "go" {
		funcs["goDescribeResponse"] = func(response *holonsv1.DescribeResponse) string {
			return goDescribeResponseLiteral(response)
		}
	}
	return funcs
}

func goDescribeResponseLiteral(response *holonsv1.DescribeResponse) string {
	if response == nil {
		return "&holonsv1.DescribeResponse{}"
	}
	return literalValue(0, func(buf *strings.Builder, indent int) {
		writeDescribeResponseLiteral(buf, response, indent)
	})
}

func writeDescribeResponseLiteral(buf *strings.Builder, response *holonsv1.DescribeResponse, indent int) {
	writeLine(buf, indent, "&holonsv1.DescribeResponse{")
	if response.GetManifest() != nil {
		buf.WriteString(goIndent(indent + 1))
		buf.WriteString("Manifest: ")
		buf.WriteString(literalValue(indent+1, func(buf *strings.Builder, indent int) {
			writeHolonManifestLiteral(buf, response.GetManifest(), indent)
		}))
		buf.WriteString(",\n")
	}
	if len(response.GetServices()) > 0 {
		writeLine(buf, indent+1, "Services: []*holonsv1.ServiceDoc{")
		for _, service := range response.GetServices() {
			buf.WriteString(literalValue(indent+2, func(buf *strings.Builder, indent int) {
				writeServiceDocLiteral(buf, service, indent)
			}))
			buf.WriteString(",\n")
		}
		writeLine(buf, indent+1, "},")
	}
	writeLine(buf, indent, "}")
}

func writeHolonManifestLiteral(buf *strings.Builder, manifest *holonsv1.HolonManifest, indent int) {
	if manifest == nil {
		buf.WriteString("nil")
		return
	}

	writeLine(buf, indent, "&holonsv1.HolonManifest{")
	if manifest.GetIdentity() != nil {
		buf.WriteString(goIndent(indent + 1))
		buf.WriteString("Identity: ")
		buf.WriteString(literalValue(indent+1, func(buf *strings.Builder, indent int) {
			writeIdentityLiteral(buf, manifest.GetIdentity(), indent)
		}))
		buf.WriteString(",\n")
	}
	writeStringField(buf, indent+1, "Description", manifest.GetDescription())
	writeStringField(buf, indent+1, "Lang", manifest.GetLang())
	if len(manifest.GetSkills()) > 0 {
		writeLine(buf, indent+1, "Skills: []*holonsv1.HolonManifest_Skill{")
		for _, skill := range manifest.GetSkills() {
			buf.WriteString(literalValue(indent+2, func(buf *strings.Builder, indent int) {
				writeSkillLiteral(buf, skill, indent)
			}))
			buf.WriteString(",\n")
		}
		writeLine(buf, indent+1, "},")
	}
	writeStringField(buf, indent+1, "Kind", manifest.GetKind())
	if manifest.GetBuild() != nil {
		buf.WriteString(goIndent(indent + 1))
		buf.WriteString("Build: ")
		buf.WriteString(literalValue(indent+1, func(buf *strings.Builder, indent int) {
			writeBuildLiteral(buf, manifest.GetBuild(), indent)
		}))
		buf.WriteString(",\n")
	}
	if manifest.GetRequires() != nil {
		buf.WriteString(goIndent(indent + 1))
		buf.WriteString("Requires: ")
		buf.WriteString(literalValue(indent+1, func(buf *strings.Builder, indent int) {
			writeRequiresLiteral(buf, manifest.GetRequires(), indent)
		}))
		buf.WriteString(",\n")
	}
	if manifest.GetArtifacts() != nil {
		buf.WriteString(goIndent(indent + 1))
		buf.WriteString("Artifacts: ")
		buf.WriteString(literalValue(indent+1, func(buf *strings.Builder, indent int) {
			writeArtifactsLiteral(buf, manifest.GetArtifacts(), indent)
		}))
		buf.WriteString(",\n")
	}
	if len(manifest.GetSequences()) > 0 {
		writeLine(buf, indent+1, "Sequences: []*holonsv1.HolonManifest_Sequence{")
		for _, sequence := range manifest.GetSequences() {
			buf.WriteString(literalValue(indent+2, func(buf *strings.Builder, indent int) {
				writeSequenceLiteral(buf, sequence, indent)
			}))
			buf.WriteString(",\n")
		}
		writeLine(buf, indent+1, "},")
	}
	writeLine(buf, indent, "}")
}

func writeIdentityLiteral(buf *strings.Builder, identity *holonsv1.HolonManifest_Identity, indent int) {
	if identity == nil {
		buf.WriteString("nil")
		return
	}

	writeLine(buf, indent, "&holonsv1.HolonManifest_Identity{")
	writeStringField(buf, indent+1, "Schema", identity.GetSchema())
	writeStringField(buf, indent+1, "Uuid", identity.GetUuid())
	writeStringField(buf, indent+1, "GivenName", identity.GetGivenName())
	writeStringField(buf, indent+1, "FamilyName", identity.GetFamilyName())
	writeStringField(buf, indent+1, "Motto", identity.GetMotto())
	writeStringField(buf, indent+1, "Composer", identity.GetComposer())
	writeStringField(buf, indent+1, "Status", identity.GetStatus())
	writeStringField(buf, indent+1, "Born", identity.GetBorn())
	writeStringField(buf, indent+1, "Version", identity.GetVersion())
	writeStringSliceField(buf, indent+1, "Aliases", identity.GetAliases())
	writeLine(buf, indent, "}")
}

func writeSkillLiteral(buf *strings.Builder, skill *holonsv1.HolonManifest_Skill, indent int) {
	if skill == nil {
		buf.WriteString("nil")
		return
	}

	writeLine(buf, indent, "&holonsv1.HolonManifest_Skill{")
	writeStringField(buf, indent+1, "Name", skill.GetName())
	writeStringField(buf, indent+1, "Description", skill.GetDescription())
	writeStringField(buf, indent+1, "When", skill.GetWhen())
	writeStringSliceField(buf, indent+1, "Steps", skill.GetSteps())
	writeLine(buf, indent, "}")
}

func writeSequenceLiteral(buf *strings.Builder, sequence *holonsv1.HolonManifest_Sequence, indent int) {
	if sequence == nil {
		buf.WriteString("nil")
		return
	}

	writeLine(buf, indent, "&holonsv1.HolonManifest_Sequence{")
	writeStringField(buf, indent+1, "Name", sequence.GetName())
	writeStringField(buf, indent+1, "Description", sequence.GetDescription())
	if len(sequence.GetParams()) > 0 {
		writeLine(buf, indent+1, "Params: []*holonsv1.HolonManifest_Sequence_Param{")
		for _, param := range sequence.GetParams() {
			buf.WriteString(literalValue(indent+2, func(buf *strings.Builder, indent int) {
				writeSequenceParamLiteral(buf, param, indent)
			}))
			buf.WriteString(",\n")
		}
		writeLine(buf, indent+1, "},")
	}
	writeStringSliceField(buf, indent+1, "Steps", sequence.GetSteps())
	writeLine(buf, indent, "}")
}

func writeSequenceParamLiteral(buf *strings.Builder, param *holonsv1.HolonManifest_Sequence_Param, indent int) {
	if param == nil {
		buf.WriteString("nil")
		return
	}

	writeLine(buf, indent, "&holonsv1.HolonManifest_Sequence_Param{")
	writeStringField(buf, indent+1, "Name", param.GetName())
	writeStringField(buf, indent+1, "Description", param.GetDescription())
	writeBoolField(buf, indent+1, "Required", param.GetRequired())
	writeStringField(buf, indent+1, "Default", param.GetDefault())
	writeLine(buf, indent, "}")
}

func writeBuildLiteral(buf *strings.Builder, build *holonsv1.HolonManifest_Build, indent int) {
	if build == nil {
		buf.WriteString("nil")
		return
	}

	writeLine(buf, indent, "&holonsv1.HolonManifest_Build{")
	writeStringField(buf, indent+1, "Runner", build.GetRunner())
	writeStringField(buf, indent+1, "Main", build.GetMain())
	writeLine(buf, indent, "}")
}

func writeRequiresLiteral(buf *strings.Builder, requires *holonsv1.HolonManifest_Requires, indent int) {
	if requires == nil {
		buf.WriteString("nil")
		return
	}

	writeLine(buf, indent, "&holonsv1.HolonManifest_Requires{")
	writeStringSliceField(buf, indent+1, "Commands", requires.GetCommands())
	writeStringSliceField(buf, indent+1, "Files", requires.GetFiles())
	writeStringSliceField(buf, indent+1, "Platforms", requires.GetPlatforms())
	writeLine(buf, indent, "}")
}

func writeArtifactsLiteral(buf *strings.Builder, artifacts *holonsv1.HolonManifest_Artifacts, indent int) {
	if artifacts == nil {
		buf.WriteString("nil")
		return
	}

	writeLine(buf, indent, "&holonsv1.HolonManifest_Artifacts{")
	writeStringField(buf, indent+1, "Binary", artifacts.GetBinary())
	writeStringField(buf, indent+1, "Primary", artifacts.GetPrimary())
	writeLine(buf, indent, "}")
}

func writeServiceDocLiteral(buf *strings.Builder, service *holonsv1.ServiceDoc, indent int) {
	if service == nil {
		buf.WriteString("nil")
		return
	}

	writeLine(buf, indent, "&holonsv1.ServiceDoc{")
	writeStringField(buf, indent+1, "Name", service.GetName())
	writeStringField(buf, indent+1, "Description", service.GetDescription())
	if len(service.GetMethods()) > 0 {
		writeLine(buf, indent+1, "Methods: []*holonsv1.MethodDoc{")
		for _, method := range service.GetMethods() {
			buf.WriteString(literalValue(indent+2, func(buf *strings.Builder, indent int) {
				writeMethodDocLiteral(buf, method, indent)
			}))
			buf.WriteString(",\n")
		}
		writeLine(buf, indent+1, "},")
	}
	writeLine(buf, indent, "}")
}

func writeMethodDocLiteral(buf *strings.Builder, method *holonsv1.MethodDoc, indent int) {
	if method == nil {
		buf.WriteString("nil")
		return
	}

	writeLine(buf, indent, "&holonsv1.MethodDoc{")
	writeStringField(buf, indent+1, "Name", method.GetName())
	writeStringField(buf, indent+1, "Description", method.GetDescription())
	writeStringField(buf, indent+1, "InputType", method.GetInputType())
	writeStringField(buf, indent+1, "OutputType", method.GetOutputType())
	if len(method.GetInputFields()) > 0 {
		writeLine(buf, indent+1, "InputFields: []*holonsv1.FieldDoc{")
		for _, field := range method.GetInputFields() {
			buf.WriteString(literalValue(indent+2, func(buf *strings.Builder, indent int) {
				writeFieldDocLiteral(buf, field, indent)
			}))
			buf.WriteString(",\n")
		}
		writeLine(buf, indent+1, "},")
	}
	if len(method.GetOutputFields()) > 0 {
		writeLine(buf, indent+1, "OutputFields: []*holonsv1.FieldDoc{")
		for _, field := range method.GetOutputFields() {
			buf.WriteString(literalValue(indent+2, func(buf *strings.Builder, indent int) {
				writeFieldDocLiteral(buf, field, indent)
			}))
			buf.WriteString(",\n")
		}
		writeLine(buf, indent+1, "},")
	}
	writeBoolField(buf, indent+1, "ClientStreaming", method.GetClientStreaming())
	writeBoolField(buf, indent+1, "ServerStreaming", method.GetServerStreaming())
	writeStringField(buf, indent+1, "ExampleInput", method.GetExampleInput())
	writeLine(buf, indent, "}")
}

func writeFieldDocLiteral(buf *strings.Builder, field *holonsv1.FieldDoc, indent int) {
	if field == nil {
		buf.WriteString("nil")
		return
	}

	writeLine(buf, indent, "&holonsv1.FieldDoc{")
	writeStringField(buf, indent+1, "Name", field.GetName())
	writeStringField(buf, indent+1, "Type", field.GetType())
	writeInt32Field(buf, indent+1, "Number", field.GetNumber())
	writeStringField(buf, indent+1, "Description", field.GetDescription())
	writeFieldLabelField(buf, indent+1, "Label", field.GetLabel())
	writeStringField(buf, indent+1, "MapKeyType", field.GetMapKeyType())
	writeStringField(buf, indent+1, "MapValueType", field.GetMapValueType())
	if len(field.GetNestedFields()) > 0 {
		writeLine(buf, indent+1, "NestedFields: []*holonsv1.FieldDoc{")
		for _, nested := range field.GetNestedFields() {
			buf.WriteString(literalValue(indent+2, func(buf *strings.Builder, indent int) {
				writeFieldDocLiteral(buf, nested, indent)
			}))
			buf.WriteString(",\n")
		}
		writeLine(buf, indent+1, "},")
	}
	if len(field.GetEnumValues()) > 0 {
		writeLine(buf, indent+1, "EnumValues: []*holonsv1.EnumValueDoc{")
		for _, value := range field.GetEnumValues() {
			buf.WriteString(literalValue(indent+2, func(buf *strings.Builder, indent int) {
				writeEnumValueDocLiteral(buf, value, indent)
			}))
			buf.WriteString(",\n")
		}
		writeLine(buf, indent+1, "},")
	}
	writeBoolField(buf, indent+1, "Required", field.GetRequired())
	writeStringField(buf, indent+1, "Example", field.GetExample())
	writeLine(buf, indent, "}")
}

func writeEnumValueDocLiteral(buf *strings.Builder, value *holonsv1.EnumValueDoc, indent int) {
	if value == nil {
		buf.WriteString("nil")
		return
	}

	writeLine(buf, indent, "&holonsv1.EnumValueDoc{")
	writeStringField(buf, indent+1, "Name", value.GetName())
	writeInt32Field(buf, indent+1, "Number", value.GetNumber())
	writeStringField(buf, indent+1, "Description", value.GetDescription())
	writeLine(buf, indent, "}")
}

func writeStringField(buf *strings.Builder, indent int, fieldName, value string) {
	if value == "" {
		return
	}
	writeLine(buf, indent, fmt.Sprintf("%s: %s,", fieldName, strconv.Quote(value)))
}

func writeStringSliceField(buf *strings.Builder, indent int, fieldName string, values []string) {
	if len(values) == 0 {
		return
	}

	writeLine(buf, indent, fieldName+": []string{")
	for _, value := range values {
		writeLine(buf, indent+1, strconv.Quote(value)+",")
	}
	writeLine(buf, indent, "},")
}

func writeBoolField(buf *strings.Builder, indent int, fieldName string, value bool) {
	if !value {
		return
	}
	writeLine(buf, indent, fmt.Sprintf("%s: true,", fieldName))
}

func writeInt32Field(buf *strings.Builder, indent int, fieldName string, value int32) {
	if value == 0 {
		return
	}
	writeLine(buf, indent, fmt.Sprintf("%s: %d,", fieldName, value))
}

func writeFieldLabelField(buf *strings.Builder, indent int, fieldName string, value holonsv1.FieldLabel) {
	writeLine(buf, indent, fmt.Sprintf("%s: %s,", fieldName, goFieldLabelLiteral(value)))
}

func goFieldLabelLiteral(value holonsv1.FieldLabel) string {
	name := value.String()
	if strings.HasPrefix(name, "FIELD_LABEL_") {
		return "holonsv1.FieldLabel_" + name
	}
	return fmt.Sprintf("holonsv1.FieldLabel(%d)", value)
}

func literalValue(indent int, write func(*strings.Builder, int)) string {
	var buf strings.Builder
	write(&buf, indent)
	return strings.TrimSuffix(buf.String(), "\n")
}

func writeLine(buf *strings.Builder, indent int, line string) {
	buf.WriteString(goIndent(indent))
	buf.WriteString(line)
	buf.WriteString("\n")
}

func goIndent(indent int) string {
	if indent <= 0 {
		return ""
	}
	return strings.Repeat("\t", indent)
}
