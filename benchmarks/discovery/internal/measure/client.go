package measure

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type API string

const (
	APISearch API = "search"
	APILabels API = "labels"
)

type Endpoint string

const (
	EndpointMetricNames Endpoint = "metric_names"
	EndpointLabelNames  Endpoint = "label_names"
	EndpointLabelValues Endpoint = "label_values"
)

type CancelMode string

const (
	CancelNone            CancelMode = "none"
	CancelBeforeHeaders   CancelMode = "before_headers"
	CancelAfterFirstBatch CancelMode = "after_first_batch"
	CancelHalfway         CancelMode = "halfway"
)

type Request struct {
	BaseURL     string
	API         API
	Endpoint    Endpoint
	Start       time.Time
	End         time.Time
	Limit       int
	BatchSize   int
	Term        string
	Matchers    []string
	Label       string
	Cancel      CancelMode
	Timeout     time.Duration
	IncludeMeta bool
}

type Result struct {
	FirstByte       time.Duration
	FirstBatch      time.Duration
	Complete        time.Duration
	CompressedBytes int64
	DecodedBytes    int64
	ChunkCount      int
	ChunkBytes      []int
	Names           []string
	HasMore         bool
	Warnings        []string
	CancelLatency   time.Duration
	BytesAfterAbort int64
	Err             string
}

type countReader struct {
	r io.Reader
	n *atomic.Int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

func Run(ctx context.Context, client *http.Client, req Request) Result {
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	started := time.Now()
	var firstByte time.Time
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			if firstByte.IsZero() {
				firstByte = time.Now()
			}
		},
	}
	httpReq, err := buildHTTPRequest(req)
	if err != nil {
		return Result{Err: err.Error()}
	}

	if req.Cancel == CancelBeforeHeaders {
		cancel()
	}

	httpReq = httpReq.WithContext(httptrace.WithClientTrace(runCtx, trace))
	httpReq.Header.Set("Accept-Encoding", "gzip")

	resp, err := client.Do(httpReq)
	if err != nil {
		out := Result{Err: err.Error(), Complete: time.Since(started)}
		if !firstByte.IsZero() {
			out.FirstByte = firstByte.Sub(started)
		}
		return out
	}
	defer resp.Body.Close()

	compressed := &atomic.Int64{}
	body := io.Reader(&countReader{r: resp.Body, n: compressed})
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(body)
		if err != nil {
			return Result{Err: err.Error(), CompressedBytes: compressed.Load()}
		}
		defer gz.Close()
		body = gz
	}

	decoded := &atomic.Int64{}
	body = &countReader{r: body, n: decoded}

	out := Result{}
	if !firstByte.IsZero() {
		out.FirstByte = firstByte.Sub(started)
	}

	if req.API == APISearch {
		parseSearch(body, &out, started, req.Cancel, cancel)
	} else {
		parseLabels(body, &out, started, req.Cancel, cancel)
	}
	out.CompressedBytes = compressed.Load()
	out.DecodedBytes = decoded.Load()
	if out.Complete == 0 {
		out.Complete = time.Since(started)
	}
	if resp.StatusCode >= 400 && out.Err == "" {
		out.Err = fmt.Sprintf("http %d", resp.StatusCode)
	}
	return out
}

func buildHTTPRequest(req Request) (*http.Request, error) {
	u, err := url.Parse(req.BaseURL)
	if err != nil {
		return nil, err
	}
	if req.API == APISearch {
		u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/search/" + string(req.Endpoint)
	} else {
		switch req.Endpoint {
		case EndpointMetricNames:
			u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/label/__name__/values"
		case EndpointLabelNames:
			u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/labels"
		case EndpointLabelValues:
			u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/label/" + url.PathEscape(req.Label) + "/values"
		}
	}
	q := u.Query()
	if !req.Start.IsZero() {
		q.Set("start", unix(req.Start))
	}
	if !req.End.IsZero() {
		q.Set("end", unix(req.End))
	}
	if req.Limit > 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}
	if req.API == APISearch {
		if req.BatchSize > 0 {
			q.Set("batch_size", strconv.Itoa(req.BatchSize))
		}
		term := strings.Join(strings.Fields(strings.TrimSpace(req.Term)), "")
		if term != "" {
			q.Set("search[]", term)
			q.Set("sort_by", "score")
			q.Set("fuzz_threshold", "80")
			q.Set("fuzz_alg", "jarowinkler")
			q.Set("case_sensitive", "false")
		}
		if req.IncludeMeta && req.Endpoint == EndpointMetricNames {
			q.Set("include_metadata", "true")
		}
		if req.Endpoint == EndpointLabelValues && req.Label != "" {
			q.Set("label", req.Label)
		}
	}
	for _, matcher := range req.Matchers {
		q.Add("match[]", matcher)
	}
	u.RawQuery = q.Encode()
	return http.NewRequest(http.MethodGet, u.String(), nil)
}

