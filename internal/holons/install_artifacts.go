package holons

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	openv "github.com/organic-programming/grace-op/internal/env"
)

var applicationsDir = "/Applications"

func isMacAppBundlePath(path string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), ".app")
}

func installedArtifactCandidates(name string) []string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}
	candidates := []string{trimmed}
	if filepath.Ext(trimmed) == "" {
		switch runtime.GOOS {
		case "darwin":
			candidates = append(candidates, trimmed+".app")
		case "windows":
			candidates = append(candidates, trimmed+".exe")
		}
	}
	return uniqueNonEmpty(candidates)
}

func lookupInstalledArtifactInOPBIN(name string) string {
	for _, candidate := range installedArtifactCandidates(name) {
		installed := filepath.Join(openv.OPBIN(), candidate)
		info, err := os.Stat(installed)
		if err != nil {
			continue
		}
		if info.IsDir() && !isMacAppBundlePath(installed) {
			continue
		}
		return installed
	}
	return ""
}

func installNameForArtifact(target *Target, artifactPath string) string {
	if target != nil && target.Manifest != nil && !manifestHasPrimaryArtifact(target.Manifest) {
		if binary := target.Manifest.BinaryName(); binary != "" {
			return binary
		}
	}

	base := ""
	switch {
	case target != nil && target.Dir != "":
		base = filepath.Base(target.Dir)
	case target != nil && strings.TrimSpace(target.Ref) != "":
		base = strings.TrimSpace(target.Ref)
	default:
		base = strings.TrimSpace(filepath.Base(artifactPath))
	}

	if base == "" {
		return base
	}
	if info, err := os.Stat(artifactPath); err == nil && info.IsDir() {
		if isMacAppBundlePath(artifactPath) && filepath.Ext(base) == "" {
			return base + ".app"
		}
		return base
	}
	if ext := filepath.Ext(artifactPath); ext != "" && filepath.Ext(base) == "" {
		return base + ext
	}
	return base
}

func copyArtifact(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	_ = os.RemoveAll(dst)
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		if d.Type()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			return os.Symlink(linkTarget, targetPath)
		}
		return copyFile(path, targetPath)
	})
}

func linkBundleIntoApplications(installedPath string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("--link-applications is only supported on macOS")
	}
	linkPath := filepath.Join(applicationsDir, filepath.Base(installedPath))
	if existing, err := os.Lstat(linkPath); err == nil {
		if existing.Mode()&os.ModeSymlink != 0 || existing.IsDir() {
			if err := os.RemoveAll(linkPath); err != nil {
				return "", err
			}
		} else {
			return "", fmt.Errorf("%s already exists", linkPath)
		}
	}
	if err := os.Symlink(installedPath, linkPath); err != nil {
		return "", err
	}
	return linkPath, nil
}
