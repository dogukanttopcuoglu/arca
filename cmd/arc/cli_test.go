package main_test

import (
	"context"
	"testing"

	arccli "arca/cmd/arc/cli"
)

func TestCLIToolCommands(t *testing.T) {
	ctx := context.Background()
	app := arccli.NewApp()

	t.Run("executes 'inspect' CLI command", func(t *testing.T) {
		out, err := app.RunInspect(ctx, "sample.pdf")
		if err != nil {
			t.Fatalf("unexpected error running CLI inspect: %v", err)
		}
		if out == "" {
			t.Error("expected non-empty output from CLI inspect")
		}
	})

	t.Run("executes 'ask' CLI command", func(t *testing.T) {
		out, err := app.RunAsk(ctx, "What is creativity?")
		if err != nil {
			t.Fatalf("unexpected error running CLI ask: %v", err)
		}
		if out == "" {
			t.Error("expected non-empty output from CLI ask")
		}
	})

	t.Run("executes 'research' CLI command", func(t *testing.T) {
		out, err := app.RunResearch(ctx, "Synthesize creative principles")
		if err != nil {
			t.Fatalf("unexpected error running CLI research: %v", err)
		}
		if out == "" {
			t.Error("expected non-empty output from CLI research")
		}
	})
}
