package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Write(dir string, trials []Trial) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	summaries := Summarize(trials)
	gates := EvaluateGates(trials)
	if err := writeJSON(filepath.Join(dir, "summary.json"), summaries); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "gates.json"), gates); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(markdown(summaries, gates)), 0o644)
}

func markdown(summaries []Summary, gates []Gate) string {
	var b strings.Builder
	b.WriteString("# Discovery benchmark report\n\n")
	b.WriteString("## Gates\n\n")
	for _, gate := range gates {
		status := "FAIL"
		if gate.Passed {
			status = "PASS"
		}
		fmt.Fprintf(&b, "- %s %s: %s\n", status, gate.Name, gate.Detail)
	}
	b.WriteString("\n## Summaries\n\n")
	b.WriteString("| Group | API | n | error | complete p50 ms | complete p95 ms | first batch p50 ms | nDCG@10 | MRR |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, summary := range summaries {
		fmt.Fprintf(&b, "| %s | %s | %d | %.2f | %.1f | %.1f | %.1f | %.3f | %.3f |\n",
			summary.Group, summary.API, summary.N, summary.ErrorRate, summary.CompleteP50, summary.CompleteP95, summary.FirstBatchP50, summary.NDCGAt10, summary.MRR)
	}
	return b.String()
}

func writeJSON(path string, v any) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}
