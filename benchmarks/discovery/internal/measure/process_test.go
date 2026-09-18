package measure

import (
	"strings"
	"testing"
)

func TestParseProcessMetrics(t *testing.T) {
	body := `# HELP process_cpu_seconds_total CPU
# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 12.5
process_resident_memory_bytes 4096
`
	sample, err := parseProcessMetrics(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if sample.CPUSeconds != 12.5 {
		t.Fatalf("cpu %v", sample.CPUSeconds)
	}
	if sample.RSSBytes != 4096 {
		t.Fatalf("rss %d", sample.RSSBytes)
	}
}
