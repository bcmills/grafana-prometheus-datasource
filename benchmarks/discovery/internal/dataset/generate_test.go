package dataset

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"
)

type frozenClock struct{ t time.Time }

func (c frozenClock) Now() time.Time { return c.t }

func TestGenerateTinyExactCount(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	for _, shape := range []Shape{ShapeMetricHeavy, ShapeValueHeavy, ShapeMixed} {
		for _, storage := range []Storage{StorageActive, StorageHistorical, StorageMixed} {
			t.Run(string(shape)+"/"+string(storage), func(t *testing.T) {
				samples, manifest, err := Generate(GenerateOptions{
					Scale:   ScaleTiny,
					Shape:   shape,
					Storage: storage,
					Clock:   frozenClock{t: now},
				})
				if err != nil {
					t.Fatal(err)
				}
				if len(samples) != ScaleTiny {
					t.Fatalf("series count %d, want %d", len(samples), ScaleTiny)
				}
				if manifest.SeriesCount != ScaleTiny {
					t.Fatalf("manifest series count %d, want %d", manifest.SeriesCount, ScaleTiny)
				}
				if manifest.ActiveCount+manifest.HistoricalCount != ScaleTiny {
					t.Fatalf("active %d + historical %d != %d", manifest.ActiveCount, manifest.HistoricalCount, ScaleTiny)
				}
				if !slices.Contains(manifest.MetricNames, MetricExact) {
					t.Fatal("missing planted exact metric")
				}
				if !slices.Contains(manifest.LabelNames, LabelDatasource) {
					t.Fatal("missing datasource_uid label")
				}
				if storage != StorageHistorical && !slices.Contains(manifest.MetricNames, MetricUTF8) {
					t.Fatal("missing UTF-8 metric on active-capable dataset")
				}
				if storage == StorageMixed && shape == ShapeMetricHeavy {
					if manifest.ActiveCount == 0 || manifest.HistoricalCount == 0 {
						t.Fatalf("mixed storage should split series, active=%d historical=%d", manifest.ActiveCount, manifest.HistoricalCount)
					}
					overlap := intersect(manifest.NamesByWindow["active"], manifest.NamesByWindow["historical"])
					for _, name := range overlap {
						if strings.HasPrefix(name, "bench_metric_") {
							t.Fatalf("generated metric-heavy names should be disjoint, found %s", name)
						}
					}
				}
			})
		}
	}
}

func TestJudgedQueriesIncludeMultiWord(t *testing.T) {
	_, manifest, err := Generate(GenerateOptions{Scale: ScaleTiny, Shape: ShapeMetricHeavy, Storage: StorageActive, Clock: frozenClock{t: time.Unix(0, 0).UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	var found *JudgedQuery
	for i := range manifest.Judged {
		if manifest.Judged[i].ID == "metric-multi-word" {
			found = &manifest.Judged[i]
			break
		}
	}
	if found == nil {
		t.Fatal("missing multi-word judged query")
	}
	if found.Term != "http req" {
		t.Fatalf("term %q", found.Term)
	}
	if !slices.Contains(found.Relevant, MetricExact) {
		t.Fatalf("expected %s in relevant set %v", MetricExact, found.Relevant)
	}
}

func TestNormalizeSearchTerm(t *testing.T) {
	if got := NormalizeSearchTerm("  http   req\t"); got != "httpreq" {
		t.Fatalf("got %q", got)
	}
}

func TestWritePrometheusTextIncludesUTF8(t *testing.T) {
	samples, _, err := Generate(GenerateOptions{Scale: ScaleTiny, Shape: ShapeMetricHeavy, Storage: StorageActive, Clock: frozenClock{t: time.Unix(0, 0).UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := WritePrometheusText(&buf, samples, StorageActive); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), MetricUTF8) {
		t.Fatal("active text format should include UTF-8 metric")
	}
}

func TestWriteOpenMetricsSkipsUTF8(t *testing.T) {
	samples, _, err := Generate(GenerateOptions{Scale: ScaleTiny, Shape: ShapeMetricHeavy, Storage: StorageMixed, Clock: frozenClock{t: time.Unix(0, 0).UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := WriteOpenMetrics(&buf, samples, time.Unix(1_700_000_000, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	if !strings.HasSuffix(strings.TrimSpace(body), "# EOF") {
		t.Fatal("openmetrics must end with # EOF")
	}
	if strings.Contains(body, MetricUTF8) {
		t.Fatal("openmetrics should skip UTF-8 metric names")
	}
}

func TestGenerateRejectsUnknownShape(t *testing.T) {
	_, _, err := Generate(GenerateOptions{Scale: 10, Shape: "nope", Storage: StorageActive})
	if err == nil {
		t.Fatal("expected error")
	}
}

func intersect(a, b []string) []string {
	set := map[string]struct{}{}
	for _, v := range a {
		set[v] = struct{}{}
	}
	var out []string
	for _, v := range b {
		if _, ok := set[v]; ok {
			out = append(out, v)
		}
	}
	return out
}
