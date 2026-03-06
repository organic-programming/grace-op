package holons

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Operation string

const (
	OperationCheck Operation = "check"
	OperationBuild Operation = "build"
	OperationTest  Operation = "test"
	OperationClean Operation = "clean"
)

// BuildOptions controls target selection, build mode, and dry-run.
// Zero value means: target=runtime GOOS, mode=debug, dry-run=false.
type BuildOptions struct {
	Target string // macos, linux, windows, ios, android
	Mode   string // debug, release, profile
	DryRun bool
}

func (o BuildOptions) resolvedTarget() string {
	if o.Target != "" {
		return o.Target
	}
	return runtimePlatform()
}

func (o BuildOptions) resolvedMode() string {
	if o.Mode != "" {
		return o.Mode
	}
	return "debug"
}

var validTargets = map[string]bool{
	"macos": true, "linux": true, "windows": true,
	"ios": true, "android": true, "darwin": true,
}

var validModes = map[string]bool{
	"debug": true, "release": true, "profile": true,
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
	check(*LoadedManifest) error
	build(*LoadedManifest, *Report) error
	test(*LoadedManifest, *Report) error
	clean(*LoadedManifest, *Report) error
}

// ExecuteLifecycle runs a lifecycle operation on a holon.
// opts is only meaningful for OperationBuild; zero value gives defaults.
func ExecuteLifecycle(op Operation, ref string, opts ...BuildOptions) (Report, error) {
	var bo BuildOptions
	if len(opts) > 0 {
		bo = opts[0]
	}

	// Validate target and mode early.
	if t := bo.resolvedTarget(); !validTargets[t] {
		return Report{Operation: string(op), Target: normalizedTarget(ref)},
			fmt.Errorf("unsupported target %q (supported: macos, linux, windows, ios, android)", t)
	}
	if m := bo.resolvedMode(); !validModes[m] {
		return Report{Operation: string(op), Target: normalizedTarget(ref)},
			fmt.Errorf("unsupported mode %q (supported: debug, release, profile)", m)
	}

	target, err := ResolveTarget(ref)
	if err != nil {
		return Report{Operation: string(op), Target: normalizedTarget(ref)}, err
	}
	if target.ManifestErr != nil {
		return baseReport(op, target, bo), target.ManifestErr
	}
	if target.Manifest == nil {
		return baseReport(op, target, bo), fmt.Errorf("no %s found in %s", ManifestFileName, target.RelativePath)
	}

	report := baseReport(op, target, bo)
	r, err := runnerFor(target.Manifest)
	if err != nil {
		return report, err
	}

	if op != OperationClean {
		if err := preflight(target.Manifest); err != nil {
			return report, err
		}
		if err := r.check(target.Manifest); err != nil {
			return report, err
		}
	}

	if bo.DryRun {
		report.Notes = append(report.Notes, "dry run — no commands executed")
		return report, nil
	}

	switch op {
	case OperationCheck:
		report.Notes = append(report.Notes, "manifest and prerequisites validated")
		return report, nil
	case OperationBuild:
		err = r.build(target.Manifest, &report)
		if err == nil {
			err = verifyArtifact(target.Manifest, &report)
		}
	case OperationTest:
		err = r.test(target.Manifest, &report)
	case OperationClean:
		err = r.clean(target.Manifest, &report)
	default:
		err = fmt.Errorf("unsupported operation %q", op)
	}

	return report, err
}

// verifyArtifact checks the primary artifact exists after build (success contract).
func verifyArtifact(manifest *LoadedManifest, report *Report) error {
	artifactPath := manifest.PrimaryArtifactPath()
	if artifactPath == "" {
		return nil // no artifact declared
	}
	if _, err := os.Stat(artifactPath); err != nil {
		return fmt.Errorf("primary artifact missing after build: %s", workspaceRelativePath(artifactPath))
	}
	report.Artifact = workspaceRelativePath(artifactPath)
	report.Notes = append(report.Notes, fmt.Sprintf("artifact: %s", report.Artifact))
	return nil
}

