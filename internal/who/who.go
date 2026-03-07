package who

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	openv "github.com/organic-programming/grace-op/internal/env"
	sophiapb "github.com/organic-programming/sophia-who/gen/go/sophia_who/v1"
	"github.com/organic-programming/sophia-who/pkg/identity"
	sophiaservice "github.com/organic-programming/sophia-who/pkg/service"
)

// List returns local and cached identities, preserving their origin labels.
func List(root string) (*sophiapb.ListIdentitiesResponse, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	root = filepath.Clean(root)

	var entries []*sophiapb.HolonEntry

	localSeen := map[string]struct{}{}
	appendEntries := func(scanRoot, origin string, dedupe map[string]struct{}) error {
		info, err := os.Stat(scanRoot)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !info.IsDir() {
			return nil
		}

		located, err := identity.FindAllWithPaths(scanRoot)
		if err != nil {
			return err
		}

		for _, holon := range located {
			key := holon.Identity.UUID
			if key == "" {
				key = holon.Path
			}
			if dedupe != nil {
				if _, ok := dedupe[key]; ok {
					continue
				}
				dedupe[key] = struct{}{}
			}

			entries = append(entries, &sophiapb.HolonEntry{
				Identity:     toProto(holon.Identity),
				Origin:       origin,
				RelativePath: relativeHolonDir(scanRoot, holon.Path),
			})
		}
		return nil
	}

	if err := appendEntries(filepath.Join(root, "holons"), "local", localSeen); err != nil {
		return nil, err
	}
	if err := appendEntries(root, "local", localSeen); err != nil {
		return nil, err
	}
	if err := appendEntries(openv.CacheDir(), "cached", nil); err != nil {
		return nil, err
	}

	return &sophiapb.ListIdentitiesResponse{Entries: entries}, nil
}

// Show resolves an identity by UUID or prefix, searching local first then cache.
func Show(target string) (*sophiapb.ShowIdentityResponse, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("uuid is required")
	}

	for _, root := range []string{".", openv.CacheDir()} {
		path, err := identity.FindByUUID(root, target)
		if err != nil {
			if isIdentityNotFound(err) {
				continue
			}
			return nil, err
		}

		id, raw, err := identity.ReadHolonYAML(path)
		if err != nil {
			return nil, err
		}

		return &sophiapb.ShowIdentityResponse{
			Identity:   toProto(id),
			FilePath:   path,
			RawContent: string(raw),
		}, nil
	}

	return nil, fmt.Errorf("holon not found: %s", target)
}

// CreateFromJSON creates an identity from a non-interactive JSON payload.
func CreateFromJSON(raw string) (*sophiapb.CreateIdentityResponse, error) {
	req, err := parseCreateIdentityJSON(raw)
	if err != nil {
		return nil, err
	}

	srv := &sophiaservice.Server{}
	return srv.CreateIdentity(context.Background(), req)
}

// CreateInteractive interactively scaffolds a new identity using stdin/stdout.
func CreateInteractive(in io.Reader, out io.Writer) (*sophiapb.CreateIdentityResponse, error) {
	scanner := bufio.NewScanner(in)
	id := identity.New()

	fmt.Fprintln(out, "─── Sophia Who? — New Holon Identity ───")
	fmt.Fprintf(out, "UUID: %s (generated)\n\n", id.UUID)

	req := &sophiapb.CreateIdentityRequest{}
	req.FamilyName = ask(scanner, out, "Family name (the function — e.g. Transcriber, Prober)")
	req.GivenName = ask(scanner, out, "Given name (the character — e.g. Swift, Deep)")
	req.Composer = ask(scanner, out, "Composer (who is making this decision?)")
	req.Motto = ask(scanner, out, "Motto (the dessein in one sentence)")

	fmt.Fprintln(out, "\nClade (computational nature):")
	for i, clade := range identity.Clades {
		fmt.Fprintf(out, "  %d. %s\n", i+1, clade)
	}
	req.Clade = stringToClade(askChoice(scanner, out, "Choose clade", identity.Clades))

	fmt.Fprintln(out, "\nReproduction mode:")
	for i, reproduction := range identity.ReproductionModes {
		fmt.Fprintf(out, "  %d. %s\n", i+1, reproduction)
	}
	req.Reproduction = stringToReproduction(askChoice(scanner, out, "Choose reproduction mode", identity.ReproductionModes))

	req.Lang = askDefault(scanner, out, "Implementation language", "go")

	aliases := askDefault(scanner, out, "Aliases (comma-separated, or empty)", "")
	if aliases != "" {
		for _, alias := range strings.Split(aliases, ",") {
			if trimmed := strings.TrimSpace(alias); trimmed != "" {
				req.Aliases = append(req.Aliases, trimmed)
			}
		}
	}

	defaultOutputDir := strings.ToLower(req.GivenName + "-" + strings.TrimSuffix(req.FamilyName, "?"))
	defaultOutputDir = strings.ReplaceAll(defaultOutputDir, " ", "-")
	req.OutputDir = askDefault(scanner, out, "Output directory", filepath.Join("holons", defaultOutputDir))

	srv := &sophiaservice.Server{}
	return srv.CreateIdentity(context.Background(), req)
}

