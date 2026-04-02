package search_test

import (
	"testing"

	"github.com/bjluckow/fsvector/internal/search"
)

func approxEqual(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

func TestNormalize_SingleModality(t *testing.T) {
	results := []search.SearchResult{
		{Modality: "text", Score: 0.9},
		{Modality: "text", Score: 0.6},
		{Modality: "text", Score: 0.3},
	}

	got := search.Normalize(results)

	if !approxEqual(got[0].NormScore, 1.0, 1e-9) {
		t.Errorf("expected 1.0, got %f", got[0].NormScore)
	}
	if !approxEqual(got[2].NormScore, 0.0, 1e-9) {
		t.Errorf("expected 0.0, got %f", got[2].NormScore)
	}
	// middle value should be 0.5
	if !approxEqual(got[1].NormScore, 0.5, 1e-9) {
		t.Errorf("expected 0.5, got %f", got[1].NormScore)
	}
}

func TestNormalize_MultipleModalities(t *testing.T) {
	results := []search.SearchResult{
		{Modality: "text", Score: 0.9},
		{Modality: "text", Score: 0.5},
		{Modality: "image", Score: 0.3},
		{Modality: "image", Score: 0.1},
	}

	got := search.Normalize(results)

	// global: max=0.9 min=0.1, range=0.8
	// 0.9 → 1.0
	if !approxEqual(got[0].NormScore, 1.0, 1e-9) {
		t.Errorf("expected 1.0, got %f", got[0].NormScore)
	}
	// 0.5 → (0.5-0.1)/0.8 = 0.5
	if !approxEqual(got[1].NormScore, 0.5, 1e-9) {
		t.Errorf("expected 0.5, got %f", got[1].NormScore)
	}
	// 0.3 → (0.3-0.1)/0.8 = 0.25
	if !approxEqual(got[2].NormScore, 0.25, 1e-9) {
		t.Errorf("expected 0.25, got %f", got[2].NormScore)
	}
	// 0.1 → 0.0
	if !approxEqual(got[3].NormScore, 0.0, 1e-9) {
		t.Errorf("expected 0.0, got %f", got[3].NormScore)
	}
}

func TestNormalize_SingleResult(t *testing.T) {
	results := []search.SearchResult{
		{Modality: "image", Score: 0.25},
	}
	got := search.Normalize(results)
	if !approxEqual(got[0].NormScore, 1.0, 1e-9) {
		t.Errorf("expected 1.0 for single result, got %f", got[0].NormScore)
	}
}

func TestNormalize_Empty(t *testing.T) {
	got := search.Normalize([]search.SearchResult{})
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d results", len(got))
	}
}

func TestNormalize_EqualScores(t *testing.T) {
	results := []search.SearchResult{
		{Modality: "text", Score: 0.7},
		{Modality: "text", Score: 0.7},
	}
	got := search.Normalize(results)
	for i, r := range got {
		if r.NormScore != 1.0 {
			t.Errorf("result %d: expected 1.0 for equal scores, got %f", i, r.NormScore)
		}
	}
}
