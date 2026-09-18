package relevance

import (
	"math"
	"testing"
)

func TestPrecisionRecallMRR(t *testing.T) {
	results := []string{"a", "x", "b", "y", "c"}
	relevant := []string{"a", "b", "c"}
	scores := Evaluate(results, relevant, 100, nil)
	if scores.PrecisionAt5 != 0.6 {
		t.Fatalf("p@5 = %v", scores.PrecisionAt5)
	}
	if scores.RecallAt10 != 1 {
		t.Fatalf("r@10 = %v", scores.RecallAt10)
	}
	if scores.MRR != 1 {
		t.Fatalf("mrr = %v", scores.MRR)
	}
	if scores.ExpectedRank != 1 {
		t.Fatalf("rank = %d", scores.ExpectedRank)
	}
	if scores.DuplicateCount != 0 {
		t.Fatalf("dupes = %d", scores.DuplicateCount)
	}
}

func TestNDCGPerfect(t *testing.T) {
	relevant := []string{"a", "b"}
	got := ndcgAt([]string{"a", "b", "c"}, relevant, 10)
	if math.Abs(got-1) > 1e-9 {
		t.Fatalf("ndcg = %v", got)
	}
}

func TestNoMatch(t *testing.T) {
	scores := Evaluate([]string{}, []string{}, 100, []string{})
	if scores.NDCGAt10 != 1 || scores.MRR != 1 || scores.RecallAt10 != 1 {
		t.Fatalf("empty vs empty should be perfect: %+v", scores)
	}
	scores = Evaluate([]string{"nope"}, []string{}, 100, []string{})
	if scores.NDCGAt10 != 0 {
		t.Fatalf("unexpected results for no-match query: %+v", scores)
	}
}

func TestDuplicatesAndSetEqual(t *testing.T) {
	scores := Evaluate([]string{"a", "a", "b"}, []string{"a", "b"}, 10, []string{"a", "b"})
	if scores.DuplicateCount != 1 {
		t.Fatalf("dupes = %d", scores.DuplicateCount)
	}
	if scores.SetEqual {
		t.Fatal("duplicate list should not be set-equal")
	}
	scores = Evaluate([]string{"b", "a"}, []string{"a", "b"}, 10, []string{"a", "b"})
	if !scores.SetEqual {
		t.Fatal("same members should be set-equal")
	}
}
