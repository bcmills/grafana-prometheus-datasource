package dataset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	Seed int64 = 20260917

	ScaleTiny = 200
	Scale10k  = 10_000
	Scale100k = 100_000

	JobName      = "discovery-bench"
	InstanceName = "fixture-0"
	ClusterName  = "prod"
	EnvName      = "bench"

	MetricExact      = "http_requests_total"
	MetricMultiWord  = "http_request_duration_seconds"
	MetricDistractor = "http_responses_total"
	MetricPrefixA    = "prometheus_http_requests_total"
	MetricPrefixB    = "prometheus_http_handler_requests_total"
	MetricInfix      = "go_gc_duration_seconds"
	MetricUTF8       = "http_requests_ütf8_total"
	MetricRare       = "zz_rare_canary_metric"
	MetricExtraHTTP  = "extra_http_requests"

	LabelCanary      = "canary"
	LabelDatasource  = "datasource_uid"
	LabelExtraPrefix = "extra_label_name"
	LabelUTF8        = "label_ütf8"
	LabelInstanceID  = "instance_id"
	LabelPod         = "pod"
	LabelContainer   = "container"
	LabelRoute       = "route"

	ValueUIDAlpha = "handler-grafana-uid-alpha"
	ValueUIDBeta  = "handler-grafana-uid-beta"
	ValueUTF8     = "value_ütf8"
	ValueCanary   = "exact_canary_value"
)

type Shape string

const (
	ShapeMetricHeavy Shape = "metric-heavy"
	ShapeValueHeavy  Shape = "value-heavy"
	ShapeMixed       Shape = "mixed"
)

type Storage string

const (
	StorageActive     Storage = "active"
	StorageHistorical Storage = "historical"
	StorageMixed      Storage = "mixed"
)

type Sample struct {
	Name     string            `json:"name"`
	Labels   map[string]string `json:"labels"`
	Storage  Storage           `json:"storage"`
	Planted  bool              `json:"planted,omitempty"`
	UTF8Name bool              `json:"utf8_name,omitempty"`
}

type TimeWindow struct {
	ID    string    `json:"id"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type SelectorGroup struct {
	ID       string   `json:"id"`
	Matchers []string `json:"matchers"`
}

type JudgedQuery struct {
	ID          string    `json:"id"`
	Endpoint    string    `json:"endpoint"`
	Kind        string    `json:"kind"`
	Term        string    `json:"term"`
	Label       string    `json:"label,omitempty"`
	Relevant    []string  `json:"relevant"`
	Storages    []Storage `json:"storages,omitempty"`
	ExpectedCap int       `json:"expected_population,omitempty"`
}

type Manifest struct {
	Seed            int64                          `json:"seed"`
	GeneratedAt     time.Time                      `json:"generated_at"`
	Scale           int                            `json:"scale"`
	ScaleName       string                         `json:"scale_name"`
	Shape           Shape                          `json:"shape"`
	Storage         Storage                        `json:"storage"`
	SeriesCount     int                            `json:"series_count"`
	ActiveCount     int                            `json:"active_count"`
	HistoricalCount int                            `json:"historical_count"`
	MetricNames     []string                       `json:"metric_names"`
	LabelNames      []string                       `json:"label_names"`
	LabelValues     map[string][]string            `json:"label_values"`
	Windows         []TimeWindow                   `json:"windows"`
	Selectors       []SelectorGroup                `json:"selectors"`
	Judged          []JudgedQuery                  `json:"judged_queries"`
	NamesByWindow   map[string][]string            `json:"metric_names_by_window"`
	ValuesByWindow  map[string]map[string][]string `json:"label_values_by_window"`
}

func ScaleName(n int) string {
	switch n {
	case ScaleTiny:
		return "tiny"
	case Scale10k:
		return "10k"
	case Scale100k:
		return "100k"
	default:
		return "custom"
	}
}

func ProfileDir(scaleName string, shape Shape, storage Storage) string {
	return scaleName + "-" + string(shape) + "-" + string(storage)
}

func WriteManifest(dir string, m *Manifest) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), body, 0o644)
}

func ReadManifest(path string) (*Manifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
