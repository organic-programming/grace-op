package identity

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/dynamic"
	"google.golang.org/protobuf/proto"
)

const (
	ProtoManifestFileName   = "holon.proto"
	manifestExtensionNumber = 50000
)

// Resolved describes the identity source discovered for a holon.
type Resolved struct {
	Identity         Identity
	SourcePath       string
	Description      string
	Skills           []ResolvedSkill
	Sequences        []ResolvedSequence
	Kind             string
	Transport        string
	Platforms        []string
	BuildRunner      string
	RequiredFiles    []string
	RequiredCommands []string
	BuildMain        string
	MemberPaths      []string
	ArtifactBinary   string
	PrimaryArtifact  string
	DelegateCommands []string
}

type ResolvedSkill struct {
	Name        string
	Description string
	When        string
	Steps       []string
}

type ResolvedSequence struct {
	Name        string
	Description string
	Params      []ResolvedSequenceParam
	Steps       []string
}

type ResolvedSequenceParam struct {
	Name        string
	Description string
	Required    bool
	Default     string
}

// Resolve discovers a holon identity from dir, preferring holon.proto and
// falling back to legacy holon.yaml.
func Resolve(dir string) (*Resolved, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve dir %s: %w", dir, err)
	}

	if resolved, err := resolveFromProto(absDir); err == nil {
		return resolved, nil
	}

	yamlPath := filepath.Join(absDir, ManifestFileName)
	id, _, err := ReadHolonYAML(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("no %s or %s found in %s", ProtoManifestFileName, ManifestFileName, absDir)
	}

	return &Resolved{
		Identity:   id,
		SourcePath: yamlPath,
	}, nil
}

// ResolveFromProtoFile extracts a holon identity from a specific holon.proto.
func ResolveFromProtoFile(path string) (*Resolved, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve proto path %s: %w", path, err)
	}
	if filepath.Base(absPath) != ProtoManifestFileName {
		return nil, fmt.Errorf("%s is not a %s file", absPath, ProtoManifestFileName)
	}

	files, err := parseProtoFiles(filepath.Dir(absPath), []string{filepath.Base(absPath)})
	if err != nil {
		return nil, err
	}

	for _, fd := range files {
		if resolved, ok := extractResolved(fd); ok {
			resolved.SourcePath = absPath
			return resolved, nil
		}
	}

	return nil, fmt.Errorf("no manifest extension found in %s", absPath)
}

func resolveFromProto(absDir string) (*Resolved, error) {
	protoFiles, err := collectProtoFiles(absDir)
	if err != nil {
		return nil, err
	}
	if len(protoFiles) == 0 {
		return nil, fmt.Errorf("no proto files found in %s", absDir)
	}

	files, err := parseProtoFiles(absDir, protoFiles)
	if err != nil {
		return nil, err
	}

	for _, fd := range files {
		if resolved, ok := extractResolved(fd); ok {
			resolved.SourcePath = filepath.Join(absDir, filepath.FromSlash(fd.GetName()))
			return resolved, nil
		}
	}

	return nil, fmt.Errorf("no manifest extension found in %s", absDir)
}

func parseProtoFiles(baseDir string, relFiles []string) ([]*desc.FileDescriptor, error) {
	parser := protoparse.Parser{
		ImportPaths:               buildImportPaths(baseDir),
		InferImportPaths:          true,
		IncludeSourceCodeInfo:     false,
		LookupImport:              desc.LoadFileDescriptor,
		AllowExperimentalEditions: true,
	}

	files, err := parser.ParseFiles(relFiles...)
	if err != nil {
		return nil, fmt.Errorf("parse proto files in %s: %w", baseDir, err)
	}
	return files, nil
}

