package api_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/organic-programming/grace-op/api"
	opv1 "github.com/organic-programming/grace-op/gen/go/op/v1"

	"google.golang.org/protobuf/encoding/protojson"
)

func TestRunCLIVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := api.RunCLIWithVersion([]string{"version"}, "0.1.0-test", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunCLIWithVersion returned %d, want 0", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "op 0.1.0-test" {
		t.Fatalf("version output = %q, want %q", got, "op 0.1.0-test")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCLIHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := api.RunCLI([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunCLI returned %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "Organic Programming CLI") {
		t.Fatalf("help output missing usage banner: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCLIListJSON(t *testing.T) {
	root := t.TempDir()
	writeProtoHolon(t, root)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := api.RunCLI([]string{"--format", "json", "list", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunCLI returned %d, want 0; stderr=%s", code, stderr.String())
	}

	var resp opv1.ListIdentitiesResponse
	if err := protojson.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("invalid list json: %v\noutput=%s", err, stdout.String())
	}
	if len(resp.GetEntries()) != 1 {
		t.Fatalf("entries = %d, want 1", len(resp.GetEntries()))
	}
	if got := resp.GetEntries()[0].GetIdentity().GetGivenName(); got != "Alpha" {
		t.Fatalf("given_name = %q, want %q", got, "Alpha")
	}
}

func TestRunCLICheckJSON(t *testing.T) {
	root := t.TempDir()
	dir := writeProtoHolon(t, root)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := api.RunCLI([]string{"--format", "json", "check", dir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunCLI returned %d, want 0; stderr=%s", code, stderr.String())
	}

	var resp opv1.LifecycleResponse
	if err := protojson.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("invalid check json: %v\noutput=%s", err, stdout.String())
	}
	if got := resp.GetReport().GetOperation(); got != "check" {
		t.Fatalf("operation = %q, want %q", got, "check")
	}
	if got := filepath.Base(resp.GetReport().GetManifest()); got != "holon.proto" {
		t.Fatalf("manifest basename = %q, want %q", got, "holon.proto")
	}
}

func TestRunCLIInspectJSON(t *testing.T) {
	root := t.TempDir()
	dir := writeProtoHolon(t, root)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := api.RunCLI([]string{"inspect", dir, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunCLI returned %d, want 0; stderr=%s", code, stderr.String())
	}

	var resp opv1.InspectResponse
	if err := protojson.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("invalid inspect json: %v\noutput=%s", err, stdout.String())
	}
	if len(resp.GetDocument().GetServices()) != 1 {
		t.Fatalf("services = %d, want 1", len(resp.GetDocument().GetServices()))
	}
	if got := resp.GetDocument().GetServices()[0].GetName(); got != "alpha.v1.AlphaService" {
		t.Fatalf("service name = %q, want %q", got, "alpha.v1.AlphaService")
	}
}

func TestRunCLIModInitJSON(t *testing.T) {
	root := t.TempDir()
	withWorkingDir(t, root)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := api.RunCLI([]string{"--format", "json", "mod", "init", "sample/alpha"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunCLI returned %d, want 0; stderr=%s", code, stderr.String())
	}

	var resp opv1.ModInitResponse
	if err := protojson.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("invalid mod init json: %v\noutput=%s", err, stdout.String())
	}
	if got := filepath.Base(resp.GetModFile()); got != "holon.mod" {
		t.Fatalf("mod file basename = %q, want %q", got, "holon.mod")
	}
	if got := resp.GetHolonPath(); got != "sample/alpha" {
		t.Fatalf("holon_path = %q, want %q", got, "sample/alpha")
	}
}

func TestRunCLITransportDispatchJSON(t *testing.T) {
	root := t.TempDir()
	writeProtoHolon(t, root)

	payload, err := json.Marshal(map[string]string{"rootDir": root})
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := api.RunCLI([]string{"--format", "json", "grpc+mem://op", "ListIdentities", string(payload)}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunCLI returned %d, want 0; stderr=%s", code, stderr.String())
	}

	var resp opv1.ListIdentitiesResponse
	if err := protojson.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("invalid dispatch json: %v\noutput=%s", err, stdout.String())
	}
	if len(resp.GetEntries()) != 1 {
		t.Fatalf("entries = %d, want 1", len(resp.GetEntries()))
	}
	if got := resp.GetEntries()[0].GetIdentity().GetFamilyName(); got != "Service" {
		t.Fatalf("family_name = %q, want %q", got, "Service")
	}
}
