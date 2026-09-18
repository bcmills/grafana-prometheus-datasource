package report

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
)

type Trial struct {
	PairID          string   `json:"pair_id"`
	API             string   `json:"api"`
	Endpoint        string   `json:"endpoint"`
	Scale           int      `json:"scale"`
	Shape           string   `json:"shape"`
	Storage         string   `json:"storage"`
	Window          string   `json:"window"`
	QueryKind       string   `json:"query_kind"`
	Term            string   `json:"term"`
	Limit           int      `json:"limit"`
	BatchSize       int      `json:"batch_size"`
	Concurrency     int      `json:"concurrency"`
	CancelMode      string   `json:"cancel_mode"`
	CacheClass      string   `json:"cache_class"`
	Network         string   `json:"network"`
	Mode            string   `json:"mode"`
	Warmup          bool     `json:"warmup"`
	FirstByteNs     int64    `json:"first_byte_ns"`
	FirstBatchNs    int64    `json:"first_batch_ns"`
	CompleteNs      int64    `json:"complete_ns"`
	FirstRenderNs   int64    `json:"first_render_ns,omitempty"`
	CompressedBytes int64    `json:"compressed_bytes"`
	DecodedBytes    int64    `json:"decoded_bytes"`
	ChunkCount      int      `json:"chunk_count"`
	ResultCount     int      `json:"result_count"`
	HasMore         bool     `json:"has_more"`
	Warnings        []string `json:"warnings,omitempty"`
	Results         []string `json:"results,omitempty"`
	CancelLatencyNs int64    `json:"cancel_latency_ns,omitempty"`
	BytesAfterAbort int64    `json:"bytes_after_abort,omitempty"`
	CPUSeconds      float64  `json:"cpu_seconds,omitempty"`
	RSSBytes        int64    `json:"rss_bytes,omitempty"`
	HeapBytes       int64    `json:"heap_bytes,omitempty"`
	Error           string   `json:"error,omitempty"`
	PrecisionAt5    float64  `json:"precision_at_5"`
	PrecisionAt20   float64  `json:"precision_at_20"`
	RecallAt10      float64  `json:"recall_at_10"`
	RecallAt100     float64  `json:"recall_at_100"`
	MRR             float64  `json:"mrr"`
	NDCGAt10        float64  `json:"ndcg_at_10"`
	ExpectedRank    int      `json:"expected_rank"`
	DuplicateCount  int      `json:"duplicate_count"`
	SetEqual        bool     `json:"set_equal"`
}

type Summary struct {
	Group         string  `json:"group"`
	API           string  `json:"api"`
	N             int     `json:"n"`
	ErrorRate     float64 `json:"error_rate"`
	CompleteP50   float64 `json:"complete_p50_ms"`
	CompleteP95   float64 `json:"complete_p95_ms"`
	CompleteP99   float64 `json:"complete_p99_ms"`
	FirstBatchP50 float64 `json:"first_batch_p50_ms"`
	FirstBatchP95 float64 `json:"first_batch_p95_ms"`
	NDCGAt10      float64 `json:"ndcg_at_10_mean"`
	MRR           float64 `json:"mrr_mean"`
	DecodedP50    float64 `json:"decoded_bytes_p50"`
}

type Gate struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

func ReadJSONL(path string) ([]Trial, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var trials []Trial
	for {
		var trial Trial
		if err := dec.Decode(&trial); err != nil {
			if err == io.EOF {
				return trials, nil
			}
			return nil, err
		}
		if !trial.Warmup {
			trials = append(trials, trial)
		}
	}
}

func Summarize(trials []Trial) []Summary {
	groups := map[string][]Trial{}
	for _, trial := range trials {
		key := trial.API + "|" + trial.Endpoint + "|" + trial.QueryKind + "|" + trial.Window
		groups[key] = append(groups[key], trial)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]Summary, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		complete := make([]float64, 0, len(group))
		first := make([]float64, 0, len(group))
		decoded := make([]float64, 0, len(group))
		var ndcg, mrr float64
		errors := 0
		for _, trial := range group {
			if trial.Error != "" {
				errors++
				continue
			}
			complete = append(complete, float64(trial.CompleteNs)/1e6)
			first = append(first, float64(trial.FirstBatchNs)/1e6)
			decoded = append(decoded, float64(trial.DecodedBytes))
			ndcg += trial.NDCGAt10
			mrr += trial.MRR
		}
		ok := len(complete)
		summary := Summary{Group: key, API: group[0].API, N: len(group), ErrorRate: float64(errors) / float64(len(group))}
		if ok > 0 {
			summary.CompleteP50 = percentile(complete, 0.50)
			summary.CompleteP95 = percentile(complete, 0.95)
			summary.CompleteP99 = percentile(complete, 0.99)
			summary.FirstBatchP50 = percentile(first, 0.50)
			summary.FirstBatchP95 = percentile(first, 0.95)
			summary.DecodedP50 = percentile(decoded, 0.50)
			summary.NDCGAt10 = ndcg / float64(ok)
			summary.MRR = mrr / float64(ok)
		}
		out = append(out, summary)
	}
	return out
}

