package measure

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseSearchNDJSON(t *testing.T) {
	body := `{"results":[{"name":"http_requests_total","score":1}]}
{"results":[{"name":"http_request_duration_seconds"}]}
{"status":"success","has_more":true,"warnings":["capped"]}
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	result := Run(t.Context(), server.Client(), Request{
		BaseURL:   server.URL,
		API:       APISearch,
		Endpoint:  EndpointMetricNames,
		Limit:     1000,
		BatchSize: 100,
		Timeout:   5 * time.Second,
	})
	if result.Err != "" {
		t.Fatal(result.Err)
	}
	if len(result.Names) != 2 {
		t.Fatalf("names %v", result.Names)
	}
	if !result.HasMore {
		t.Fatal("expected has_more")
	}
	if result.FirstBatch == 0 || result.Complete == 0 {
		t.Fatalf("missing timings %+v", result)
	}
}

func TestParseLabelsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/v1/label/__name__/values") {
			t.Fatalf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":["up","http_requests_total"]}`))
	}))
	defer server.Close()

	result := Run(t.Context(), server.Client(), Request{
		BaseURL:  server.URL,
		API:      APILabels,
		Endpoint: EndpointMetricNames,
		Limit:    1000,
		Timeout:  5 * time.Second,
	})
	if result.Err != "" {
		t.Fatal(result.Err)
	}
	if len(result.Names) != 2 {
		t.Fatalf("names %v", result.Names)
	}
}

func TestSearchRequestNormalizesTerm(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("search[]")
		_, _ = w.Write([]byte(`{"status":"success","has_more":false}` + "\n"))
	}))
	defer server.Close()

	result := Run(t.Context(), server.Client(), Request{
		BaseURL:  server.URL,
		API:      APISearch,
		Endpoint: EndpointMetricNames,
		Term:     "http req",
		Timeout:  5 * time.Second,
	})
	if result.Err != "" {
		t.Fatal(result.Err)
	}
	if got != "httpreq" {
		t.Fatalf("search term %q", got)
	}
}
