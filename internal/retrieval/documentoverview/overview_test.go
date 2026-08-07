package documentoverview_test

import (
	"context"
	"testing"

	"arca/internal/indexing/store"
	indexingmodel "arca/internal/indexing/model"
	documentoverview "arca/internal/retrieval/documentoverview"
	"arca/internal/retrieval/seam"
	pdfmodel "arca/internal/pdfinspector/model"
)

// point builds one store point; contentType "" means a regular chunk.
func point(docID, chunkID, section string, order int, contentType string) store.VectorPoint {
	ct := pdfmodel.ContentTypeParagraph
	if contentType != "" {
		ct = contentType
	}
	return store.VectorPoint{
		ID:              docID + "/" + chunkID,
		Vector:          []float32{float32(order)},
		ContentMarkdown: "content " + chunkID,
		Metadata: indexingmodel.VectorMetadata{
			DocumentID:  docID,
			ChunkID:     docID + "/" + chunkID,
			ChunkOrder:  order,
			SectionPath: section,
			ContentType: ct,
			ContentHash: chunkID + "-hash",
		},
	}
}

func seededStore(t *testing.T) *store.InMemoryVectorStore {
	t.Helper()
	vs := store.NewInMemoryVectorStore()
	points := []store.VectorPoint{
		point("doc-a", "document-profile/001", "Document Profile", 0, pdfmodel.ContentTypeDocumentProfile),
		point("doc-a", "frontmatter/001", "Copyright", 1, ""),
		point("doc-a", "intro/001", "Intro", 2, ""),
		point("doc-a", "intro/002", "Intro", 3, ""),
		point("doc-a", "body/001", "Body", 4, ""),
		point("doc-a", "body/002", "Body", 5, ""),
		point("doc-a", "body/003", "Body", 6, ""),
		point("doc-b", "document-profile/001", "Document Profile", 0, pdfmodel.ContentTypeDocumentProfile),
		point("doc-b", "intro/001", "Intro", 2, ""),
	}
	if err := vs.UpsertPoints(context.Background(), points); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return vs
}

func retrieve(t *testing.T, r seam.Retriever, query seam.RetrievalQuery) []seam.SearchResult {
	t.Helper()
	results, err := r.Retrieve(context.Background(), query)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	return results
}

func chunkIDs(results []seam.SearchResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.ChunkID
	}
	return out
}

func TestOverviewRetriever_FilteredSelection(t *testing.T) {
	vs := seededStore(t)
	r := documentoverview.NewRetriever(vs)

	t.Run("profile first, then first real content chunks, front matter excluded", func(t *testing.T) {
		results := retrieve(t, r, seam.RetrievalQuery{
			QueryText: "Bu kitap ne anlatıyor?",
			TopK:      5,
			Filter:    indexingmodel.MetadataFilter{DocumentIDs: []string{"doc-a"}},
		})
		want := []string{
			"doc-a/document-profile/001",
			"doc-a/intro/001",
			"doc-a/intro/002",
			"doc-a/body/001",
			"doc-a/body/002",
		}
		got := chunkIDs(results)
		if len(got) != len(want) {
			t.Fatalf("results = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("results = %v, want %v (position %d)", got, want, i)
			}
		}
		// The copyright page (order 1) must never appear.
		for _, id := range got {
			if id == "doc-a/frontmatter/001" {
				t.Fatalf("front matter leaked into the overview: %v", got)
			}
		}
	})

	t.Run("results carry full payload metadata for citation rendering", func(t *testing.T) {
		results := retrieve(t, r, seam.RetrievalQuery{
			QueryText: "yazar kim?",
			TopK:      2,
			Filter:    indexingmodel.MetadataFilter{DocumentIDs: []string{"doc-a"}},
		})
		p := results[0]
		if p.Metadata.DocumentID != "doc-a" || p.Metadata.SectionPath != "Document Profile" {
			t.Fatalf("profile metadata = %+v", p.Metadata)
		}
		if p.ContentMarkdown == "" {
			t.Fatal("profile content must be carried for the gate/LLM")
		}
	})

	t.Run("TopK truncates deterministically", func(t *testing.T) {
		results := retrieve(t, r, seam.RetrievalQuery{
			QueryText: "özetle",
			TopK:      3,
			Filter:    indexingmodel.MetadataFilter{DocumentIDs: []string{"doc-a"}},
		})
		if len(results) != 3 {
			t.Fatalf("expected 3 results at TopK 3, got %v", chunkIDs(results))
		}
		if results[0].ChunkID != "doc-a/document-profile/001" {
			t.Fatalf("profile must always rank first, got %v", chunkIDs(results))
		}
	})
}

func TestOverviewRetriever_UnfilteredSelection(t *testing.T) {
	vs := seededStore(t)
	r := documentoverview.NewRetriever(vs)

	t.Run("no filter returns all document profile points only", func(t *testing.T) {
		results := retrieve(t, r, seam.RetrievalQuery{QueryText: "kütüphanede hangi kitaplar var?", TopK: 5})
		want := []string{"doc-a/document-profile/001", "doc-b/document-profile/001"}
		got := chunkIDs(results)
		if len(got) != len(want) {
			t.Fatalf("results = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("results = %v, want %v", got, want)
			}
		}
	})

	t.Run("empty store returns empty, not an error", func(t *testing.T) {
		empty := store.NewInMemoryVectorStore()
		r := documentoverview.NewRetriever(empty)
		results := retrieve(t, r, seam.RetrievalQuery{QueryText: "bu kitap ne anlatıyor?", TopK: 5})
		if len(results) != 0 {
			t.Fatalf("expected empty result, got %v", chunkIDs(results))
		}
	})
}
