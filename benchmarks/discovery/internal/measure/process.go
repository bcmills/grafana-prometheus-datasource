package measure

import (
	"bufio"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type ProcessSample struct {
	CPUSeconds float64
	RSSBytes   int64
}

func SampleProcess(metricsURL string) (ProcessSample, error) {
	resp, err := http.Get(metricsURL)
	if err != nil {
		return ProcessSample{}, err
	}
	defer resp.Body.Close()
	return parseProcessMetrics(resp.Body)
}

func parseProcessMetrics(r io.Reader) (ProcessSample, error) {
	var sample ProcessSample
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "process_cpu_seconds_total "):
			v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, "process_cpu_seconds_total ")), 64)
			if err != nil {
				return sample, err
			}
			sample.CPUSeconds = v
		case strings.HasPrefix(line, "process_resident_memory_bytes "):
			v, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "process_resident_memory_bytes ")), 10, 64)
			if err != nil {
				return sample, err
			}
			sample.RSSBytes = v
		}
	}
	return sample, scanner.Err()
}
