package dataset

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type GenerateOptions struct {
	Scale   int
	Shape   Shape
	Storage Storage
	Clock   Clock
}

func Generate(opts GenerateOptions) ([]Sample, *Manifest, error) {
	if opts.Scale <= 0 {
		return nil, nil, fmt.Errorf("scale must be positive")
	}
	switch opts.Shape {
	case ShapeMetricHeavy, ShapeValueHeavy, ShapeMixed:
	default:
		return nil, nil, fmt.Errorf("unknown shape %q", opts.Shape)
	}
	switch opts.Storage {
	case StorageActive, StorageHistorical, StorageMixed:
	default:
		return nil, nil, fmt.Errorf("unknown storage %q", opts.Storage)
	}
	clock := opts.Clock
	if clock == nil {
		clock = systemClock{}
	}
	now := clock.Now().UTC()
	rng := rand.New(rand.NewPCG(uint64(Seed), uint64(Seed)))

	plants := plantedSamples(opts.Shape, opts.Storage)
	if len(plants) >= opts.Scale {
		return nil, nil, fmt.Errorf("scale %d is smaller than planted series %d", opts.Scale, len(plants))
	}

	fill := opts.Scale - len(plants)
	var generated []Sample
	switch opts.Shape {
	case ShapeMetricHeavy:
		generated = fillMetricHeavy(fill, opts.Storage, rng)
	case ShapeValueHeavy:
		generated = fillValueHeavy(fill, opts.Storage, rng)
	case ShapeMixed:
		generated = fillMixed(fill, opts.Storage, rng)
	}

	samples := append(plants, generated...)
	if len(samples) != opts.Scale {
		return nil, nil, fmt.Errorf("generated %d series, want %d", len(samples), opts.Scale)
	}

	active := 0
	historical := 0
	for i := range samples {
		switch samples[i].Storage {
		case StorageActive:
			active++
		case StorageHistorical:
			historical++
		}
	}

	manifest := buildManifest(opts, now, samples, active, historical)
	return samples, manifest, nil
}

func plantedSamples(shape Shape, storage Storage) []Sample {
	metrics := []struct {
		name     string
		utf8     bool
		storages []Storage
	}{
		{name: MetricExact},
		{name: MetricMultiWord},
		{name: MetricDistractor},
		{name: MetricPrefixA},
		{name: MetricPrefixB},
		{name: MetricInfix},
		{name: MetricRare},
		{name: MetricExtraHTTP},
		{name: MetricUTF8, utf8: true, storages: []Storage{StorageActive, StorageMixed}},
	}

	out := make([]Sample, 0, len(metrics))
	for _, m := range metrics {
		if !storageAllowed(storage, m.storages) {
			continue
		}
		sampleStorage := assignPlantStorage(storage, m.utf8)
		labels := baseLabels(0)
		labels[LabelCanary] = ValueCanary
		out = append(out, Sample{
			Name:     m.name,
			Labels:   labels,
			Storage:  sampleStorage,
			Planted:  true,
			UTF8Name: m.utf8,
		})
	}
	if shape == ShapeValueHeavy {
		// Keep planted metrics as the only extra names on this shape.
		return out
	}
	return out
}

func storageAllowed(storage Storage, allowed []Storage) bool {
	if len(allowed) == 0 {
		return true
	}
	return slices.Contains(allowed, storage)
}

func assignPlantStorage(dataset Storage, utf8 bool) Storage {
	if utf8 {
		return StorageActive
	}
	if dataset == StorageMixed {
		return StorageActive
	}
	return dataset
}

func baseLabels(i int) map[string]string {
	uid := ValueUIDAlpha
	if i%2 == 1 {
		uid = ValueUIDBeta
	}
	labels := map[string]string{
		"cluster":              ClusterName,
		"env":                  EnvName,
		"job":                  JobName,
		"instance":             InstanceName,
		LabelDatasource:        uid,
		LabelInstanceID:        "i0",
		LabelCanary:            "default",
		LabelUTF8:              ValueUTF8,
		LabelExtraPrefix + "1": "default",
		LabelExtraPrefix + "2": "default",
		LabelExtraPrefix + "3": "default",
		LabelExtraPrefix + "4": "default",
		LabelExtraPrefix + "5": "default",
		LabelExtraPrefix + "6": "default",
	}
	return labels
}

