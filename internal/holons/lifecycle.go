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

type Report struct {
	Operation string   `json:"operation"`
	Target    string   `json:"target"`
	Holon     string   `json:"holon"`
	Dir       string   `json:"dir"`
	Manifest  string   `json:"manifest"`
	Runner    string   `json:"runner,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Binary    string   `json:"binary,omitempty"`
	Commands  []string `json:"commands,omitempty"`
	Notes     []string `json:"notes,omitempty"`
}

type runner interface {
	check(*LoadedManifest) error
	build(*LoadedManifest, *Report) error
	test(*LoadedManifest, *Report) error
	clean(*LoadedManifest, *Report) error
}

func ExecuteLifecycle(op Operation, ref string) (Report, error) {
	target, err := ResolveTarget(ref)
	if err != nil {
		return Report{Operation: string(op), Target: normalizedTarget(ref)}, err
	}
	if target.ManifestErr != nil {
		return baseReport(op, target), target.ManifestErr
	}
	if target.Manifest == nil {
		return baseReport(op, target), fmt.Errorf("no %s found in %s", ManifestFileName, target.RelativePath)
	}

	report := baseReport(op, target)
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

	switch op {
	case OperationCheck:
		report.Notes = append(report.Notes, "manifest and prerequisites validated")
		return report, nil
	case OperationBuild:
		err = r.build(target.Manifest, &report)
	case OperationTest:
		err = r.test(target.Manifest, &report)
	case OperationClean:
		err = r.clean(target.Manifest, &report)
	default:
		err = fmt.Errorf("unsupported operation %q", op)
	}

	return report, err
}

func baseReport(op Operation, target *Target) Report {
	holonName := filepath.Base(target.Dir)
	if ref := strings.TrimSpace(target.Ref); ref != "" && ref != "." && !strings.ContainsAny(ref, `/\`) {
		holonName = ref
	}

	report := Report{
		Operation: string(op),
		Target:    normalizedTarget(target.Ref),
		Holon:     holonName,
		Dir:       target.RelativePath,
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
