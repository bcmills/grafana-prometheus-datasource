package report

import "testing"

func TestNdcgGateAllowsSmallDelta(t *testing.T) {
	trials := []Trial{
		{API: "search", NDCGAt10: 0.99, QueryKind: "exact", CompleteNs: 20_000_000, FirstBatchNs: 5_000_000, DecodedBytes: 100},
		{API: "labels", NDCGAt10: 1.00, QueryKind: "exact", CompleteNs: 40_000_000, FirstBatchNs: 40_000_000, DecodedBytes: 400},
	}
	gates := EvaluateGates(trials)
	byName := map[string]Gate{}
	for _, gate := range gates {
		byName[gate.Name] = gate
	}
	if !byName["ndcg_at_10"].Passed {
		t.Fatalf("%+v", byName["ndcg_at_10"])
	}
	if !byName["p95_completion"].Passed {
		t.Fatalf("%+v", byName["p95_completion"])
	}
}

func TestErrorGate(t *testing.T) {
	trials := make([]Trial, 100)
	for i := range trials {
		trials[i].API = "search"
	}
	trials[0].Error = "boom"
	gates := EvaluateGates(trials)
	for _, gate := range gates {
		if gate.Name == "error_rate" && gate.Passed {
			t.Fatalf("1%% error rate should fail the <1%% gate: %+v", gate)
		}
	}
}

func TestPercentile(t *testing.T) {
	got := percentile([]float64{1, 2, 3, 4}, 0.5)
	if got != 2 && got != 3 {
		t.Fatalf("p50=%v", got)
	}
}