func baseReport(op Operation, target *Target, bo BuildOptions) Report {
	holonName := filepath.Base(target.Dir)
	if ref := strings.TrimSpace(target.Ref); ref != "" && ref != "." && !strings.ContainsAny(ref, `/\`) {
		holonName = ref
	}

	report := Report{
		Operation:   string(op),
		Target:      normalizedTarget(target.Ref),
		Holon:       holonName,
		Dir:         target.RelativePath,
		BuildTarget: bo.resolvedTarget(),
		BuildMode:   bo.resolvedMode(),
	}
	if target.Manifest != nil {
		report.Manifest = workspaceRelativePath(target.Manifest.Path)
		report.Runner = target.Manifest.Manifest.Build.Runner
		report.Kind = target.Manifest.Manifest.Kind
		report.Binary = workspaceRelativePath(target.Manifest.BinaryPath())
	}
	return report
}

func preflight(manifest *LoadedManifest) error {
	if !manifest.SupportsCurrentPlatform() {
		return fmt.Errorf("platform %q is not supported by %s", runtimePlatform(), workspaceRelativePath(manifest.Path))
	}

	for _, requiredFile := range manifest.Manifest.Requires.Files {
		fullPath := filepath.Join(manifest.Dir, filepath.FromSlash(requiredFile))
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

func runnerFor(manifest *LoadedManifest) (runner, error) {
	switch manifest.Manifest.Build.Runner {
	case RunnerGoModule:
		return goModuleRunner{}, nil
	case RunnerCMake:
		return cmakeRunner{}, nil
	case RunnerRecipe:
		return recipeRunner{}, nil
	default:
		return nil, fmt.Errorf("unsupported runner %q", manifest.Manifest.Build.Runner)
	}
}

type goModuleRunner struct{}

func (goModuleRunner) check(manifest *LoadedManifest) error {
	mainPackage := manifest.GoMainPackage()
	if strings.HasPrefix(mainPackage, ".") {
		fullPath := filepath.Join(manifest.Dir, filepath.FromSlash(mainPackage))
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

func (goModuleRunner) build(manifest *LoadedManifest, report *Report) error {
	if err := os.MkdirAll(filepath.Dir(manifest.BinaryPath()), 0755); err != nil {
		return err
	}

	args := []string{"go", "build", "-o", manifest.BinaryPath(), manifest.GoMainPackage()}
	report.Commands = append(report.Commands, commandString(args))
	if output, err := runCommand(manifest.Dir, args); err != nil {
		return fmt.Errorf("%s\n%s", err, output)
	}

	report.Notes = append(report.Notes, "binary built")
	return nil
}

func (goModuleRunner) test(manifest *LoadedManifest, report *Report) error {
	args := []string{"go", "test", "./..."}
	report.Commands = append(report.Commands, commandString(args))
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

func (cmakeRunner) check(manifest *LoadedManifest) error {
	return nil
}

func (r cmakeRunner) build(manifest *LoadedManifest, report *Report) error {
	if err := os.MkdirAll(manifest.CMakeBuildDir(), 0755); err != nil {
		return err
	}

	binDir := filepath.Join(manifest.Dir, ".op", "build", "bin")
	configureArgs := []string{
		"cmake",
		"-S", ".",
		"-B", manifest.CMakeBuildDir(),
		"-DCMAKE_RUNTIME_OUTPUT_DIRECTORY=" + binDir,
		"-DCMAKE_RUNTIME_OUTPUT_DIRECTORY_DEBUG=" + binDir,
		"-DCMAKE_RUNTIME_OUTPUT_DIRECTORY_RELEASE=" + binDir,
	}
	report.Commands = append(report.Commands, commandString(configureArgs))
	if output, err := runCommand(manifest.Dir, configureArgs); err != nil {
		return fmt.Errorf("%s\n%s", err, output)
	}

	buildArgs := []string{"cmake", "--build", manifest.CMakeBuildDir()}
	report.Commands = append(report.Commands, commandString(buildArgs))
	if output, err := runCommand(manifest.Dir, buildArgs); err != nil {
		return fmt.Errorf("%s\n%s", err, output)
	}

	report.Notes = append(report.Notes, "cmake build complete")
	return nil
}

func (r cmakeRunner) test(manifest *LoadedManifest, report *Report) error {
	if err := r.build(manifest, report); err != nil {
		return err
	}

	listArgs := []string{"ctest", "--test-dir", manifest.CMakeBuildDir(), "-N"}
	report.Commands = append(report.Commands, commandString(listArgs))
	listOutput, err := runCommand(manifest.Dir, listArgs)
	if err != nil {
		return fmt.Errorf("%s\n%s", err, listOutput)
	}
	if strings.Contains(listOutput, "Total Tests: 0") {
		return fmt.Errorf("no tests configured for cmake runner; register tests with enable_testing() and add_test()")
	}

	testArgs := []string{"ctest", "--test-dir", manifest.CMakeBuildDir(), "--output-on-failure"}
	report.Commands = append(report.Commands, commandString(testArgs))
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

func (recipeRunner) check(manifest *LoadedManifest) error {
	// Verify all member paths exist on disk.
	for _, member := range manifest.Manifest.Build.Members {
		memberDir := filepath.Join(manifest.Dir, filepath.FromSlash(member.Path))
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

func (r recipeRunner) build(manifest *LoadedManifest, report *Report) error {
	// Resolve the target from recipe defaults.
	buildTarget := r.resolveTarget(manifest)

	target, ok := manifest.Manifest.Build.Targets[buildTarget]
	if !ok {
		// Try "darwin" as an alias for "macos" on macOS.
		if buildTarget == "darwin" {
			target, ok = manifest.Manifest.Build.Targets["macos"]
		}
		if !ok {
			available := make([]string, 0, len(manifest.Manifest.Build.Targets))
			for name := range manifest.Manifest.Build.Targets {
				available = append(available, name)
			}
			return fmt.Errorf("no recipe target %q (available: %s)", buildTarget, strings.Join(available, ", "))
		}
	}

	memberMap := make(map[string]RecipeMember, len(manifest.Manifest.Build.Members))
	for _, m := range manifest.Manifest.Build.Members {
		memberMap[m.ID] = m
	}

	for i, step := range target.Steps {
		stepLabel := fmt.Sprintf("step %d", i+1)
		if err := r.executeStep(manifest, step, memberMap, report, stepLabel); err != nil {
			return fmt.Errorf("target %q %s: %w", buildTarget, stepLabel, err)
		}
	}

	report.Notes = append(report.Notes, fmt.Sprintf("recipe target %q completed (%d steps)", buildTarget, len(target.Steps)))
	return nil
}

func (r recipeRunner) executeStep(manifest *LoadedManifest, step RecipeStep, members map[string]RecipeMember, report *Report, label string) error {
	switch {
	case step.BuildMember != "":
		return r.stepBuildMember(manifest, step.BuildMember, members, report)
	case step.Exec != nil:
		return r.stepExec(manifest, step.Exec, report)
	case step.Copy != nil:
		return r.stepCopy(manifest, step.Copy, report)
	case step.AssertFile != nil:
		return r.stepAssertFile(manifest, step.AssertFile, report)
	default:
		return fmt.Errorf("%s: empty step (no action defined)", label)
	}
}

// stepBuildMember recursively builds a child holon.
func (recipeRunner) stepBuildMember(manifest *LoadedManifest, memberID string, members map[string]RecipeMember, report *Report) error {
	member, ok := members[memberID]
	if !ok {
		return fmt.Errorf("unknown member %q", memberID)
	}
	if member.Type != "holon" {
		return fmt.Errorf("build_member can only target holon members, %q is %q", memberID, member.Type)
	}

	memberDir := filepath.Join(manifest.Dir, filepath.FromSlash(member.Path))
	childReport, err := ExecuteLifecycle(OperationBuild, memberDir)
	report.Children = append(report.Children, childReport)
	if err != nil {
		return fmt.Errorf("build_member %q: %w", memberID, err)
	}

	report.Notes = append(report.Notes, fmt.Sprintf("built member %q", memberID))
	return nil
}

// stepExec runs an argv command in an explicit working directory.
func (recipeRunner) stepExec(manifest *LoadedManifest, e *RecipeStepExec, report *Report) error {
	if len(e.Argv) == 0 {
		return fmt.Errorf("exec step has empty argv")
	}

	cwd := manifest.Dir
	if e.Cwd != "" {
		cwd = filepath.Join(manifest.Dir, filepath.FromSlash(e.Cwd))
	}

	report.Commands = append(report.Commands, fmt.Sprintf("(cwd=%s) %s", e.Cwd, commandString(e.Argv)))
	if output, err := runCommand(cwd, e.Argv); err != nil {
		return fmt.Errorf("%s\n%s", err, output)
	}

	return nil
}

// stepCopy copies a file from one manifest-relative path to another.
func (recipeRunner) stepCopy(manifest *LoadedManifest, c *RecipeStepCopy, report *Report) error {
	src := filepath.Join(manifest.Dir, filepath.FromSlash(c.From))
	dst := filepath.Join(manifest.Dir, filepath.FromSlash(c.To))

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
func (recipeRunner) stepAssertFile(manifest *LoadedManifest, a *RecipeStepFile, report *Report) error {
	path := filepath.Join(manifest.Dir, filepath.FromSlash(a.Path))
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("assert_file: expected %s but not found", a.Path)
	}
	report.Notes = append(report.Notes, fmt.Sprintf("verified %s", a.Path))
	return nil
}

func (recipeRunner) resolveTarget(manifest *LoadedManifest) string {
	if d := manifest.Manifest.Build.Defaults; d != nil && d.Target != "" {
		return d.Target
	}
	return runtimePlatform()
}

func (recipeRunner) test(manifest *LoadedManifest, report *Report) error {
	// Run op test on each holon member.
	for _, member := range manifest.Manifest.Build.Members {
		if member.Type != "holon" {
			continue
		}
		memberDir := filepath.Join(manifest.Dir, filepath.FromSlash(member.Path))
		childReport, err := ExecuteLifecycle(OperationTest, memberDir)
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
