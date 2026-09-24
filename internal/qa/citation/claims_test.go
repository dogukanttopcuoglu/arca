package citation_test

import (
	"reflect"
	"strings"
	"testing"

	qacitation "arca/internal/qa/citation"
)

func TestExtractClaims(t *testing.T) {
	t.Run("keeps one claim per sentence carrying a single ref", func(t *testing.T) {
		claims := qacitation.ExtractClaims("Creativity is a discipline [Ref 1].")
		if len(claims) != 1 {
			t.Fatalf("expected 1 claim, got %d", len(claims))
		}
		if claims[0].Sentence != "Creativity is a discipline [Ref 1]." {
			t.Errorf("sentence = %q", claims[0].Sentence)
		}
		if !reflect.DeepEqual(claims[0].Refs, []int{1}) {
			t.Errorf("refs = %v, want [1]", claims[0].Refs)
		}
	})

	t.Run("expands combined refs in a single bracket", func(t *testing.T) {
		claims := qacitation.ExtractClaims("Both are supported [Ref 1, 2].")
		if len(claims) != 1 {
			t.Fatalf("expected 1 claim, got %d", len(claims))
		}
		if !reflect.DeepEqual(claims[0].Refs, []int{1, 2}) {
			t.Errorf("refs = %v, want [1 2]", claims[0].Refs)
		}
	})

	t.Run("splits multiple sentences on all terminators", func(t *testing.T) {
		claims := qacitation.ExtractClaims("First [Ref 1]. Second [Ref 2]! Third [Ref 3]?")
		if len(claims) != 3 {
			t.Fatalf("expected 3 claims, got %d", len(claims))
		}
		for i, want := range [][]int{{1}, {2}, {3}} {
			if !reflect.DeepEqual(claims[i].Refs, want) {
				t.Errorf("claim %d refs = %v, want %v", i, claims[i].Refs, want)
			}
		}
	})

	t.Run("drops sentences without markers", func(t *testing.T) {
		claims := qacitation.ExtractClaims("Uncited preamble. Cited claim [Ref 1].")
		if len(claims) != 1 {
			t.Fatalf("expected 1 claim, got %d", len(claims))
		}
		if claims[0].Sentence != "Cited claim [Ref 1]." {
			t.Errorf("sentence = %q", claims[0].Sentence)
		}
	})

	t.Run("no markers anywhere returns nil", func(t *testing.T) {
		if got := qacitation.ExtractClaims("A claim with no citation markers at all."); got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})

	t.Run("empty and blank text return nil", func(t *testing.T) {
		if got := qacitation.ExtractClaims(""); got != nil {
			t.Fatalf("expected nil for empty text, got %+v", got)
		}
		if got := qacitation.ExtractClaims("   \n  "); got != nil {
			t.Fatalf("expected nil for blank text, got %+v", got)
		}
	})

	t.Run("bare number lists in prose are not refs", func(t *testing.T) {
		if got := qacitation.ExtractClaims("The inputs were [1, 2] as shown."); got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})

	t.Run("period not followed by space does not split", func(t *testing.T) {
		claims := qacitation.ExtractClaims("v1.2 discipline matters [Ref 1].")
		if len(claims) != 1 {
			t.Fatalf("expected 1 claim, got %d", len(claims))
		}
		if claims[0].Sentence != "v1.2 discipline matters [Ref 1]." {
			t.Errorf("sentence = %q", claims[0].Sentence)
		}
	})

	t.Run("e.g. fragment is dropped for carrying no ref", func(t *testing.T) {
		claims := qacitation.ExtractClaims("e.g. discipline matters [Ref 1].")
		if len(claims) != 1 {
			t.Fatalf("expected 1 claim, got %d", len(claims))
		}
		if claims[0].Sentence != "discipline matters [Ref 1]." {
			t.Errorf("sentence = %q", claims[0].Sentence)
		}
	})
}

func TestExtractClaims_QuoteBoundaries(t *testing.T) {
	got := qacitation.ExtractClaims(`The book calls it a misconception to say we listen with the ears or the mind: "We listen with the whole body, our whole self." Certain bass sounds can only be felt in the body [Ref 1]. "When listening, there is only now" [Ref 2].`)
	if len(got) != 2 {
		t.Fatalf("claims = %d, want 2 (ref-less sentence dropped): %+v", len(got), got)
	}
	if !strings.Contains(got[0].Sentence, "Certain bass sounds") || got[0].Refs[0] != 1 {
		t.Errorf("first claim = %+v", got[0])
	}
	if !strings.Contains(got[1].Sentence, "When listening") || got[1].Refs[0] != 2 {
		t.Errorf("second claim = %+v", got[1])
	}
}
