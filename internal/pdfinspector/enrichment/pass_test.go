package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

type MockPass struct {
	name     string
	requires []enrichment.Capability
	provides []enrichment.Capability
	executed bool
}

func (m *MockPass) Name() string                     { return m.name }
func (m *MockPass) Requires() []enrichment.Capability { return m.requires }
func (m *MockPass) Provides() []enrichment.Capability { return m.provides }
func (m *MockPass) Execute(ctx context.Context, input *enrichment.EnrichmentInput) error {
	m.executed = true
	return nil
}

func TestCompositeEnricher(t *testing.T) {
	ctx := context.Background()

	t.Run("executes passes sequentially when capability requirements are met", func(t *testing.T) {
		pass1 := &MockPass{
			name:     "pass-1",
			provides: []enrichment.Capability{enrichment.CapabilityResolvedTitle},
		}
		pass2 := &MockPass{
			name:     "pass-2",
			requires: []enrichment.Capability{enrichment.CapabilityResolvedTitle},
			provides: []enrichment.Capability{enrichment.CapabilityResolvedPages},
		}

		comp := enrichment.NewCompositeEnricher([]enrichment.EnricherPass{pass1, pass2})
		input := &enrichment.EnrichmentInput{
			Metadata: &pdfmodel.DocumentMetadata{},
		}

		report, err := comp.ExecutePasses(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error executing passes: %v", err)
		}

		if report == nil || len(report.StageDurations) == 0 {
			t.Error("expected non-empty StageDurations in report")
		}

		if !pass1.executed || !pass2.executed {
			t.Error("expected all registered passes to be executed")
		}
	})
}
