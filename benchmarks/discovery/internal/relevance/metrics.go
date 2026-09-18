package relevance

import (
	"math"
	"strings"
)

type Scores struct {
	PrecisionAt5   float64 `json:"precision_at_5"`
	PrecisionAt20  float64 `json:"precision_at_20"`
	RecallAt10     float64 `json:"recall_at_10"`
	RecallAt100    float64 `json:"recall_at_100"`
	MRR            float64 `json:"mrr"`
	NDCGAt10       float64 `json:"ndcg_at_10"`
	ExpectedRank   int     `json:"expected_rank"`
	DuplicateCount int     `json:"duplicate_count"`
	SetEqual       bool    `json:"set_equal"`
}

func Evaluate(results, relevant []string, limit int, expectedSet []string) Scores {
	scores := Scores{
		DuplicateCount: duplicates(results),
		ExpectedRank:   firstRelevantRank(results, relevant),
	}
	scores.PrecisionAt5 = precisionAt(results, relevant, 5)
	scores.PrecisionAt20 = precisionAt(results, relevant, 20)
	scores.RecallAt10 = recallAt(results, relevant, 10)
	scores.RecallAt100 = recallAt(results, relevant, 100)
	scores.MRR = mrr(results, relevant)
	scores.NDCGAt10 = ndcgAt(results, relevant, 10)
	if expectedSet != nil && len(expectedSet) <= limit {
		scores.SetEqual = setEqual(results, expectedSet)
	}
	return scores
}

func precisionAt(results, relevant []string, k int) float64 {
	if k <= 0 {
		return 0
	}
	n := 0
	limit := min(k, len(results))
	rel := asSet(relevant)
	for i := 0; i < limit; i++ {
		if rel[results[i]] {
			n++
		}
	}
	return float64(n) / float64(k)
}

func recallAt(results, relevant []string, k int) float64 {
	if len(relevant) == 0 {
		if len(results) == 0 {
			return 1
		}
		return 0
	}
	rel := asSet(relevant)
	found := 0
	limit := min(k, len(results))
	for i := 0; i < limit; i++ {
		if rel[results[i]] {
			found++
		}
	}
	return float64(found) / float64(len(relevant))
}

func mrr(results, relevant []string) float64 {
	rank := firstRelevantRank(results, relevant)
	if rank <= 0 {
		if len(relevant) == 0 {
			return 1
		}
		return 0
	}
	return 1 / float64(rank)
}

func ndcgAt(results, relevant []string, k int) float64 {
	if len(relevant) == 0 {
		if len(results) == 0 {
			return 1
		}
		return 0
	}
	rel := asSet(relevant)
	var dcg float64
	limit := min(k, len(results))
	for i := 0; i < limit; i++ {
		if rel[results[i]] {
			dcg += 1 / math.Log2(float64(i+2))
		}
	}
	idealN := min(k, len(relevant))
	var idcg float64
	for i := 0; i < idealN; i++ {
		idcg += 1 / math.Log2(float64(i+2))
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

func firstRelevantRank(results, relevant []string) int {
	if len(relevant) == 0 {
		return 0
	}
	rel := asSet(relevant)
	for i, name := range results {
		if rel[name] {
			return i + 1
		}
	}
	return 0
}

func duplicates(results []string) int {
	seen := map[string]int{}
	dupes := 0
	for _, name := range results {
		seen[name]++
		if seen[name] == 2 {
			dupes++
		}
	}
	return dupes
}

func setEqual(results, expected []string) bool {
	if len(results) != len(expected) {
		return false
	}
	a := asSet(results)
	for _, name := range expected {
		if !a[name] {
			return false
		}
	}
	return true
}

func asSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}

func ContainsFold(haystack []string, needle string) bool {
	for _, v := range haystack {
		if strings.EqualFold(v, needle) {
			return true
		}
	}
	return false
}
