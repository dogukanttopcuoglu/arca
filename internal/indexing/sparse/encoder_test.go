package sparse

import (
	"context"
	"math"
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	t.Run("lowercases and keeps alphanumeric tokens only", func(t *testing.T) {
		got := tokenize("Rick Rubin's Def Jam! Founded in NEW YORK, 1984.")
		want := []string{"rick", "rubin", "s", "def", "jam", "founded", "in", "new", "york", "1984"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("returns empty slice for empty or non-alphanumeric text", func(t *testing.T) {
		if got := tokenize(""); len(got) != 0 {
			t.Errorf("expected no tokens, got %v", got)
		}
		if got := tokenize("!!! ... ???"); len(got) != 0 {
			t.Errorf("expected no tokens, got %v", got)
		}
	})
}

func TestBuildCorpusStats(t *testing.T) {
	t.Run("computes document frequency, document count, and average length", func(t *testing.T) {
		stats, err := BuildCorpusStats([]string{
			"the cat sat",
			"the dog ran",
			"cat cat dog",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if stats.TotalDocs != 3 {
			t.Errorf("expected 3 documents, got %d", stats.TotalDocs)
		}
		if stats.AvgDocLen != 3.0 {
			t.Errorf("expected average doc length 3.0, got %v", stats.AvgDocLen)
		}
		// Document frequencies: the=2, cat=2, dog=2, sat=1, ran=1
		if stats.DocFreq("the") != 2 || stats.DocFreq("cat") != 2 || stats.DocFreq("sat") != 1 {
			t.Errorf("unexpected document frequencies: %+v", stats)
		}
	})

	t.Run("rejects an empty corpus", func(t *testing.T) {
		if _, err := BuildCorpusStats(nil); err == nil {
			t.Error("expected error for empty corpus")
		}
		if _, err := BuildCorpusStats([]string{"", "   "}); err == nil {
			t.Error("expected error for corpus without tokens")
		}
	})

	t.Run("assigns deterministic term ids over sorted vocabulary", func(t *testing.T) {
		stats, err := BuildCorpusStats([]string{"zebra alpha", "mango zebra"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Sorted vocabulary: alpha, mango, zebra
		if stats.TermID("alpha") != 0 || stats.TermID("mango") != 1 || stats.TermID("zebra") != 2 {
			t.Errorf("expected sorted term ids, got alpha=%d mango=%d zebra=%d",
				stats.TermID("alpha"), stats.TermID("mango"), stats.TermID("zebra"))
		}
	})
}

func TestBM25EncoderEncode(t *testing.T) {
	// Hand-computed over corpus: ["the cat sat", "the dog ran", "cat cat dog"]
	// N=3, avgdl=3. IDF(t) = ln(1 + (N - df + 0.5)/(df + 0.5)):
	//   cat df=2 -> ln(1 + 1.5/2.5) = 0.47000362924573563
	//   dog df=2 -> 0.47000362924573563
	// With k1=1.5, b=0.75, dl=3, avgdl=3: factor (1 - b + b*dl/avgdl) = 1.0
	// weight(t) = idf * tf*(k1+1) / (tf + k1)
	//   "cat cat dog": cat tf=2 -> 0.47000362924573563 * 5/3.5 = 0.6714337560653366
	//                  dog tf=1 -> 0.47000362924573563 * 2.5/2.5 = 0.47000362924573563
	ctx := context.Background()

	stats, err := BuildCorpusStats([]string{"the cat sat", "the dog ran", "cat cat dog"})
	if err != nil {
		t.Fatalf("failed to build corpus stats: %v", err)
	}
	enc := NewBM25Encoder(stats)

	vec, err := enc.Encode(ctx, "cat cat dog")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Sorted vocabulary: cat(0), dog(1), ran(2), sat(3), the(4)
	want := SparseVector{
		Indices: []uint32{0, 1},
		Values:  []float32{0.6714337560653366, 0.47000362924573563},
	}
	if !reflect.DeepEqual(vec.Indices, want.Indices) {
		t.Errorf("expected indices %v, got %v", want.Indices, vec.Indices)
	}
	if len(vec.Values) != len(want.Values) {
		t.Fatalf("expected %d values, got %d", len(want.Values), len(vec.Values))
	}
	for i := range want.Values {
		if math.Abs(float64(vec.Values[i]-want.Values[i])) > 1e-6 {
			t.Errorf("value %d: expected %v, got %v", i, want.Values[i], vec.Values[i])
		}
	}

	t.Run("encode is deterministic for the same corpus", func(t *testing.T) {
		v1, err := enc.Encode(ctx, "cat cat dog")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		v2, err := enc.Encode(ctx, "cat cat dog")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(v1, v2) {
			t.Error("expected identical sparse vectors for identical input")
		}
	})

	t.Run("corpus change changes weights", func(t *testing.T) {
		// Same term "cat": idf differs by corpus. Corpus A: N=3, df=2 ->
		// ln(1 + 1.5/2.5) = 0.4700036. Corpus B (single doc "cat cat cat"):
		// N=1, df=1 -> idf = ln(1 + 0.5/1.5) = 0.28768207.
		// Encode("cat") has docLen=1, avgdl=3 -> denomFactor = 0.25+0.75/3 = 0.5
		// -> weight = idf * 2.5/(1 + 1.5*0.5) = 0.28768207 * 1.42857143 = 0.41097439
		otherStats, err := BuildCorpusStats([]string{"cat cat cat"})
		if err != nil {
			t.Fatalf("failed to build corpus stats: %v", err)
		}
		other := NewBM25Encoder(otherStats)
		v, err := other.Encode(ctx, "cat")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := math.Log(1+0.5/1.5) * 2.5 / (1 + 1.5*0.5)
		if len(v.Values) != 1 || math.Abs(float64(v.Values[0])-want) > 1e-6 {
			t.Errorf("expected single weight %v for cat in single-doc corpus, got %+v", want, v)
		}
	})

	t.Run("excludes terms absent from the corpus vocabulary", func(t *testing.T) {
		v, err := enc.Encode(ctx, "aquaman")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v.Indices) != 0 || len(v.Values) != 0 {
			t.Errorf("expected empty vector for out-of-vocabulary term, got %+v", v)
		}
	})

	t.Run("encodes empty text to an empty vector", func(t *testing.T) {
		v, err := enc.Encode(ctx, "   ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(v.Indices) != 0 || len(v.Values) != 0 {
			t.Errorf("expected empty vector for empty text, got %+v", v)
		}
	})
}
