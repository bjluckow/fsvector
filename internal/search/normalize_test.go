package search_test

import (
	"testing"

	"github.com/bjluckow/fsvector/internal/search"
)

func TestNormalize_SingleModality(t *testing.T) {
	results := []search.SearchResult{
		{Modality: "text", Score: 0.9},
		{Modality: "text", Score: 0.6},
		{Modality: "text", Score: 0.3},
	}

	got := search.Normalize(results)

	if got[0].NormScore != 1.0 {
		t.Errorf("expected 1.0, got %f", got[0].NormScore)
	}
	if got[2].NormScore != 0.0 {
		t.Errorf("expected 0.0, got %f", got[2].NormScore)
	}
	// middle value should be 0.5
	if got[1].NormScore != 0.5 {
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

	// text: max=0.9 min=0.5
	if got[0].NormScore != 1.0 {
		t.Errorf("text max: expected 1.0, got %f", got[0].NormScore)
	}
	if got[1].NormScore != 0.0 {
		t.Errorf("text min: expected 0.0, got %f", got[1].NormScore)
	}

	// image: max=0.3 min=0.1
	if got[2].NormScore != 1.0 {
		t.Errorf("image max: expected 1.0, got %f", got[2].NormScore)
	}
	if got[3].NormScore != 0.0 {
		t.Errorf("image min: expected 0.0, got %f", got[3].NormScore)
	}
}

func TestNormalize_SingleResult(t *testing.T) {
	results := []search.SearchResult{
		{Modality: "image", Score: 0.25},
	}
	got := search.Normalize(results)
	if got[0].NormScore != 1.0 {
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