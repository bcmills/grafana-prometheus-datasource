package dataset

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func ValidateAgainstPrometheus(promURL string, manifest *Manifest, windowID string) error {
	window, ok := windowByID(manifest, windowID)
	if !ok {
		return fmt.Errorf("unknown window %q", windowID)
	}
	count, err := instantCount(promURL, `{job="`+JobName+`"}`, window.End)
	if err != nil {
		return err
	}
	want := manifest.ActiveCount
	if windowID == "historical" {
		want = manifest.HistoricalCount
	}
	if windowID == "mixed" {
		want = manifest.SeriesCount
	}
	if count != want {
		return fmt.Errorf("prometheus series count %d, manifest %s count %d", count, windowID, want)
	}
	return nil
}

func windowByID(manifest *Manifest, id string) (TimeWindow, bool) {
	for _, window := range manifest.Windows {
		if window.ID == id {
			return window, true
		}
	}
	return TimeWindow{}, false
}

type promAPIResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value [2]any `json:"value"`
		} `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

func instantCount(promURL, query string, at time.Time) (int, error) {
	u, err := url.Parse(promURL)
	if err != nil {
		return 0, err
	}
	u.Path = "/api/v1/query"
	q := u.Query()
	q.Set("query", "count("+query+")")
	q.Set("time", strconv.FormatInt(at.Unix(), 10))
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus query status %d: %s", resp.StatusCode, body)
	}
	var parsed promAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, err
	}
	if parsed.Status != "success" {
		return 0, fmt.Errorf("prometheus query failed: %s", parsed.Error)
	}
	if len(parsed.Data.Result) == 0 {
		return 0, nil
	}
	raw, ok := parsed.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected count value %#v", parsed.Data.Result[0].Value[1])
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return n, nil
}