func fillMetricHeavy(n int, storage Storage, rng *rand.Rand) []Sample {
	samples := make([]Sample, 0, n)
	for i := 0; i < n; i++ {
		labels := baseLabels(i)
		labels[LabelInstanceID] = "i0"
		samples = append(samples, Sample{
			Name:    fmt.Sprintf("bench_metric_%05d", i),
			Labels:  labels,
			Storage: assignGeneratedStorage(storage, i, rng),
		})
	}
	return samples
}

func fillValueHeavy(n int, storage Storage, rng *rand.Rand) []Sample {
	samples := make([]Sample, 0, n)
	for i := 0; i < n; i++ {
		labels := baseLabels(i)
		labels[LabelInstanceID] = fmt.Sprintf("id_%05d", i)
		if i == 0 {
			labels[LabelInstanceID] = "id_exact_canary"
		}
		if i == 1 {
			labels[LabelInstanceID] = "id_prefix_aaaa"
		}
		samples = append(samples, Sample{
			Name:    "bench_value_heavy_requests_total",
			Labels:  labels,
			Storage: assignGeneratedStorage(storage, i, rng),
		})
	}
	return samples
}

func fillMixed(n int, storage Storage, rng *rand.Rand) []Sample {
	layout := mixedLayout(n)
	samples := make([]Sample, 0, n)
	i := 0
	for m := 0; m < layout.metrics && i < n; m++ {
		for inst := 0; inst < layout.instances && i < n; inst++ {
			for p := 0; p < layout.pods && i < n; p++ {
				for c := 0; c < layout.containers && i < n; c++ {
					for r := 0; r < layout.routes && i < n; r++ {
						labels := baseLabels(i)
						labels[LabelInstanceID] = fmt.Sprintf("inst_%03d", inst)
						labels[LabelPod] = fmt.Sprintf("pod-%d", p)
						labels[LabelContainer] = fmt.Sprintf("container-%d", c)
						labels[LabelRoute] = fmt.Sprintf("/api/route/%d", r)
						samples = append(samples, Sample{
							Name:    fmt.Sprintf("bench_mixed_metric_%03d", m),
							Labels:  labels,
							Storage: assignGeneratedStorage(storage, i, rng),
						})
						i++
					}
				}
			}
		}
	}
	for i < n {
		labels := baseLabels(i)
		labels[LabelInstanceID] = fmt.Sprintf("inst_fill_%d", i)
		labels[LabelPod] = "pod-0"
		labels[LabelContainer] = "container-0"
		labels[LabelRoute] = "/api/route/0"
		samples = append(samples, Sample{
			Name:    "bench_mixed_metric_fill",
			Labels:  labels,
			Storage: assignGeneratedStorage(storage, i, rng),
		})
		i++
	}
	return samples[:n]
}

type mixedDims struct {
	metrics    int
	instances  int
	pods       int
	containers int
	routes     int
}

func mixedLayout(n int) mixedDims {
	metrics := max(8, n/100)
	remaining := max(1, n/metrics)
	instances := max(2, remaining/10)
	remaining = max(1, remaining/instances)
	pods := max(2, remaining/4)
	remaining = max(1, remaining/pods)
	containers := max(1, remaining/2)
	routes := max(1, remaining/containers)
	return mixedDims{metrics: metrics, instances: instances, pods: pods, containers: containers, routes: routes}
}

func assignGeneratedStorage(dataset Storage, i int, _ *rand.Rand) Storage {
	switch dataset {
	case StorageActive:
		return StorageActive
	case StorageHistorical:
		return StorageHistorical
	default:
		if i%2 == 0 {
			return StorageActive
		}
		return StorageHistorical
	}
}

