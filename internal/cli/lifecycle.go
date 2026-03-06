package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/organic-programming/grace-op/internal/holons"
)

func cmdLifecycle(format Format, operation holons.Operation, args []string) int {
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "op %s: accepts at most one <holon-or-path>\n", operation)
		return 1
	}

	target := "."
	if len(args) == 1 {
		target = args[0]
	}

	report, err := holons.ExecuteLifecycle(operation, target)
	if err != nil {
		if format == FormatJSON {
			type errorReport struct {
				holons.Report
				Error string `json:"error"`
			}
			payload := errorReport{
				Report: report,
				Error:  err.Error(),
			}
			out, marshalErr := json.MarshalIndent(payload, "", "  ")
			if marshalErr == nil {
				fmt.Println(string(out))
			} else {
				fmt.Fprintf(os.Stderr, "op %s: %v\n", operation, err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "op %s: %v\n", operation, err)
		}
		return 1
	}

	fmt.Println(formatLifecycleReport(format, report))
	return 0
}

func formatLifecycleReport(format Format, report holons.Report) string {
	if format == FormatJSON {
		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return "{}"
		}
		return string(out)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Operation: %s\n", report.Operation)
	fmt.Fprintf(&b, "Holon: %s\n", defaultDash(report.Holon))
	fmt.Fprintf(&b, "Dir: %s\n", defaultDash(report.Dir))
	if report.Manifest != "" {
		fmt.Fprintf(&b, "Manifest: %s\n", report.Manifest)
	}
	if report.Runner != "" {
		fmt.Fprintf(&b, "Runner: %s\n", report.Runner)
	}
	if report.Kind != "" {
		fmt.Fprintf(&b, "Kind: %s\n", report.Kind)
	}
	if report.Binary != "" {
		fmt.Fprintf(&b, "Binary: %s\n", report.Binary)
	}
	if len(report.Commands) > 0 {
		b.WriteString("Commands:\n")
		for _, command := range report.Commands {
			fmt.Fprintf(&b, "- %s\n", command)
		}
	}
	if len(report.Notes) > 0 {
		b.WriteString("Notes:\n")
		for _, note := range report.Notes {
			fmt.Fprintf(&b, "- %s\n", note)
		}
	}
	return strings.TrimSpace(b.String())
}
