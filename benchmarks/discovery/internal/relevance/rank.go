package relevance

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

const DefaultJaroWinklerThreshold = 0.80

func RankLabels(names []string, term string) []string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(term)), "")
	if normalized == "" {
		out := append([]string(nil), names...)
		return out
	}
	needle := strings.ToLower(normalized)
	needle = strings.ReplaceAll(needle, "_", "")
	type scored struct {
		name  string
		score float64
	}
	var hits []scored
	for _, name := range names {
		lower := strings.ToLower(name)
		compact := strings.ReplaceAll(strings.Join(strings.Fields(lower), ""), "_", "")
		score := JaroWinkler(compact, needle)
		if strings.Contains(compact, needle) || strings.Contains(needle, compact) {
			if score < 0.95 {
				score = 0.95
			}
		}
		if score >= DefaultJaroWinklerThreshold || strings.Contains(compact, needle) {
			hits = append(hits, scored{name: name, score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score == hits[j].score {
			return hits[i].name < hits[j].name
		}
		return hits[i].score > hits[j].score
	})
	out := make([]string, len(hits))
	for i, hit := range hits {
		out[i] = hit.name
	}
	return out
}

func JaroWinkler(s1, s2 string) float64 {
	j := jaro(s1, s2)
	if j == 0 {
		return 0
	}
	prefix := 0
	r1 := []rune(s1)
	r2 := []rune(s2)
	limit := min(4, min(len(r1), len(r2)))
	for i := 0; i < limit; i++ {
		if r1[i] != r2[i] {
			break
		}
		prefix++
	}
	return j + 0.1*float64(prefix)*(1-j)
}

func jaro(s1, s2 string) float64 {
	if s1 == s2 {
		return 1
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	if len(r1) == 0 || len(r2) == 0 {
		return 0
	}
	matchDist := int(math.Max(float64(max(len(r1), len(r2))/2-1), 0))
	matched1 := make([]bool, len(r1))
	matched2 := make([]bool, len(r2))
	matches := 0
	for i := 0; i < len(r1); i++ {
		start := max(0, i-matchDist)
		end := min(i+matchDist+1, len(r2))
		for j := start; j < end; j++ {
			if matched2[j] || r1[i] != r2[j] {
				continue
			}
			matched1[i] = true
			matched2[j] = true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0
	}
	transpositions := 0
	k := 0
	for i := 0; i < len(r1); i++ {
		if !matched1[i] {
			continue
		}
		for !matched2[k] {
			k++
		}
		if r1[i] != r2[k] {
			transpositions++
		}
		k++
	}
	m := float64(matches)
	return (m/float64(len(r1)) + m/float64(len(r2)) + (m-float64(transpositions)/2)/m) / 3
}

func RuneLen(s string) int { return utf8.RuneCountInString(s) }