func buildManifest(opts GenerateOptions, now time.Time, samples []Sample, active, historical int) *Manifest {
	windows := []TimeWindow{
		{ID: "active", Start: now.Add(-time.Hour), End: now.Add(time.Minute)},
		{ID: "historical", Start: now.Add(-7 * 24 * time.Hour), End: now.Add(-6 * time.Hour)},
		{ID: "mixed", Start: now.Add(-7 * 24 * time.Hour), End: now.Add(time.Minute)},
	}

	metricSet := map[string]struct{}{}
	labelSet := map[string]struct{}{}
	values := map[string]map[string]struct{}{}
	namesByWindow := map[string]map[string]struct{}{
		"active":     {},
		"historical": {},
		"mixed":      {},
	}
	valuesByWindow := map[string]map[string]map[string]struct{}{
		"active":     {},
		"historical": {},
		"mixed":      {},
	}

	addValue := func(store map[string]map[string]struct{}, label, value string) {
		if store[label] == nil {
			store[label] = map[string]struct{}{}
		}
		store[label][value] = struct{}{}
	}

	for _, sample := range samples {
		metricSet[sample.Name] = struct{}{}
		namesByWindow["mixed"][sample.Name] = struct{}{}
		if sample.Storage == StorageActive {
			namesByWindow["active"][sample.Name] = struct{}{}
		}
		if sample.Storage == StorageHistorical {
			namesByWindow["historical"][sample.Name] = struct{}{}
		}
		for k, v := range sample.Labels {
			labelSet[k] = struct{}{}
			if values[k] == nil {
				values[k] = map[string]struct{}{}
			}
			values[k][v] = struct{}{}
			addValue(valuesByWindow["mixed"], k, v)
			if sample.Storage == StorageActive {
				addValue(valuesByWindow["active"], k, v)
			}
			if sample.Storage == StorageHistorical {
				addValue(valuesByWindow["historical"], k, v)
			}
		}
	}

	m := &Manifest{
		Seed:            Seed,
		GeneratedAt:     now,
		Scale:           opts.Scale,
		ScaleName:       ScaleName(opts.Scale),
		Shape:           opts.Shape,
		Storage:         opts.Storage,
		SeriesCount:     len(samples),
		ActiveCount:     active,
		HistoricalCount: historical,
		MetricNames:     sortedKeys(metricSet),
		LabelNames:      sortedKeys(labelSet),
		LabelValues:     sortedValueMap(values),
		Windows:         windows,
		Selectors: []SelectorGroup{
			{ID: "none"},
			{ID: "one", Matchers: []string{`{cluster="prod"}`}},
			{ID: "several", Matchers: []string{`{cluster="prod",env="bench"}`}},
		},
		NamesByWindow: map[string][]string{
			"active":     sortedKeys(namesByWindow["active"]),
			"historical": sortedKeys(namesByWindow["historical"]),
			"mixed":      sortedKeys(namesByWindow["mixed"]),
		},
		ValuesByWindow: map[string]map[string][]string{
			"active":     sortedValueMap(valuesByWindow["active"]),
			"historical": sortedValueMap(valuesByWindow["historical"]),
			"mixed":      sortedValueMap(valuesByWindow["mixed"]),
		},
	}
	m.Judged = judgedQueries(m)
	return m
}