func EvaluateGates(trials []Trial) []Gate {
	search, labels := splitByAPI(trials)
	gates := []Gate{
		ndcgGate(search, labels),
		firstBatchGate(search, labels),
		completionGate(search, labels),
		payloadGate(search, labels),
		hasMoreGate(search),
		cancelGate(search),
		errorGate(trials),
	}
	return gates
}

func splitByAPI(trials []Trial) (search, labels []Trial) {
	for _, trial := range trials {
		if trial.Error != "" {
			continue
		}
		switch trial.API {
		case "search":
			search = append(search, trial)
		case "labels":
			labels = append(labels, trial)
		}
	}
	return search, labels
}

func ndcgGate(search, labels []Trial) Gate {
	s := meanNDCG(search)
	l := meanNDCG(labels)
	passed := s+0.02 >= l || math.IsNaN(l)
	return Gate{Name: "ndcg_at_10", Passed: passed, Detail: fmt.Sprintf("search=%.4f labels=%.4f", s, l)}
}

func firstBatchGate(search, labels []Trial) Gate {
	s := meanDuration(search, func(t Trial) int64 { return t.FirstBatchNs })
	l := meanDuration(labels, func(t Trial) int64 { return t.CompleteNs })
	passed := l == 0 || s <= 0.35*l
	return Gate{Name: "warm_first_batch_vs_labels_complete", Passed: passed, Detail: fmt.Sprintf("search_first_batch=%.1fms labels_complete=%.1fms", s/1e6, l/1e6)}
}

func completionGate(search, labels []Trial) Gate {
	s := percentileNs(search, 0.95, func(t Trial) int64 { return t.CompleteNs })
	l := percentileNs(labels, 0.95, func(t Trial) int64 { return t.CompleteNs })
	passed := l == 0 || s <= 1.25*l
	return Gate{Name: "p95_completion", Passed: passed, Detail: fmt.Sprintf("search=%.1fms labels=%.1fms", s/1e6, l/1e6)}
}

func payloadGate(search, labels []Trial) Gate {
	var sSel, lSel []float64
	for _, trial := range search {
		if trial.QueryKind != "empty" && trial.Term != "" {
			sSel = append(sSel, float64(trial.DecodedBytes))
		}
	}
	for _, trial := range labels {
		if trial.QueryKind != "empty" && trial.Term != "" {
			lSel = append(lSel, float64(trial.DecodedBytes))
		}
	}
	if len(sSel) == 0 || len(lSel) == 0 {
		return Gate{Name: "selective_payload", Passed: true, Detail: "no selective trials"}
	}
	s := percentile(sSel, 0.50)
	l := percentile(lSel, 0.50)
	passed := s <= 0.5*l
	return Gate{Name: "selective_payload", Passed: passed, Detail: fmt.Sprintf("search_p50=%.0f labels_p50=%.0f", s, l)}
}

func hasMoreGate(search []Trial) Gate {
	for _, trial := range search {
		if trial.QueryKind == "empty" && trial.Limit < trial.ResultCount {
			return Gate{Name: "has_more_on_capped_empty", Passed: false, Detail: "result count exceeded limit without a consistent cap"}
		}
		if trial.QueryKind == "empty" && trial.Limit == 10000 && trial.Scale >= 100000 && !trial.HasMore {
			return Gate{Name: "has_more_on_capped_empty", Passed: false, Detail: fmt.Sprintf("100k empty search limit=10000 has_more=%v", trial.HasMore)}
		}
	}
	return Gate{Name: "has_more_on_capped_empty", Passed: true, Detail: "ok"}
}

func cancelGate(search []Trial) Gate {
	for _, trial := range search {
		if trial.CancelMode == "none" || trial.CancelMode == "" {
			continue
		}
		if trial.CancelLatencyNs > 250_000_000 {
			return Gate{Name: "cancel_latency", Passed: false, Detail: fmt.Sprintf("%s cancel_latency=%.1fms", trial.PairID, float64(trial.CancelLatencyNs)/1e6)}
		}
	}
	return Gate{Name: "cancel_latency", Passed: true, Detail: "ok"}
}

func errorGate(trials []Trial) Gate {
	if len(trials) == 0 {
		return Gate{Name: "error_rate", Passed: true, Detail: "no trials"}
	}
	n := 0
	for _, trial := range trials {
		if trial.Error != "" {
			n++
		}
	}
	rate := float64(n) / float64(len(trials))
	return Gate{Name: "error_rate", Passed: rate < 0.01, Detail: fmt.Sprintf("%.2f%%", rate*100)}
}

func meanNDCG(trials []Trial) float64 {
	if len(trials) == 0 {
		return math.NaN()
	}
	var sum float64
	for _, trial := range trials {
		sum += trial.NDCGAt10
	}
	return sum / float64(len(trials))
}

func meanDuration(trials []Trial, fn func(Trial) int64) float64 {
	if len(trials) == 0 {
		return 0
	}
	var sum float64
	for _, trial := range trials {
		sum += float64(fn(trial))
	}
	return sum / float64(len(trials))
}

func percentileNs(trials []Trial, p float64, fn func(Trial) int64) float64 {
	values := make([]float64, 0, len(trials))
	for _, trial := range trials {
		values = append(values, float64(fn(trial)))
	}
	return percentile(values, p)
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	if p <= 0 {
		return values[0]
	}
	if p >= 1 {
		return values[len(values)-1]
	}
	idx := int(math.Ceil(p*float64(len(values)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}
