package holons

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/organic-programming/grace-op/internal/progress"
)

type Operation string

const (
	OperationCheck Operation = "check"
	OperationBuild Operation = "build"
	OperationTest  Operation = "test"
	OperationClean Operation = "clean"

	buildModeDebug   = "debug"
	buildModeRelease = "release"
	buildModeProfile = "profile"
)

// BuildOptions captures CLI/build request overrides before manifest defaults are applied.
type BuildOptions struct {
	Target   string
	Mode     string
	DryRun   bool
	Progress progress.Reporter
}

// BuildContext is the canonical build request used by runners and artifact resolution.
type BuildContext struct {
	Target   string
	Mode     string
	DryRun   bool
	Progress progress.Reporter
}

type Report struct {
	Operation   string   `json:"operation"`
	Target      string   `json:"target"`
	Holon       string   `json:"holon"`
	Dir         string   `json:"dir"`
	Manifest    string   `json:"manifest"`
	Runner      string   `json:"runner,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Binary      string   `json:"binary,omitempty"`
	BuildTarget string   `json:"build_target,omitempty"`
	BuildMode   string   `json:"build_mode,omitempty"`
	Artifact    string   `json:"artifact,omitempty"`
	Commands    []string `json:"commands,omitempty"`
	Notes       []string `json:"notes,omitempty"`
	Children    []Report `json:"children,omitempty"`
}

type runner interface {
	check(*LoadedManifest, BuildContext) error
	build(*LoadedManifest, BuildContext, *Report) error
	test(*LoadedManifest, BuildContext, *Report) error
	clean(*LoadedManifest, *Report) error
}

// ExecuteLifecycle runs a lifecycle operation on a holon.
func ExecuteLifecycle(op Operation, ref string, opts ...BuildOptions) (Report, error) {
	var bo BuildOptions
	if len(opts) > 0 {
		bo = opts[0]
	}
	reporter := bo.Progress
	if reporter == nil {
		reporter = progress.Silence()
	}

	target, err := ResolveTarget(ref)
	if err != nil {
		return Report{Operation: string(op), Target: normalizedTarget(ref)}, err
	}
	if target.ManifestErr != nil {
		return baseReport(op, target, BuildContext{}), target.ManifestErr
	}
	if target.Manifest == nil {
		return baseReport(op, target, BuildContext{}), fmt.Errorf("no %s found in %s", ManifestFileName, target.RelativePath)
	}

	ctx, err := resolveBuildContext(target.Manifest, bo)
	if err != nil {
		return baseReport(op, target, BuildContext{}), err
	}
	if op != OperationBuild {
		ctx.DryRun = false
	}
	ctx.Progress = reporter

	report := baseReport(op, target, ctx)
	r, err := runnerFor(target.Manifest)
	if err != nil {
		return report, err
	}

	if op != OperationClean {
		reporter.Step("checking manifest...")
		reporter.Step("validating prerequisites...")
		if err := preflight(target.Manifest, ctx); err != nil {
			return report, err
		}
		if err := r.check(target.Manifest, ctx); err != nil {
			return report, err
		}
	}

	switch op {
	case OperationCheck:
		report.Notes = append(report.Notes, "manifest and prerequisites validated")
		return report, nil
	case OperationBuild:
		err = r.build(target.Manifest, ctx, &report)
		if err == nil && !isAggregateBuildTarget(ctx.Target) {
			reporter.Step("verifying artifact...")
			err = resolveArtifact(target.Manifest, ctx, &report)
		}
		if err == nil && !ctx.DryRun && !isAggregateBuildTarget(ctx.Target) {
			err = verifyArtifact(target.Manifest, ctx, &report)
		}
		if err == nil && ctx.DryRun {
			report.Notes = append(report.Notes, "dry run — no commands executed")
		}
	case OperationTest:
		err = r.test(target.Manifest, ctx, &report)
	case OperationClean:
		reporter.Step("removing .op/...")
		err = r.clean(target.Manifest, &report)
	default:
		err = fmt.Errorf("unsupported operation %q", op)
	}

	return report, err
}

func ResolveBuildContext(manifest *LoadedManifest, opts BuildOptions) (BuildContext, error) {
	return resolveBuildContext(manifest, opts)
}

func resolveArtifact(manifest *LoadedManifest, ctx BuildContext, report *Report) error {
	artifactPath := manifest.ArtifactPath(ctx)
	if artifactPath == "" {
		return fmt.Errorf("no artifact declared for target %q mode %q", ctx.Target, ctx.Mode)
	}
	report.Artifact = workspaceRelativePath(artifactPath)
	return nil
}

// verifyArtifact checks the primary artifact exists after build (success contract).
func verifyArtifact(manifest *LoadedManifest, ctx BuildContext, report *Report) error {
	artifactPath := manifest.ArtifactPath(ctx)
	if artifactPath == "" {
		return fmt.Errorf("no artifact declared for target %q mode %q", ctx.Target, ctx.Mode)
	}
	if _, err := os.Stat(artifactPath); err != nil {
		return fmt.Errorf("primary artifact missing after build: %s", workspaceRelativePath(artifactPath))
	}
	report.Artifact = workspaceRelativePath(artifactPath)
	report.Notes = append(report.Notes, fmt.Sprintf("artifact: %s", report.Artifact))
	return nil
}

func baseReport(op Operation, target *Target, ctx BuildContext) Report {
	holonName := filepath.Base(target.Dir)
	if ref := strings.TrimSpace(target.Ref); ref != "" && ref != "." && !strings.ContainsAny(ref, `/\`) {
		holonName = ref
	}

	report := Report{
		Operation:   string(op),
		Target:      normalizedTarget(target.Ref),
		Holon:       holonName,
		Dir:         target.RelativePath,
		BuildTarget: ctx.Target,
		BuildMode:   ctx.Mode,
	}
	if target.Manifest != nil {
		report.Manifest = workspaceRelativePath(target.Manifest.Path)
		report.Runner = target.Manifest.Manifest.Build.Runner
		report.Kind = target.Manifest.Manifest.Kind
		if binaryPath := target.Manifest.BinaryPath(); binaryPath != "" {
			report.Binary = workspaceRelativePath(binaryPath)
		}
		if op == OperationBuild {
			if artifactPath := target.Manifest.ArtifactPath(ctx); artifactPath != "" {
				report.Artifact = workspaceRelativePath(artifactPath)
			}
		}
	}
	return report
}

func preflight(manifest *LoadedManifest, ctx BuildContext) error {
	if !isAggregateBuildTarget(ctx.Target) && !manifest.SupportsTarget(ctx.Target) {
		return fmt.Errorf("target %q is not supported by %s", ctx.Target, workspaceRelativePath(manifest.Path))
	}

	for _, requiredFile := range manifest.Manifest.Requires.Files {
		fullPath, err := resolveManifestPattern(manifest.Dir, requiredFile)
		if err != nil {
			return fmt.Errorf("invalid required file %q: %w", requiredFile, err)
		}
		if containsGlob(requiredFile) {
			matches, globErr := filepath.Glob(fullPath)
			if globErr != nil {
				return fmt.Errorf("invalid required file %q: %w", requiredFile, globErr)
			}
			if len(matches) == 0 {
				return fmt.Errorf("missing required file %q (%s)", requiredFile, workspaceRelativePath(fullPath))
			}
			continue
		}
		if _, err := os.Stat(fullPath); err != nil {
			return fmt.Errorf("missing required file %q (%s)", requiredFile, workspaceRelativePath(fullPath))
		}
	}

	for _, command := range requiredCommands(manifest) {
		if _, err := exec.LookPath(command); err != nil {
			return fmt.Errorf("missing required command %q on PATH; %s", command, installHint(command))
		}
	}

	return nil
}

func requiredCommands(manifest *LoadedManifest) []string {
	commands := append([]string{}, manifest.Manifest.Requires.Commands...)
	if manifest.Manifest.Kind == KindWrapper {
		commands = append(commands, manifest.Manifest.Delegates.Commands...)
	}
	return uniqueNonEmpty(commands)
}

func resolveBuildContext(manifest *LoadedManifest, opts BuildOptions) (BuildContext, error) {
	target := strings.TrimSpace(opts.Target)
	if target != "" {
		if strings.EqualFold(target, "darwin") {
			return BuildContext{}, fmt.Errorf("unsupported target %q (supported: macos, linux, windows, ios, ios-simulator, tvos, tvos-simulator, watchos, watchos-simulator, visionos, visionos-simulator, android, all)", target)
		}
		normalizedTarget, err := normalizeBuildTarget(target)
		if err != nil {
			return BuildContext{}, err
		}
		target = normalizedTarget
	} else if defaults := manifest.Manifest.Build.Defaults; defaults != nil && strings.TrimSpace(defaults.Target) != "" {
		target = defaults.Target
	} else {
		target = canonicalRuntimeTarget()
	}

	if isAggregateBuildTarget(target) && manifest.Manifest.Build.Runner != RunnerRecipe {
		return BuildContext{}, fmt.Errorf("target %q is only supported for recipe runners", target)
	}

	mode := strings.TrimSpace(opts.Mode)
	if mode != "" {
		mode = normalizeBuildMode(mode)
		if !isValidBuildMode(mode) {
			return BuildContext{}, fmt.Errorf("unsupported mode %q (supported: debug, release, profile)", opts.Mode)
		}
	} else if defaults := manifest.Manifest.Build.Defaults; defaults != nil && strings.TrimSpace(defaults.Mode) != "" {
		mode = normalizeBuildMode(defaults.Mode)
		if !isValidBuildMode(mode) {
			return BuildContext{}, fmt.Errorf("unsupported mode %q (supported: debug, release, profile)", defaults.Mode)
		}
	} else {
		mode = buildModeDebug
	}

	return BuildContext{
		Target:   target,
		Mode:     mode,
		DryRun:   opts.DryRun,
		Progress: opts.Progress,
	}, nil
}

func runnerFor(manifest *LoadedManifest) (runner, error) {
	if manifest == nil {
		return nil, fmt.Errorf("manifest required")
	}
	name := strings.TrimSpace(manifest.Manifest.Build.Runner)
	r, ok := runnerRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unsupported runner %q", name)
	}
	return r, nil
}

type goModuleRunner struct{}

func (goModuleRunner) check(manifest *LoadedManifest, _ BuildContext) error {
	mainPackage := manifest.GoMainPackage()
	if strings.HasPrefix(mainPackage, ".") {
		fullPath, err := manifest.ResolveManifestPath(mainPackage)
		if err != nil {
			return fmt.Errorf("go main package %q: %w", mainPackage, err)
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf("go main package %q not found (%s)", mainPackage, workspaceRelativePath(fullPath))
		}
		if !info.IsDir() {
			return fmt.Errorf("go main package %q must be a directory", mainPackage)
		}
	}
	return nil
}

func (goModuleRunner) build(manifest *LoadedManifest, ctx BuildContext, report *Report) error {
	if err := ensureHostBuildTarget(RunnerGoModule, ctx); err != nil {
		return err
	}

	binaryPath := manifest.BinaryPath()
	args := []string{"go", "build", "-o", binaryPath, manifest.GoMainPackage()}
	report.Commands = append(report.Commands, commandString(args))
	ctx.Progress.Step(commandString(args))
	if ctx.DryRun {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		return err
	}
	if output, err := runCommand(manifest.Dir, args); err != nil {
		return fmt.Errorf("%s\n%s", err, output)
	}

	report.Notes = append(report.Notes, "binary built")
	return nil
}

func (goModuleRunner) test(manifest *LoadedManifest, ctx BuildContext, report *Report) error {
	if err := ensureHostBuildTarget(RunnerGoModule, ctx); err != nil {
		return err
	}
	args := []string{"go", "test", "./..."}
	report.Commands = append(report.Commands, commandString(args))
	ctx.Progress.Step(commandString(args))
	if output, err := runCommand(manifest.Dir, args); err != nil {
		return fmt.Errorf("%s\n%s", err, output)
	}

	report.Notes = append(report.Notes, "tests passed")
	return nil
}

func (goModuleRunner) clean(manifest *LoadedManifest, report *Report) error {
	if err := os.RemoveAll(manifest.OpRoot()); err != nil {
		return err
	}
	report.Notes = append(report.Notes, "removed .op/")
	return nil
}

type cmakeRunner struct{}

func (cmakeRunner) check(manifest *LoadedManifest, _ BuildContext) error {
	return nil
}

func (r cmakeRunner) build(manifest *LoadedManifest, ctx BuildContext, report *Report) error {
	if err := ensureHostBuildTarget(RunnerCMake, ctx); err != nil {
		return err
	}

	config := cmakeBuildConfig(ctx.Mode)
	binDir := filepath.Join(manifest.Dir, ".op", "build", "bin")
	configureArgs := []string{
		"cmake",
		"-S", ".",
		"-B", manifest.CMakeBuildDir(),
		"-DCMAKE_BUILD_TYPE=" + config,
		"-DCMAKE_RUNTIME_OUTPUT_DIRECTORY=" + binDir,
		"-DCMAKE_RUNTIME_OUTPUT_DIRECTORY_DEBUG=" + binDir,
		"-DCMAKE_RUNTIME_OUTPUT_DIRECTORY_RELEASE=" + binDir,
		"-DCMAKE_RUNTIME_OUTPUT_DIRECTORY_RELWITHDEBINFO=" + binDir,
	}
	report.Commands = append(report.Commands, commandString(configureArgs))
	buildArgs := []string{"cmake", "--build", manifest.CMakeBuildDir(), "--config", config}
	report.Commands = append(report.Commands, commandString(buildArgs))
	if ctx.DryRun {
		return nil
	}
	if err := os.MkdirAll(manifest.CMakeBuildDir(), 0755); err != nil {
		return err
	}
	ctx.Progress.Step(commandString(configureArgs))
	if output, err := runCommand(manifest.Dir, configureArgs); err != nil {
		return fmt.Errorf("%s\n%s", err, output)
	}
	ctx.Progress.Step(commandString(buildArgs))
	if output, err := runCommand(manifest.Dir, buildArgs); err != nil {
		return fmt.Errorf("%s\n%s", err, output)
	}

	report.Notes = append(report.Notes, "cmake build complete")
	return nil
}

func (r cmakeRunner) test(manifest *LoadedManifest, ctx BuildContext, report *Report) error {
	if err := r.build(manifest, ctx, report); err != nil {
		return err
	}

	config := cmakeBuildConfig(ctx.Mode)
	listArgs := []string{"ctest", "--test-dir", manifest.CMakeBuildDir(), "-N", "-C", config}
	report.Commands = append(report.Commands, commandString(listArgs))
	ctx.Progress.Step(commandString(listArgs))
	listOutput, err := runCommand(manifest.Dir, listArgs)
	if err != nil {
		return fmt.Errorf("%s\n%s", err, listOutput)
	}
	if strings.Contains(listOutput, "Total Tests: 0") {
		return fmt.Errorf("no tests configured for cmake runner; register tests with enable_testing() and add_test()")
	}

	testArgs := []string{"ctest", "--test-dir", manifest.CMakeBuildDir(), "--output-on-failure", "-C", config}
	report.Commands = append(report.Commands, commandString(testArgs))
	ctx.Progress.Step(commandString(testArgs))
	if output, err := runCommand(manifest.Dir, testArgs); err != nil {
		return fmt.Errorf("%s\n%s", err, output)
	}

	report.Notes = append(report.Notes, "ctest passed")
	return nil
}

func (cmakeRunner) clean(manifest *LoadedManifest, report *Report) error {
	if err := os.RemoveAll(manifest.OpRoot()); err != nil {
		return err
	}
	report.Notes = append(report.Notes, "removed .op/")
	return nil
}

// --- Recipe runner (composite holons) ---

type recipeRunner struct{}

func (recipeRunner) check(manifest *LoadedManifest, ctx BuildContext) error {
	if isAggregateBuildTarget(ctx.Target) {
		if len(manifest.Manifest.Build.Targets) == 0 {
			return fmt.Errorf("no recipe targets declared")
		}
		return nil
	}
	if _, ok := manifest.Manifest.Build.Targets[ctx.Target]; !ok {
		return fmt.Errorf("no recipe target %q", ctx.Target)
	}
	// Verify all member paths exist on disk.
	for _, member := range manifest.Manifest.Build.Members {
		memberDir, err := manifest.ResolveManifestPath(member.Path)
		if err != nil {
			return fmt.Errorf("recipe member %q path: %w", member.ID, err)
		}
		if _, err := os.Stat(memberDir); err != nil {
			return fmt.Errorf("recipe member %q path not found: %s", member.ID, memberDir)
		}
		// If the member is a holon, its holon.yaml must exist.
		if member.Type == "holon" {
			manifestPath := filepath.Join(memberDir, ManifestFileName)
			if _, err := os.Stat(manifestPath); err != nil {
				return fmt.Errorf("recipe member %q (type=holon) missing %s in %s", member.ID, ManifestFileName, memberDir)
			}
		}
	}
	return nil
}

func (r recipeRunner) build(manifest *LoadedManifest, ctx BuildContext, report *Report) error {
	if isAggregateBuildTarget(ctx.Target) {
		targets := sortedRecipeTargets(manifest)
		if len(targets) == 0 {
			return fmt.Errorf("no recipe targets declared")
		}

		for _, name := range targets {
			ctx.Progress.Step("building target: " + name)
			childReport, err := ExecuteLifecycle(OperationBuild, manifest.Dir, BuildOptions{
				Target:   name,
				Mode:     ctx.Mode,
				DryRun:   ctx.DryRun,
				Progress: ctx.Progress.Child(),
			})
			report.Children = append(report.Children, childReport)
			if err != nil {
				return fmt.Errorf("recipe target %q: %w", name, err)
			}
		}

		report.Notes = append(report.Notes, fmt.Sprintf("recipe aggregate build covered targets: %s", strings.Join(targets, ", ")))
		return nil
	}

	target, ok := manifest.Manifest.Build.Targets[ctx.Target]
	if !ok {
		available := make([]string, 0, len(manifest.Manifest.Build.Targets))
		for name := range manifest.Manifest.Build.Targets {
			available = append(available, name)
		}
		return fmt.Errorf("no recipe target %q (available: %s)", ctx.Target, strings.Join(available, ", "))
	}

	memberMap := make(map[string]RecipeMember, len(manifest.Manifest.Build.Members))
	for _, m := range manifest.Manifest.Build.Members {
		memberMap[m.ID] = m
	}

	for i, step := range target.Steps {
		stepLabel := fmt.Sprintf("step %d", i+1)
		if err := r.executeStep(manifest, ctx, step, memberMap, report, stepLabel); err != nil {
			return fmt.Errorf("target %q %s: %w", ctx.Target, stepLabel, err)
		}
	}

	if !ctx.DryRun {
		report.Notes = append(report.Notes, fmt.Sprintf("recipe target %q completed (%d steps)", ctx.Target, len(target.Steps)))
	}
	return nil
}

func (r recipeRunner) executeStep(manifest *LoadedManifest, ctx BuildContext, step RecipeStep, members map[string]RecipeMember, report *Report, label string) error {
	switch {
	case step.BuildMember != "":
		return r.stepBuildMember(manifest, ctx, step.BuildMember, members, report)
	case step.Exec != nil:
		return r.stepExec(manifest, ctx, step.Exec, report)
	case step.Copy != nil:
		return r.stepCopy(manifest, ctx, step.Copy, report)
	case step.AssertFile != nil:
		return r.stepAssertFile(manifest, ctx, step.AssertFile, report)
	default:
		return fmt.Errorf("%s: empty step (no action defined)", label)
	}
}

// stepBuildMember recursively builds a child holon.
func (recipeRunner) stepBuildMember(manifest *LoadedManifest, ctx BuildContext, memberID string, members map[string]RecipeMember, report *Report) error {
	member, ok := members[memberID]
	if !ok {
		return fmt.Errorf("unknown member %q", memberID)
	}
	if member.Type != "holon" {
		return fmt.Errorf("build_member can only target holon members, %q is %q", memberID, member.Type)
	}

	memberDir, err := manifest.ResolveManifestPath(member.Path)
	if err != nil {
		return fmt.Errorf("build_member %q path: %w", memberID, err)
	}
	if resolved, err := filepath.EvalSymlinks(memberDir); err == nil {
		memberDir = resolved
	}
	report.Commands = append(report.Commands, "build_member "+memberID)
	ctx.Progress.Step("building member: " + memberID)
	childReport, err := ExecuteLifecycle(OperationBuild, memberDir, BuildOptions{
		Target:   ctx.Target,
		Mode:     ctx.Mode,
		DryRun:   ctx.DryRun,
		Progress: ctx.Progress.Child(),
	})
	report.Children = append(report.Children, childReport)
	if err != nil {
		return fmt.Errorf("build_member %q: %w", memberID, err)
	}

	if !ctx.DryRun {
		report.Notes = append(report.Notes, fmt.Sprintf("built member %q", memberID))
	}
	return nil
}

// stepExec runs an argv command in an explicit working directory.
func (recipeRunner) stepExec(manifest *LoadedManifest, ctx BuildContext, e *RecipeStepExec, report *Report) error {
	if len(e.Argv) == 0 {
		return fmt.Errorf("exec step has empty argv")
	}

	cwd, err := manifest.ResolveManifestPath(e.Cwd)
	if err != nil {
		return fmt.Errorf("exec cwd %q: %w", e.Cwd, err)
	}
	report.Commands = append(report.Commands, fmt.Sprintf("(cwd=%s) %s", manifestRelativePath(manifest, cwd), commandString(e.Argv)))
	ctx.Progress.Step(commandString(e.Argv))
	if ctx.DryRun {
		return nil
	}
	if output, err := runCommand(cwd, e.Argv); err != nil {
		return fmt.Errorf("%s\n%s", err, output)
	}

	return nil
}

// stepCopy copies a file from one manifest-relative path to another.
func (recipeRunner) stepCopy(manifest *LoadedManifest, ctx BuildContext, c *RecipeStepCopy, report *Report) error {
	src, err := manifest.ResolveManifestPath(c.From)
	if err != nil {
		return fmt.Errorf("copy from %q: %w", c.From, err)
	}
	dst, err := manifest.ResolveManifestPath(c.To)
	if err != nil {
		return fmt.Errorf("copy to %q: %w", c.To, err)
	}
	report.Commands = append(report.Commands, fmt.Sprintf("copy %s -> %s", c.From, c.To))
	ctx.Progress.Step(fmt.Sprintf("copy %s -> %s", c.From, c.To))
	if ctx.DryRun {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("copy: create dir for %s: %w", c.To, err)
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("copy: read %s: %w", c.From, err)
	}
	// Preserve the source file's permissions.
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("copy: stat %s: %w", c.From, err)
	}
	if err := os.WriteFile(dst, data, info.Mode()); err != nil {
		return fmt.Errorf("copy: write %s: %w", c.To, err)
	}

	report.Notes = append(report.Notes, fmt.Sprintf("copied %s → %s", c.From, c.To))
	return nil
}

// stepAssertFile verifies a manifest-relative file exists.
func (recipeRunner) stepAssertFile(manifest *LoadedManifest, ctx BuildContext, a *RecipeStepFile, report *Report) error {
	path, err := manifest.ResolveManifestPath(a.Path)
	if err != nil {
		return fmt.Errorf("assert_file path %q: %w", a.Path, err)
	}
	report.Commands = append(report.Commands, "assert_file "+a.Path)
	ctx.Progress.Step("assert_file " + a.Path)
	if ctx.DryRun {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("assert_file: expected %s but not found", a.Path)
	}
	report.Notes = append(report.Notes, fmt.Sprintf("verified %s", a.Path))
	return nil
}

func (recipeRunner) test(manifest *LoadedManifest, ctx BuildContext, report *Report) error {
	// Run op test on each holon member.
	for _, member := range manifest.Manifest.Build.Members {
		if member.Type != "holon" {
			continue
		}
		memberDir, err := manifest.ResolveManifestPath(member.Path)
		if err != nil {
			return fmt.Errorf("test member %q path: %w", member.ID, err)
		}
		childReport, err := ExecuteLifecycle(OperationTest, memberDir, BuildOptions{
			Target: ctx.Target,
			Mode:   ctx.Mode,
		})
		report.Children = append(report.Children, childReport)
		if err != nil {
			return fmt.Errorf("test member %q: %w", member.ID, err)
		}
	}
	report.Notes = append(report.Notes, "all holon members tested")
	return nil
}

func (recipeRunner) clean(manifest *LoadedManifest, report *Report) error {
	if err := os.RemoveAll(manifest.OpRoot()); err != nil {
		return err
	}
	report.Notes = append(report.Notes, "removed .op/")
	return nil
}

func runCommand(dir string, args []string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed: %w", commandString(args), err)
	}
	return string(output), nil
}

func commandString(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t") {
			quoted = append(quoted, fmt.Sprintf("%q", arg))
			continue
		}
		quoted = append(quoted, arg)
	}
	return strings.Join(quoted, " ")
}

func normalizedTarget(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "."
	}
	return ref
}

func normalizeBuildTarget(target string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(target))
	switch normalized {
	case "darwin", "macos":
		return "macos", nil
	case "linux", "windows", "ios", "ios-simulator", "tvos", "tvos-simulator", "watchos", "watchos-simulator", "visionos", "visionos-simulator", "android", "all":
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported target %q (supported: macos, linux, windows, ios, ios-simulator, tvos, tvos-simulator, watchos, watchos-simulator, visionos, visionos-simulator, android, all)", target)
	}
}

func normalizePlatformName(platform string) string {
	normalized, err := normalizeBuildTarget(platform)
	if err == nil {
		return normalized
	}
	return strings.ToLower(strings.TrimSpace(platform))
}

func normalizeBuildMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
}

func isValidBuildMode(mode string) bool {
	switch normalizeBuildMode(mode) {
	case buildModeDebug, buildModeRelease, buildModeProfile:
		return true
	default:
		return false
	}
}

func isAggregateBuildTarget(target string) bool {
	return strings.EqualFold(strings.TrimSpace(target), "all")
}

func sortedRecipeTargets(manifest *LoadedManifest) []string {
	targets := make([]string, 0, len(manifest.Manifest.Build.Targets))
	for name := range manifest.Manifest.Build.Targets {
		targets = append(targets, name)
	}

	order := map[string]int{
		"macos":              10,
		"ios":                20,
		"ios-simulator":      21,
		"tvos":               30,
		"tvos-simulator":     31,
		"watchos":            40,
		"watchos-simulator":  41,
		"visionos":           50,
		"visionos-simulator": 51,
		"android":            60,
		"linux":              70,
		"windows":            80,
	}

	sort.Slice(targets, func(i, j int) bool {
		left := order[targets[i]]
		right := order[targets[j]]
		if left != right {
			return left < right
		}
		return targets[i] < targets[j]
	})
	return targets
}

func canonicalRuntimeTarget() string {
	switch runtimePlatform() {
	case "darwin":
		return "macos"
	default:
		return runtimePlatform()
	}
}

func ensureHostBuildTarget(runnerName string, ctx BuildContext) error {
	if ctx.Target == canonicalRuntimeTarget() {
		return nil
	}
	return fmt.Errorf("%s cross-target build not implemented (requested %q on host %q)", runnerName, ctx.Target, canonicalRuntimeTarget())
}

func cmakeBuildConfig(mode string) string {
	switch normalizeBuildMode(mode) {
	case buildModeRelease:
		return "Release"
	case buildModeProfile:
		return "RelWithDebInfo"
	default:
		return "Debug"
	}
}

func manifestRelativePath(manifest *LoadedManifest, absPath string) string {
	rel, err := filepath.Rel(manifest.Dir, absPath)
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}

func installHint(command string) string {
	switch runtimePlatform() {
	case "darwin":
		switch command {
		case "ctest":
			return "install CMake, which provides ctest (`brew install cmake`)"
		default:
			return fmt.Sprintf("install it with Homebrew if available (`brew install %s`)", command)
		}
	case "linux":
		switch command {
		case "ctest":
			return "install CMake, which provides ctest, with your distribution package manager"
		default:
			return fmt.Sprintf("install it with your distribution package manager (for example `%s`)", linuxInstallExample(command))
		}
	case "windows":
		switch command {
		case "ctest":
			return "install CMake and ensure `ctest.exe` is on PATH"
		default:
			return fmt.Sprintf("install %s and ensure it is on PATH", command)
		}
	default:
		return fmt.Sprintf("install %s and ensure it is on PATH", command)
	}
}

func linuxInstallExample(command string) string {
	switch command {
	case "go":
		return "sudo apt install golang-go"
	case "cmake", "ctest":
		return "sudo apt install cmake"
	default:
		return "sudo apt install " + command
	}
}

func runtimePlatform() string {
	return runtime.GOOS
}
