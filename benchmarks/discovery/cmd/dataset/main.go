package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grafana/grafana-prometheus-datasource/benchmarks/discovery/internal/dataset"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: dataset generate|serve|validate [flags]")
	}
	switch args[0] {
	case "generate":
		return generate(args[1:])
	case "serve":
		return serve(args[1:])
	case "validate":
		return validate(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func generate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	scaleName := fs.String("scale", "tiny", "tiny, 10k, or 100k")
	shape := fs.String("shape", "mixed", "metric-heavy, value-heavy, or mixed")
	storage := fs.String("storage", "active", "active, historical, or mixed")
	outDir := fs.String("out", "benchmarks/discovery/out", "output directory")
	allFull := fs.Bool("all-full", false, "generate every 10k and 100k shape and storage combination")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *allFull {
		for _, scale := range []struct {
			name string
			n    int
		}{{"10k", dataset.Scale10k}, {"100k", dataset.Scale100k}} {
			for _, shape := range []dataset.Shape{dataset.ShapeMetricHeavy, dataset.ShapeValueHeavy, dataset.ShapeMixed} {
				for _, storage := range []dataset.Storage{dataset.StorageActive, dataset.StorageHistorical, dataset.StorageMixed} {
					if err := generateOne(*outDir, scale.name, scale.n, shape, storage); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	n, err := parseScale(*scaleName)
	if err != nil {
		return err
	}
	return generateOne(*outDir, *scaleName, n, dataset.Shape(*shape), dataset.Storage(*storage))
}

func generateOne(outDir, scaleName string, n int, shape dataset.Shape, storage dataset.Storage) error {
	samples, manifest, err := dataset.Generate(dataset.GenerateOptions{Scale: n, Shape: shape, Storage: storage})
	if err != nil {
		return err
	}
	dir := filepath.Join(outDir, dataset.ProfileDir(scaleName, shape, storage))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := dataset.WriteManifest(dir, manifest); err != nil {
		return err
	}
	activePath := filepath.Join(dir, "active.prom")
	active, err := os.Create(activePath)
	if err != nil {
		return err
	}
	if err := dataset.WritePrometheusText(active, samples, dataset.StorageActive); err != nil {
		active.Close()
		return err
	}
	if err := active.Close(); err != nil {
		return err
	}
	historicalPath := filepath.Join(dir, "historical.om")
	historical, err := os.Create(historicalPath)
	if err != nil {
		return err
	}
	histTS := manifest.Windows[1].End
	if err := dataset.WriteOpenMetrics(historical, samples, histTS); err != nil {
		historical.Close()
		return err
	}
	if err := historical.Close(); err != nil {
		return err
	}
	fmt.Printf("wrote %s series_count=%d active=%d historical=%d\n", dir, manifest.SeriesCount, manifest.ActiveCount, manifest.HistoricalCount)
	return nil
}

func parseScale(name string) (int, error) {
	switch strings.ToLower(name) {
	case "tiny":
		return dataset.ScaleTiny, nil
	case "10k":
		return dataset.Scale10k, nil
	case "100k":
		return dataset.Scale100k, nil
	default:
		return 0, fmt.Errorf("unknown scale %q", name)
	}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":9115", "listen address")
	metrics := fs.String("metrics-file", "", "path to active.prom")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *metrics == "" {
		return fmt.Errorf("--metrics-file is required")
	}
	fmt.Printf("serving %s on %s/metrics\n", *metrics, *addr)
	return dataset.ServeMetricsFile(*addr, *metrics)
}

func validate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "", "path to manifest.json")
	promURL := fs.String("prometheus", "http://localhost:9090", "Prometheus base URL")
	window := fs.String("window", "active", "active, historical, or mixed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *manifestPath == "" {
		return fmt.Errorf("--manifest is required")
	}
	manifest, err := dataset.ReadManifest(*manifestPath)
	if err != nil {
		return err
	}
	return dataset.ValidateAgainstPrometheus(*promURL, manifest, *window)
}
