package identity

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const ManifestFileName = "holon.yaml"

// LocatedIdentity pairs a parsed Identity with the holon.yaml path it came from.
type LocatedIdentity struct {
	Identity Identity
	Path     string
}

// ScanProgress reports scan progress for holon.yaml discovery.
type ScanProgress struct {
	ScannedFiles int
	HolonsFound  int
}

// FindAll scans the directory tree from root for holon.yaml files
// and returns the parsed identities.
func FindAll(root string) ([]Identity, error) {
	located, err := FindAllWithPaths(root)
	if err != nil {
		return nil, err
	}

	holons := make([]Identity, 0, len(located))
	for _, h := range located {
		holons = append(holons, h.Identity)
	}

	return holons, nil
}

// FindAllWithPaths scans the directory tree from root for holon.yaml files
// and returns parsed identities with source file paths.
func FindAllWithPaths(root string) ([]LocatedIdentity, error) {
	var holons []LocatedIdentity

	err := ScanAllWithPaths(root, 0, func(h LocatedIdentity) {
		holons = append(holons, h)
	}, nil)

	return holons, err
}

// ScanAllWithPaths scans the directory tree from root for holon.yaml files.
// Each parsed holon is emitted through onFound as soon as it is discovered.
// If onProgress is provided, it is called periodically and once at the end.
func ScanAllWithPaths(root string, progressEvery int, onFound func(LocatedIdentity), onProgress func(ScanProgress)) error {
	if progressEvery < 0 {
		progressEvery = 0
	}

	scanned := 0
	found := 0
	reportProgress := func(force bool) {
		if onProgress == nil {
			return
		}
		if force || (progressEvery > 0 && scanned > 0 && scanned%progressEvery == 0) {
			onProgress(ScanProgress{
				ScannedFiles: scanned,
				HolonsFound:  found,
			})
		}
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name != "." && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		scanned++
		reportProgress(false)

		if d.Name() != ManifestFileName {
			return nil
		}

		id, _, err := ReadHolonYAML(path)
		if err != nil {
			return nil
		}

		located := LocatedIdentity{
			Identity: id,
			Path:     path,
		}
		found++
		if onFound != nil {
			onFound(located)
		}

		return nil
	})

	if err != nil {
		return err
	}

	reportProgress(true)
	return nil
}

// FindByUUID locates a holon.yaml file by full UUID or prefix.
func FindByUUID(root, target string) (string, error) {
	var found string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != ManifestFileName {
			return nil
		}
		id, _, err := ReadHolonYAML(path)
		if err != nil {
			return nil
		}

		if id.UUID == target || strings.HasPrefix(id.UUID, target) {
			found = path
			return filepath.SkipAll
		}

		return nil
	})

	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("holon not found: %s", target)
	}
	return found, nil
}

// ParseHolonYAML extracts identity fields from a holon.yaml file.
func ParseHolonYAML(data []byte) (Identity, error) {
	var id Identity
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&id); err != nil {
		return Identity{}, fmt.Errorf("YAML parse error: %w", err)
	}
	return id, nil
}

// ReadHolonYAML reads and parses a holon.yaml file.
func ReadHolonYAML(path string) (Identity, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Identity{}, nil, err
	}
	id, err := ParseHolonYAML(data)
	if err != nil {
		return Identity{}, nil, err
	}
	return id, data, nil
}