func parseCreateIdentityJSON(raw string) (*sophiapb.CreateIdentityRequest, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("json payload is required")
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, err
	}

	req := &sophiapb.CreateIdentityRequest{
		GivenName:    jsonString(payload, "given_name", "givenName"),
		FamilyName:   jsonString(payload, "family_name", "familyName"),
		Motto:        jsonString(payload, "motto"),
		Composer:     jsonString(payload, "composer"),
		Lang:         jsonString(payload, "lang"),
		OutputDir:    jsonString(payload, "output_dir", "outputDir"),
		Aliases:      jsonStringSlice(payload, "aliases"),
		Clade:        stringToClade(jsonString(payload, "clade")),
		Reproduction: stringToReproduction(jsonString(payload, "reproduction")),
	}
	return req, nil
}

func jsonString(payload map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			return value
		}
	}
	return ""
}

func jsonStringSlice(payload map[string]json.RawMessage, keys ...string) []string {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var values []string
		if err := json.Unmarshal(raw, &values); err == nil {
			return values
		}
	}
	return nil
}

func relativeHolonDir(rootDir, holonFilePath string) string {
	dir := filepath.Dir(holonFilePath)
	rel, err := filepath.Rel(rootDir, dir)
	if err != nil {
		return filepath.Clean(dir)
	}
	return filepath.Clean(rel)
}

func isIdentityNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(err.Error())), "holon not found:")
}

func ask(scanner *bufio.Scanner, out io.Writer, prompt string) string {
	for {
		fmt.Fprintf(out, "%s: ", prompt)
		scanner.Scan()
		answer := strings.TrimSpace(scanner.Text())
		if answer != "" {
			return answer
		}
		fmt.Fprintln(out, "  (required)")
	}
}

func askDefault(scanner *bufio.Scanner, out io.Writer, prompt, defaultVal string) string {
	if defaultVal != "" {
		fmt.Fprintf(out, "%s [%s]: ", prompt, defaultVal)
	} else {
		fmt.Fprintf(out, "%s: ", prompt)
	}
	scanner.Scan()
	answer := strings.TrimSpace(scanner.Text())
	if answer == "" {
		return defaultVal
	}
	return answer
}

func askChoice(scanner *bufio.Scanner, out io.Writer, prompt string, choices []string) string {
	for {
		answer := askDefault(scanner, out, prompt, "")
		for _, choice := range choices {
			if strings.EqualFold(answer, choice) {
				return choice
			}
		}
		for i, choice := range choices {
			if fmt.Sprintf("%d", i+1) == answer {
				return choice
			}
		}
		fmt.Fprintln(out, "  (choose a listed value or number)")
	}
}

func toProto(id identity.Identity) *sophiapb.HolonIdentity {
	return &sophiapb.HolonIdentity{
		Uuid:         id.UUID,
		GivenName:    id.GivenName,
		FamilyName:   id.FamilyName,
		Motto:        id.Motto,
		Composer:     id.Composer,
		Clade:        stringToClade(id.Clade),
		Status:       stringToStatus(id.Status),
		Born:         id.Born,
		Parents:      id.Parents,
		Reproduction: stringToReproduction(id.Reproduction),
		Aliases:      id.Aliases,
		GeneratedBy:  id.GeneratedBy,
		Lang:         id.Lang,
		ProtoStatus:  stringToStatus(id.ProtoStatus),
	}
}

func stringToClade(s string) sophiapb.Clade {
	switch strings.TrimSpace(s) {
	case "deterministic/pure":
		return sophiapb.Clade_DETERMINISTIC_PURE
	case "deterministic/stateful":
		return sophiapb.Clade_DETERMINISTIC_STATEFUL
	case "deterministic/io_bound":
		return sophiapb.Clade_DETERMINISTIC_IO_BOUND
	case "probabilistic/generative":
		return sophiapb.Clade_PROBABILISTIC_GENERATIVE
	case "probabilistic/perceptual":
		return sophiapb.Clade_PROBABILISTIC_PERCEPTUAL
	case "probabilistic/adaptive":
		return sophiapb.Clade_PROBABILISTIC_ADAPTIVE
	default:
		return sophiapb.Clade_CLADE_UNSPECIFIED
	}
}

func stringToStatus(s string) sophiapb.Status {
	switch strings.TrimSpace(s) {
	case "draft":
		return sophiapb.Status_DRAFT
	case "stable":
		return sophiapb.Status_STABLE
	case "deprecated":
		return sophiapb.Status_DEPRECATED
	case "dead":
		return sophiapb.Status_DEAD
	default:
		return sophiapb.Status_STATUS_UNSPECIFIED
	}
}

func stringToReproduction(s string) sophiapb.ReproductionMode {
	switch strings.TrimSpace(s) {
	case "manual":
		return sophiapb.ReproductionMode_MANUAL
	case "assisted":
		return sophiapb.ReproductionMode_ASSISTED
	case "automatic":
		return sophiapb.ReproductionMode_AUTOMATIC
	case "autopoietic":
		return sophiapb.ReproductionMode_AUTOPOIETIC
	case "bred":
		return sophiapb.ReproductionMode_BRED
	default:
		return sophiapb.ReproductionMode_REPRODUCTION_UNSPECIFIED
	}
}