func extractResolved(fd *desc.FileDescriptor) (*Resolved, bool) {
	opts := fd.GetFileOptions()
	if opts == nil {
		return nil, false
	}

	manifestExt := findExtension(fd, manifestExtensionNumber)
	if manifestExt == nil {
		return nil, false
	}

	optsBytes, err := proto.Marshal(opts)
	if err != nil {
		return nil, false
	}

	reg := newExtensionRegistry(fd)
	mf := dynamic.NewMessageFactoryWithExtensionRegistry(reg)

	fileOptsMd, err := desc.LoadMessageDescriptorForMessage(opts)
	if err != nil {
		return nil, false
	}

	dynOpts := mf.NewDynamicMessage(fileOptsMd)
	if err := dynOpts.Unmarshal(optsBytes); err != nil {
		return nil, false
	}

	manifestVal, err := dynOpts.TryGetFieldByNumber(manifestExtensionNumber)
	if err != nil || manifestVal == nil {
		return nil, false
	}

	manifest, ok := manifestVal.(*dynamic.Message)
	if !ok {
		return nil, false
	}

	resolved := resolvedFromDynamic(manifest)
	if resolved.Identity.GivenName == "" && resolved.Identity.FamilyName == "" {
		return nil, false
	}

	return resolved, true
}

func findExtension(fd *desc.FileDescriptor, fieldNum int32) *desc.FieldDescriptor {
	seen := map[string]bool{}
	return findExtensionRecursive(fd, fieldNum, seen)
}

func findExtensionRecursive(fd *desc.FileDescriptor, fieldNum int32, seen map[string]bool) *desc.FieldDescriptor {
	if fd == nil || seen[fd.GetName()] {
		return nil
	}
	seen[fd.GetName()] = true

	for _, ext := range fd.GetExtensions() {
		if ext.GetNumber() == fieldNum {
			return ext
		}
	}
	for _, dep := range fd.GetDependencies() {
		if ext := findExtensionRecursive(dep, fieldNum, seen); ext != nil {
			return ext
		}
	}
	return nil
}

func newExtensionRegistry(fd *desc.FileDescriptor) *dynamic.ExtensionRegistry {
	reg := dynamic.NewExtensionRegistryWithDefaults()
	addExtensions(reg, fd, map[string]bool{})
	return reg
}

func addExtensions(reg *dynamic.ExtensionRegistry, fd *desc.FileDescriptor, seen map[string]bool) {
	if fd == nil || seen[fd.GetName()] {
		return
	}
	seen[fd.GetName()] = true

	for _, ext := range fd.GetExtensions() {
		reg.AddExtension(ext)
	}
	for _, dep := range fd.GetDependencies() {
		addExtensions(reg, dep, seen)
	}
}

func resolvedFromDynamic(manifest *dynamic.Message) *Resolved {
	resolved := &Resolved{}
	resolved.Description = dynString(manifest, 3)
	resolved.Identity.Lang = dynString(manifest, 4)
	resolved.Kind = dynString(manifest, 7)
	resolved.Platforms = dynStringSlice(manifest, 8)
	resolved.Transport = dynString(manifest, 9)

	if ident := dynSubMessage(manifest, 1); ident != nil {
		resolved.Identity.Schema = dynString(ident, 1)
		resolved.Identity.UUID = dynString(ident, 2)
		resolved.Identity.GivenName = dynString(ident, 3)
		resolved.Identity.FamilyName = dynString(ident, 4)
		resolved.Identity.Motto = dynString(ident, 5)
		resolved.Identity.Composer = dynString(ident, 6)
		resolved.Identity.Clade = dynString(ident, 7)
		resolved.Identity.Status = dynString(ident, 8)
		resolved.Identity.Born = dynString(ident, 9)
	}

	if lineage := dynSubMessage(manifest, 2); lineage != nil {
		resolved.Identity.Parents = dynStringSlice(lineage, 1)
		resolved.Identity.Reproduction = dynString(lineage, 2)
		resolved.Identity.GeneratedBy = dynString(lineage, 3)
	}

	if build := dynSubMessage(manifest, 10); build != nil {
		resolved.BuildRunner = dynString(build, 1)
		resolved.BuildMain = dynString(build, 2)
		resolved.MemberPaths = make([]string, 0)
		for _, member := range dynSubMessages(build, 4) {
			if path := strings.TrimSpace(dynString(member, 2)); path != "" {
				resolved.MemberPaths = append(resolved.MemberPaths, path)
			}
		}
	}

	if requires := dynSubMessage(manifest, 11); requires != nil {
		resolved.RequiredCommands = dynStringSlice(requires, 1)
		resolved.RequiredFiles = dynStringSlice(requires, 2)
	}

	if delegates := dynSubMessage(manifest, 12); delegates != nil {
		resolved.DelegateCommands = dynStringSlice(delegates, 1)
	}

	if artifacts := dynSubMessage(manifest, 13); artifacts != nil {
		resolved.ArtifactBinary = dynString(artifacts, 1)
		resolved.PrimaryArtifact = dynString(artifacts, 2)
	}

	resolved.Skills = make([]ResolvedSkill, 0)
	for _, skill := range dynSubMessages(manifest, 5) {
		resolved.Skills = append(resolved.Skills, ResolvedSkill{
			Name:        dynString(skill, 1),
			Description: dynString(skill, 2),
			When:        dynString(skill, 3),
			Steps:       trimNonEmptyStrings(dynStringSlice(skill, 4)),
		})
	}

	resolved.Sequences = make([]ResolvedSequence, 0)
	for _, sequence := range dynSubMessages(manifest, 14) {
		params := make([]ResolvedSequenceParam, 0)
		for _, param := range dynSubMessages(sequence, 3) {
			params = append(params, ResolvedSequenceParam{
				Name:        dynString(param, 1),
				Description: dynString(param, 2),
				Required:    dynBool(param, 3),
				Default:     dynString(param, 4),
			})
		}
		resolved.Sequences = append(resolved.Sequences, ResolvedSequence{
			Name:        dynString(sequence, 1),
			Description: dynString(sequence, 2),
			Params:      params,
			Steps:       trimNonEmptyStrings(dynStringSlice(sequence, 4)),
		})
	}

	resolved.Platforms = compactStrings(resolved.Platforms)
	resolved.RequiredCommands = compactStrings(resolved.RequiredCommands)
	resolved.RequiredFiles = compactStrings(resolved.RequiredFiles)
	resolved.MemberPaths = compactStrings(resolved.MemberPaths)
	resolved.DelegateCommands = compactStrings(resolved.DelegateCommands)
	return resolved
}