func judgedQueries(m *Manifest) []JudgedQuery {
	has := func(name string) bool { return slices.Contains(m.MetricNames, name) }
	metricRelevant := func(names ...string) []string {
		out := make([]string, 0, len(names))
		for _, name := range names {
			if has(name) {
				out = append(out, name)
			}
		}
		return out
	}
	labelHas := func(name string) bool { return slices.Contains(m.LabelNames, name) }
	extraLabels := make([]string, 0, 6)
	for i := 1; i <= 6; i++ {
		name := LabelExtraPrefix + strconv.Itoa(i)
		if labelHas(name) {
			extraLabels = append(extraLabels, name)
		}
	}

	queries := []JudgedQuery{
		{ID: "metric-empty", Endpoint: "metric_names", Kind: "empty", Term: "", ExpectedCap: len(m.MetricNames)},
		{ID: "metric-exact", Endpoint: "metric_names", Kind: "exact", Term: MetricExact, Relevant: metricRelevant(MetricExact)},
		{ID: "metric-prefix", Endpoint: "metric_names", Kind: "prefix", Term: "prometheus_http", Relevant: metricRelevant(MetricPrefixA, MetricPrefixB)},
		{ID: "metric-infix", Endpoint: "metric_names", Kind: "infix", Term: "gc_duration", Relevant: metricRelevant(MetricInfix)},
		{ID: "metric-multi-word", Endpoint: "metric_names", Kind: "multi-word", Term: "http req", Relevant: metricRelevant(MetricExact, MetricMultiWord, MetricPrefixA, MetricPrefixB, MetricUTF8, MetricExtraHTTP)},
		{ID: "metric-typo", Endpoint: "metric_names", Kind: "typo", Term: "promethues", Relevant: metricRelevant(MetricPrefixA, MetricPrefixB)},
		{ID: "metric-rare", Endpoint: "metric_names", Kind: "rare", Term: "zz_rare_canary", Relevant: metricRelevant(MetricRare)},
		{ID: "metric-common", Endpoint: "metric_names", Kind: "common", Term: "http", Relevant: metricRelevant(MetricExact, MetricMultiWord, MetricDistractor, MetricPrefixA, MetricPrefixB, MetricUTF8, MetricExtraHTTP)},
		{ID: "metric-utf8", Endpoint: "metric_names", Kind: "utf8", Term: "ütf8", Relevant: metricRelevant(MetricUTF8), Storages: []Storage{StorageActive, StorageMixed}},
		{ID: "metric-no-match", Endpoint: "metric_names", Kind: "no-match", Term: "zzzz_no_such_metric_zzzz", Relevant: []string{}},
		{ID: "label-empty", Endpoint: "label_names", Kind: "empty", Term: "", ExpectedCap: len(m.LabelNames)},
		{ID: "label-exact", Endpoint: "label_names", Kind: "exact", Term: LabelDatasource, Relevant: filterPresent(m.LabelNames, LabelDatasource)},
		{ID: "label-prefix", Endpoint: "label_names", Kind: "prefix", Term: "extra_label", Relevant: extraLabels},
		{ID: "label-multi-word", Endpoint: "label_names", Kind: "multi-word", Term: "extra lab", Relevant: extraLabels},
		{ID: "label-utf8", Endpoint: "label_names", Kind: "utf8", Term: "ütf8", Relevant: filterPresent(m.LabelNames, LabelUTF8), Storages: []Storage{StorageActive, StorageMixed}},
		{ID: "label-no-match", Endpoint: "label_names", Kind: "no-match", Term: "zzzz_no_such_label_zzzz", Relevant: []string{}},
		{
			ID:       "value-exact",
			Endpoint: "label_values",
			Kind:     "exact",
			Term:     ValueUIDAlpha,
			Label:    LabelDatasource,
			Relevant: filterPresent(m.LabelValues[LabelDatasource], ValueUIDAlpha),
		},
		{
			ID:       "value-multi-word",
			Endpoint: "label_values",
			Kind:     "multi-word",
			Term:     "datasource uid",
			Label:    LabelDatasource,
			Relevant: filterPresent(m.LabelValues[LabelDatasource], ValueUIDAlpha, ValueUIDBeta),
		},
		{
			ID:       "value-utf8",
			Endpoint: "label_values",
			Kind:     "utf8",
			Term:     "ütf8",
			Label:    LabelUTF8,
			Relevant: filterPresent(m.LabelValues[LabelUTF8], ValueUTF8),
			Storages: []Storage{StorageActive, StorageMixed},
		},
		{
			ID:       "value-no-match",
			Endpoint: "label_values",
			Kind:     "no-match",
			Term:     "zzzz_no_such_value_zzzz",
			Label:    LabelDatasource,
			Relevant: []string{},
		},
	}
	return queries
}

func filterPresent(haystack []string, needles ...string) []string {
	out := make([]string, 0, len(needles))
	for _, needle := range needles {
		if slices.Contains(haystack, needle) {
			out = append(out, needle)
		}
	}
	return out
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func sortedValueMap(in map[string]map[string]struct{}) map[string][]string {
	out := make(map[string][]string, len(in))
	for k, set := range in {
		out[k] = sortedKeys(set)
	}
	return out
}

func NormalizeSearchTerm(term string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(term)), "")
}
