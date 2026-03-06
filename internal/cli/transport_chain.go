package cli

import (
	"fmt"
	"strings"

	"github.com/organic-programming/grace-op/internal/holons"
)

// selectTransport determines the best transport for a target holon.
// Priority:
//  1. Already running (known endpoint) -> dial existing
//  2. Same language + loadable -> mem:// (lazy in-process)
//  3. Binary available locally -> stdio:// (ephemeral)
//  4. Network reachable -> tcp://
func selectTransport(holonName string) (scheme string, err error) {
	binaryPath, err := resolveHolon(holonName)
	if err != nil {
		return "", fmt.Errorf("holon not reachable")
	}

	lang, err := readHolonLang(holonName, binaryPath)
	if err == nil && strings.EqualFold(lang, "go") {
		return "mem", nil
	}

	if binaryPath != "" {
		return "stdio", nil
	}

	return "", fmt.Errorf("holon not reachable")
}

func readHolonLang(holonName, binaryPath string) (string, error) {
	target, err := holons.ResolveTarget(holonName)
	if err != nil {
		return "", err
	}
	if target.Identity != nil && target.Identity.Lang != "" {
		return target.Identity.Lang, nil
	}

	return "", fmt.Errorf("holon metadata not found")
}