func dynBool(msg *dynamic.Message, fieldNum int32) bool {
	val, err := msg.TryGetFieldByNumber(int(fieldNum))
	if err != nil {
		return false
	}
	b, _ := val.(bool)
	return b
}

func dynString(msg *dynamic.Message, fieldNum int32) string {
	val, err := msg.TryGetFieldByNumber(int(fieldNum))
	if err != nil {
		return ""
	}
	s, _ := val.(string)
	return s
}

func dynSubMessage(msg *dynamic.Message, fieldNum int32) *dynamic.Message {
	val, err := msg.TryGetFieldByNumber(int(fieldNum))
	if err != nil {
		return nil
	}
	sub, _ := val.(*dynamic.Message)
	return sub
}

func dynSubMessages(msg *dynamic.Message, fieldNum int32) []*dynamic.Message {
	val, err := msg.TryGetFieldByNumber(int(fieldNum))
	if err != nil || val == nil {
		return nil
	}

	switch typed := val.(type) {
	case []*dynamic.Message:
		return typed
	case []interface{}:
		out := make([]*dynamic.Message, 0, len(typed))
		for _, item := range typed {
			if sub, ok := item.(*dynamic.Message); ok {
				out = append(out, sub)
			}
		}
		return out
	default:
		return nil
	}
}

func dynStringSlice(msg *dynamic.Message, fieldNum int32) []string {
	val, err := msg.TryGetFieldByNumber(int(fieldNum))
	if err != nil || val == nil {
		return nil
	}

	switch typed := val.(type) {
	case []string:
		return compactStrings(typed)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return compactStrings(out)
	default:
		return nil
	}
}

func collectProtoFiles(dir string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".proto" {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan proto dir %s: %w", dir, err)
	}
	sort.Strings(files)
	return files, nil
}

func buildImportPaths(dir string) []string {
	paths := []string{dir}
	seen := map[string]struct{}{dir: {}}

	for current := filepath.Dir(dir); current != "" && current != filepath.Dir(current); current = filepath.Dir(current) {
		candidate := filepath.Join(current, "_protos")
		info, err := os.Stat(candidate)
		if err != nil || !info.IsDir() {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		paths = append(paths, candidate)
		seen[candidate] = struct{}{}
	}

	return paths
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func trimNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
