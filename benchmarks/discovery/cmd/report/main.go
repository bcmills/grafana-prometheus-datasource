package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/grafana/grafana-prometheus-datasource/benchmarks/discovery/internal/report"
)

func main() {
	apiPath := flag.String("api", "benchmarks/discovery/out/api-trials.jsonl", "API trial JSONL")
	uiPath := flag.String("ui", "", "optional UI trial JSONL")
	outDir := flag.String("out", "benchmarks/discovery/out/report", "report directory")
	flag.Parse()

	trials, err := report.ReadJSONL(*apiPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if *uiPath != "" {
		ui, err := report.ReadJSONL(*uiPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		trials = append(trials, ui...)
	}
	if err := report.Write(*outDir, trials); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	gates := report.EvaluateGates(trials)
	failed := 0
	for _, gate := range gates {
		status := "PASS"
		if !gate.Passed {
			status = "FAIL"
			failed++
		}
		fmt.Printf("%s %s %s\n", status, gate.Name, gate.Detail)
	}
	fmt.Printf("wrote %s\n", *outDir)
	if failed > 0 {
		os.Exit(1)
	}
}
