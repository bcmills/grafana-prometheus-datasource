package dataset

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

func WritePrometheusText(w io.Writer, samples []Sample, storage Storage) error {
	grouped := groupByName(filterStorage(samples, storage, true))
	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, err := fmt.Fprintf(w, "# HELP %s discovery benchmark fixture\n", escapeNameForHelp(name)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "# TYPE %s gauge\n", escapeNameForHelp(name)); err != nil {
			return err
		}
		for _, sample := range grouped[name] {
			if _, err := fmt.Fprintf(w, "%s 1\n", formatSample(sample, false, time.Time{})); err != nil {
				return err
			}
		}
	}
	return nil
}

func WriteOpenMetrics(w io.Writer, samples []Sample, timestamp time.Time) error {
	grouped := groupByName(filterStorage(samples, StorageHistorical, false))
	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.ContainsFunc(name, func(r rune) bool { return r > 127 || r == '.' || r == ' ' }) {
			// OpenMetrics 1.0 rejects UTF-8 metric names. Those series stay on the active scrape path.
			continue
		}
		if _, err := fmt.Fprintf(w, "# HELP %s discovery benchmark fixture\n", name); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "# TYPE %s gauge\n", name); err != nil {
			return err
		}
		for _, sample := range grouped[name] {
			if sample.UTF8Name {
				continue
			}
			if _, err := fmt.Fprintf(w, "%s 1 %d\n", formatSample(sample, false, time.Time{}), timestamp.Unix()); err != nil {
				return err
			}
		}
	}
	_, err := io.WriteString(w, "# EOF\n")
	return err
}

func filterStorage(samples []Sample, storage Storage, includeActiveUTF8 bool) []Sample {
	out := make([]Sample, 0, len(samples))
	for _, sample := range samples {
		switch storage {
		case StorageActive:
			if sample.Storage != StorageActive {
				continue
			}
		case StorageHistorical:
			if sample.Storage != StorageHistorical {
				continue
			}
			if sample.UTF8Name {
				continue
			}
		}
		if sample.UTF8Name && !includeActiveUTF8 {
			continue
		}
		out = append(out, sample)
	}
	return out
}

func groupByName(samples []Sample) map[string][]Sample {
	out := make(map[string][]Sample)
	for _, sample := range samples {
		out[sample.Name] = append(out[sample.Name], sample)
	}
	return out
}

func formatSample(sample Sample, _ bool, _ time.Time) string {
	keys := make([]string, 0, len(sample.Labels))
	for k := range sample.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf(`%s=%q`, k, sample.Labels[k]))
	}
	body := strings.Join(parts, ",")
	if needsQuotedName(sample.Name) {
		return fmt.Sprintf("{%q,%s}", sample.Name, body)
	}
	if body == "" {
		return sample.Name
	}
	return fmt.Sprintf("%s{%s}", sample.Name, body)
}

func needsQuotedName(name string) bool {
	if name == "" {
		return true
	}
	for i, r := range name {
		ok := r == '_' || r == ':' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9'
		if !ok {
			return true
		}
	}
	return false
}

func escapeNameForHelp(name string) string {
	if needsQuotedName(name) {
		return fmt.Sprintf("%q", name)
	}
	return name
}
