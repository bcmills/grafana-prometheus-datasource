package relevance

import "testing"

func TestRankLabelsExactFirst(t *testing.T) {
	names := []string{"http_responses_total", "http_requests_total", "go_goroutines"}
	got := RankLabels(names, "http_requests_total")
	if len(got) == 0 || got[0] != "http_requests_total" {
		t.Fatalf("got %v", got)
	}
}

func TestRankLabelsMultiWord(t *testing.T) {
	names := []string{"http_requests_total", "http_request_duration_seconds", "up"}
	got := RankLabels(names, "http req")
	if len(got) < 2 {
		t.Fatalf("got %v", got)
	}
}

func TestJaroWinklerIdentical(t *testing.T) {
	if JaroWinkler("prometheus", "prometheus") != 1 {
		t.Fatal("identical strings must score 1")
	}
}
