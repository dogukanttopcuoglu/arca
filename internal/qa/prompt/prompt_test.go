package prompt_test

import (
	"context"
	"testing"

	qacontext "arca/internal/qa/context"
	qaprompt "arca/internal/qa/prompt"
)

func TestRAGPromptBuilder(t *testing.T) {
	ctx := context.Background()
	builder := qaprompt.NewRAGPromptBuilder()

	win := &qacontext.ContextWindow{
		Content: "[Ref 1] Source: doc-1\nCreativity is a discipline.\n\n",
		Sources: []qacontext.SourceReference{
			{CitationKey: "[Ref 1]", DocumentID: "doc-1", ChunkID: "chk-1"},
		},
		TokenCount: 15,
	}

	t.Run("builds prompt message with system instructions and user context", func(t *testing.T) {
		prompt, err := builder.Build(ctx, "What is creativity?", win)
		if err != nil {
			t.Fatalf("unexpected error building prompt: %v", err)
		}

		if prompt.System == "" {
			t.Error("expected non-empty System prompt instructions")
		}
		if len(prompt.Messages) == 0 {
			t.Fatal("expected non-empty Messages slice")
		}
		if prompt.Messages[0].Role != "user" {
			t.Errorf("expected user role, got %q", prompt.Messages[0].Role)
		}
	})
}
