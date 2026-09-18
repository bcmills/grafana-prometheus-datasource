package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/grafana/grafana-prometheus-datasource/benchmarks/discovery/internal/config"
	"github.com/grafana/grafana-prometheus-datasource/benchmarks/discovery/internal/dataset"
	"github.com/grafana/grafana-prometheus-datasource/benchmarks/discovery/internal/measure"
	"github.com/grafana/grafana-prometheus-datasource/benchmarks/discovery/internal/relevance"
	"github.com/grafana/grafana-prometheus-datasource/benchmarks/discovery/internal/report"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", "benchmarks/discovery/config/scenarios.yaml", "scenario config")
	profileName := flag.String("profile", "smoke", "smoke or full")
	manifestPath := flag.String("manifest", "", "truth manifest.json")
	promURL := flag.String("prometheus", "http://localhost:9090", "Prometheus base URL")
	grafanaURL := flag.String("grafana", "", "optional Grafana origin for plugin-path measurements")
	outPath := flag.String("out", "benchmarks/discovery/out/api-trials.jsonl", "JSONL output")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	profile, err := cfg.Profile(*profileName)
	if err != nil {
		return err
	}
	if *manifestPath == "" {
		return fmt.Errorf("--manifest is required")
	}
	manifest, err := dataset.ReadManifest(*manifestPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(*outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	enc := json.NewEncoder(out)

	client := &http.Client{Timeout: 0}
	rng := rand.New(rand.NewPCG(uint64(cfg.Seed), uint64(cfg.Seed)))
	cases := expand(profile, manifest)
	fmt.Printf("running %d case definitions x %d measured trials\n", len(cases), profile.MeasuredTrials)

	pair := 0
	for _, c := range cases {
		for trialN := 0; trialN < profile.Warmups+profile.MeasuredTrials; trialN++ {
			warmup := trialN < profile.Warmups
			apis := []measure.API{measure.APISearch, measure.APILabels}
			if rng.IntN(2) == 1 {
				apis[0], apis[1] = apis[1], apis[0]
			}
			pair++
			pairID := fmt.Sprintf("p%05d", pair)
			for _, api := range apis {
				baseURL := *promURL
				if *grafanaURL != "" {
					uid := cfg.Grafana.LabelsUID
					if api == measure.APISearch {
						uid = cfg.Grafana.SearchUID
					}
					baseURL = *grafanaURL + "/api/datasources/uid/" + uid + "/resources"
				}
				record, err := runTrial(context.Background(), client, *promURL, manifest, c, api, baseURL, pairID, warmup)
				if err != nil {
					record.Error = err.Error()
				}
				if err := enc.Encode(record); err != nil {
					return err
				}
			}
		}
	}
	fmt.Printf("wrote %s\n", *outPath)
	return nil
}

type benchCase struct {
	endpoint    measure.Endpoint
	window      dataset.TimeWindow
	query       dataset.JudgedQuery
	selector    dataset.SelectorGroup
	limit       int
	batchSize   int
	concurrency int
	cancel      measure.CancelMode
	network     string
	mode        string
	cache       string
}

func expand(profile config.Profile, manifest *dataset.Manifest) []benchCase {
	var cases []benchCase
	for _, endpoint := range profile.Endpoints {
		for _, windowID := range profile.Windows {
			window, ok := windowByID(manifest, windowID)
			if !ok {
				continue
			}
			for _, kind := range profile.QueryKinds {
				query, ok := queryByKind(manifest, endpoint, kind)
				if !ok {
					continue
				}
				if len(query.Storages) > 0 && !slices.Contains(query.Storages, manifest.Storage) {
					continue
				}
				for _, selectorID := range profile.Selectors {
					selector := selectorByID(manifest, selectorID)
					for _, limit := range profile.Limits {
						for _, batch := range profile.BatchSizes {
							for _, conc := range profile.Concurrency {
								for _, cancel := range profile.Cancel {
									for _, network := range profile.Networks {
										for _, mode := range profile.Modes {
											for _, cache := range profile.CacheClasses {
												cases = append(cases, benchCase{
													endpoint:    measure.Endpoint(endpoint),
													window:      window,
													query:       query,
													selector:    selector,
													limit:       limit,
													batchSize:   batch,
													concurrency: conc,
													cancel:      measure.CancelMode(cancel),
													network:     network,
													mode:        mode,
													cache:       cache,
												})
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return cases
}

func windowByID(manifest *dataset.Manifest, id string) (dataset.TimeWindow, bool) {
	for _, window := range manifest.Windows {
		if window.ID == id {
			return window, true
		}
	}
	return dataset.TimeWindow{}, false
}

func queryByKind(manifest *dataset.Manifest, endpoint, kind string) (dataset.JudgedQuery, bool) {
	for _, query := range manifest.Judged {
		if query.Endpoint == endpoint && query.Kind == kind {
			return query, true
		}
	}
	return dataset.JudgedQuery{}, false
}

func selectorByID(manifest *dataset.Manifest, id string) dataset.SelectorGroup {
	for _, selector := range manifest.Selectors {
		if selector.ID == id {
			return selector
		}
	}
	return dataset.SelectorGroup{ID: id}
}

func runTrial(ctx context.Context, client *http.Client, promURL string, manifest *dataset.Manifest, c benchCase, api measure.API, baseURL, pairID string, warmup bool) (report.Trial, error) {
	before, _ := measure.SampleProcess(promURL + "/metrics")
	req := measure.Request{
		BaseURL:     baseURL,
		API:         api,
		Endpoint:    c.endpoint,
		Start:       c.window.Start,
		End:         c.window.End,
		Limit:       c.limit,
		BatchSize:   c.batchSize,
		Term:        c.query.Term,
		Matchers:    c.selector.Matchers,
		Label:       c.query.Label,
		Cancel:      c.cancel,
		IncludeMeta: c.mode == "product" && c.endpoint == measure.EndpointMetricNames,
	}

	var result measure.Result
	if c.concurrency <= 1 {
		result = measure.Run(ctx, client, req)
	} else {
		result = runConcurrent(ctx, client, req, c.concurrency)
	}
	after, _ := measure.SampleProcess(promURL + "/metrics")

	names := result.Names
	if api == measure.APILabels && c.mode == "product" {
		names = relevance.RankLabels(names, c.query.Term)
		if c.limit > 0 && len(names) > c.limit {
			names = names[:c.limit]
		}
	}

	expectedSet := expectedSet(manifest, c)
	scores := relevance.Evaluate(names, c.query.Relevant, c.limit, expectedSet)

	record := report.Trial{
		PairID:          pairID,
		API:             string(api),
		Endpoint:        string(c.endpoint),
		Scale:           manifest.Scale,
		Shape:           string(manifest.Shape),
		Storage:         string(manifest.Storage),
		Window:          c.window.ID,
		QueryKind:       c.query.Kind,
		Term:            c.query.Term,
		Limit:           c.limit,
		BatchSize:       c.batchSize,
		Concurrency:     c.concurrency,
		CancelMode:      string(c.cancel),
		CacheClass:      c.cache,
		Network:         c.network,
		Mode:            c.mode,
		Warmup:          warmup,
		FirstByteNs:     result.FirstByte.Nanoseconds(),
		FirstBatchNs:    result.FirstBatch.Nanoseconds(),
		CompleteNs:      result.Complete.Nanoseconds(),
		CompressedBytes: result.CompressedBytes,
		DecodedBytes:    result.DecodedBytes,
		ChunkCount:      result.ChunkCount,
		ResultCount:     len(names),
		HasMore:         result.HasMore,
		Warnings:        result.Warnings,
		Results:         names,
		CancelLatencyNs: result.CancelLatency.Nanoseconds(),
		BytesAfterAbort: result.BytesAfterAbort,
		CPUSeconds:      after.CPUSeconds - before.CPUSeconds,
		RSSBytes:        after.RSSBytes,
		Error:           result.Err,
		PrecisionAt5:    scores.PrecisionAt5,
		PrecisionAt20:   scores.PrecisionAt20,
		RecallAt10:      scores.RecallAt10,
		RecallAt100:     scores.RecallAt100,
		MRR:             scores.MRR,
		NDCGAt10:        scores.NDCGAt10,
		ExpectedRank:    scores.ExpectedRank,
		DuplicateCount:  scores.DuplicateCount,
		SetEqual:        scores.SetEqual,
	}
	return record, nil
}

func expectedSet(manifest *dataset.Manifest, c benchCase) []string {
	if c.query.Kind == "no-match" {
		return []string{}
	}
	if c.query.Kind != "empty" {
		if len(c.query.Relevant) <= c.limit {
			return c.query.Relevant
		}
		return nil
	}
	switch c.endpoint {
	case measure.EndpointMetricNames:
		names := manifest.NamesByWindow[c.window.ID]
		if len(names) <= c.limit {
			return names
		}
	case measure.EndpointLabelNames:
		if len(manifest.LabelNames) <= c.limit {
			return manifest.LabelNames
		}
	}
	return nil
}

func runConcurrent(ctx context.Context, client *http.Client, req measure.Request, n int) measure.Result {
	var wg sync.WaitGroup
	results := make([]measure.Result, n)
	started := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = measure.Run(ctx, client, req)
		}(i)
	}
	wg.Wait()
	best := results[0]
	for _, result := range results[1:] {
		if result.FirstBatch > 0 && (best.FirstBatch == 0 || result.FirstBatch < best.FirstBatch) {
			best.FirstBatch = result.FirstBatch
		}
		if result.Err != "" {
			best.Err = result.Err
		}
	}
	best.Complete = time.Since(started)
	return best
}
