package sparse

import (
	"context"
	"math"
	"reflect"
	"testing"
)

type fakeCorpusSource struct {
	texts []string
}

func (f fakeCorpusSource) CorpusTexts(ctx context.Context) ([]string, error) {
	return f.texts, nil
}

func TestBM25EncoderProvider(t *testing.T) {
	t.Run("builds statistics over persisted corpus plus incoming chunks", func(t *testing.T) {
		// Persisted corpus: ["the cat sat", "the dog ran"].
		// Incoming chunk: "cat cat dog" -> corpus becomes the 3-doc corpus
		// from the encoder tests: N=3, avgdl=3, cat df=2.
		provider := NewBM25EncoderProvider(fakeCorpusSource{texts: []string{
			"the cat sat",
			"the dog ran",
		}})

		enc, err := provider.Encoder(context.Background(), []DocumentChunk{
			{ID: "chk-1", Content: "cat cat dog"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		vec, err := enc.Encode(context.Background(), "cat cat dog")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Same hand-computed expectation as the encoder test: idf 0.4700036,
		// tf=2 -> 0.47000362924573563 * 5/3.5 = 0.6714337560653366
		want := SparseVector{
			Indices: []uint32{0, 1},
			Values:  []float32{0.6714337560653366, 0.47000362924573563},
		}
		if !reflect.DeepEqual(vec.Indices, want.Indices) {
			t.Errorf("expected indices %v, got %v", want.Indices, vec.Indices)
		}
		for i := range want.Values {
			if math.Abs(float64(vec.Values[i]-want.Values[i])) > 1e-6 {
				t.Errorf("value %d: expected %v, got %v", i, want.Values[i], vec.Values[i])
			}
		}
	})

	t.Run("rejects a source failure", func(t *testing.T) {
		provider := NewBM25EncoderProvider(fakeCorpusSource{})
		_, err := provider.Encoder(context.Background(), nil)
		if err == nil {
			t.Error("expected error for empty corpus")
		}
	})

	t.Run("encoder for corpus uses the persisted corpus alone", func(t *testing.T) {
		// Persisted: ["the cat sat", "the dog ran"] -> N=2, cat df=1, dog df=1.
		// Encode("cat cat dog"): idf(cat) = idf(dog) = ln(1 + 1.5/1.5) = ln(2).
		// cat tf=2, docLen=3, avgdl=3 -> denomFactor 1.0 -> ln(2)*5/3.5.
		// dog tf=1 -> ln(2)*2.5/2.5 = ln(2).
		provider := NewBM25EncoderProvider(fakeCorpusSource{texts: []string{
			"the cat sat",
			"the dog ran",
		}})

		enc, err := provider.EncoderForCorpus(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		vec, err := enc.Encode(context.Background(), "cat cat dog")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantCat := math.Log(2) * 5 / 3.5
		wantDog := math.Log(2)
		if len(vec.Values) != 2 || math.Abs(float64(vec.Values[0])-wantCat) > 1e-6 || math.Abs(float64(vec.Values[1])-wantDog) > 1e-6 {
			t.Errorf("expected weights [%v %v], got %+v", wantCat, wantDog, vec)
		}
	})
}