func unix(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', -1, 64)
}

func parseSearch(r io.Reader, out *Result, started time.Time, cancelMode CancelMode, cancel context.CancelFunc) {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			out.ChunkCount++
			out.ChunkBytes = append(out.ChunkBytes, len(line))
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 {
				if err := consumeSearchLine(trimmed, out, started); err != nil {
					out.Err = err.Error()
					out.Complete = time.Since(started)
					return
				}
				if cancelMode == CancelAfterFirstBatch && len(out.Names) > 0 && cancel != nil {
					abortAt := time.Now()
					cancel()
					drainAfterAbort(reader, out)
					out.CancelLatency = time.Since(abortAt)
					out.Complete = time.Since(started)
					return
				}
				if cancelMode == CancelHalfway && len(out.Names) > 0 && out.ChunkCount >= 2 && cancel != nil {
					abortAt := time.Now()
					cancel()
					drainAfterAbort(reader, out)
					out.CancelLatency = time.Since(abortAt)
					out.Complete = time.Since(started)
					return
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				out.Err = err.Error()
			}
			out.Complete = time.Since(started)
			return
		}
	}
}

func consumeSearchLine(line []byte, out *Result, started time.Time) error {
	var trailer struct {
		Status   string   `json:"status"`
		HasMore  bool     `json:"has_more"`
		Warnings []string `json:"warnings"`
		Error    string   `json:"error"`
	}
	if err := json.Unmarshal(line, &trailer); err != nil {
		return err
	}
	if trailer.Status == "error" {
		return fmt.Errorf("%s", trailer.Error)
	}
	if trailer.Status == "success" {
		out.HasMore = trailer.HasMore
		out.Warnings = append(out.Warnings, trailer.Warnings...)
		if out.FirstBatch == 0 {
			out.FirstBatch = time.Since(started)
		}
		return nil
	}
	var batch struct {
		Results  []map[string]any `json:"results"`
		Warnings []string         `json:"warnings"`
	}
	if err := json.Unmarshal(line, &batch); err != nil {
		return err
	}
	if out.FirstBatch == 0 && len(batch.Results) > 0 {
		out.FirstBatch = time.Since(started)
	}
	out.Warnings = append(out.Warnings, batch.Warnings...)
	for _, result := range batch.Results {
		name, _ := result["name"].(string)
		if name == "" {
			name, _ = result["value"].(string)
		}
		if name != "" {
			out.Names = append(out.Names, name)
		}
	}
	return nil
}

func parseLabels(r io.Reader, out *Result, started time.Time, cancelMode CancelMode, cancel context.CancelFunc) {
	body, err := io.ReadAll(r)
	if err != nil {
		out.Err = err.Error()
		out.Complete = time.Since(started)
		return
	}
	out.ChunkCount = 1
	out.ChunkBytes = []int{len(body)}
	var parsed struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
		Error  string   `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		out.Err = err.Error()
		out.Complete = time.Since(started)
		return
	}
	if parsed.Status != "success" {
		out.Err = parsed.Error
	}
	out.Names = parsed.Data
	out.FirstBatch = time.Since(started)
	out.Complete = time.Since(started)
	if cancelMode != CancelNone && cancel != nil {
		abortAt := time.Now()
		cancel()
		out.CancelLatency = time.Since(abortAt)
	}
}

func drainAfterAbort(reader *bufio.Reader, out *Result) {
	for {
		line, err := reader.ReadBytes('\n')
		out.BytesAfterAbort += int64(len(line))
		if err != nil {
			return
		}
	}
}
