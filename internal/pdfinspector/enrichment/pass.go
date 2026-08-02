package enrichment

import (
	"context"
	"fmt"
)

// Capability models explicit compiler-pass style inputs and outputs for enrichment stages.
type Capability string

const (
	CapabilityRawMetadata   Capability = "raw_metadata"
	CapabilitySemanticTree  Capability = "semantic_tree"
	CapabilityResolvedTitle Capability = "resolved_title"
	CapabilityResolvedPages Capability = "resolved_pages"
	CapabilityKeywords      Capability = "keywords"
)

// EnricherPass defines the seam for an isolated enrichment stage (compiler pass pattern).
type EnricherPass interface {
	Name() string
	Requires() []Capability
	Provides() []Capability
	Execute(ctx context.Context, input *EnrichmentInput) error
}

// CompositeEnricher manages and executes an ordered sequence of EnricherPass instances with capability verification.
type CompositeEnricher struct {
	passes []EnricherPass
}

// NewCompositeEnricher constructs a CompositeEnricher instance.
func NewCompositeEnricher(passes []EnricherPass) *CompositeEnricher {
	return &CompositeEnricher{
		passes: passes,
	}
}

// ExecutePasses validates capability contracts and executes each pass sequentially.
func (c *CompositeEnricher) ExecutePasses(ctx context.Context, input *EnrichmentInput) error {
	if input == nil {
		return fmt.Errorf("enrichment input cannot be nil")
	}

	available := make(map[Capability]bool)
	available[CapabilityRawMetadata] = true
	if input.Tree != nil {
		available[CapabilitySemanticTree] = true
	}

	for _, pass := range c.passes {
		if pass == nil {
			continue
		}

		// Verify required capabilities
		for _, req := range pass.Requires() {
			if !available[req] {
				return fmt.Errorf("pass %q missing required capability %q", pass.Name(), req)
			}
		}

		if err := pass.Execute(ctx, input); err != nil {
			return fmt.Errorf("pass %q failed: %w", pass.Name(), err)
		}

		// Mark provided capabilities as available
		for _, prov := range pass.Provides() {
			available[prov] = true
		}
	}

	return nil
}

// TitleAuthorPass implements EnricherPass for document title and author resolution.
type TitleAuthorPass struct {
	titleResolver  TitleResolver
	authorResolver AuthorResolver
}

// NewTitleAuthorPass constructs a TitleAuthorPass instance.
func NewTitleAuthorPass(tr TitleResolver, ar AuthorResolver) *TitleAuthorPass {
	if tr == nil {
		tr = NewDefaultTitleResolverChain()
	}
	if ar == nil {
		ar = NewDefaultAuthorResolverChain()
	}
	return &TitleAuthorPass{
		titleResolver:  tr,
		authorResolver: ar,
	}
}

func (p *TitleAuthorPass) Name() string                     { return "TitleAuthorPass" }
func (p *TitleAuthorPass) Requires() []Capability           { return []Capability{CapabilityRawMetadata} }
func (p *TitleAuthorPass) Provides() []Capability           { return []Capability{CapabilityResolvedTitle} }
func (p *TitleAuthorPass) Execute(ctx context.Context, input *EnrichmentInput) error {
	if input.Metadata != nil {
		input.Metadata.Title = p.titleResolver.ResolveTitle(*input.Metadata, input.Tree, input.PageMap, input.Filename)
		input.Metadata.Author = p.authorResolver.ResolveAuthor(*input.Metadata, input.PageMap, input.Filename)
	}
	return nil
}

// PageResolutionPass implements EnricherPass for SemanticTree page mapping.
type PageResolutionPass struct{}

// NewPageResolutionPass constructs a PageResolutionPass instance.
func NewPageResolutionPass() *PageResolutionPass {
	return &PageResolutionPass{}
}

func (p *PageResolutionPass) Name() string                     { return "PageResolutionPass" }
func (p *PageResolutionPass) Requires() []Capability           { return []Capability{CapabilitySemanticTree} }
func (p *PageResolutionPass) Provides() []Capability           { return []Capability{CapabilityResolvedPages} }
func (p *PageResolutionPass) Execute(ctx context.Context, input *EnrichmentInput) error {
	if input.Tree != nil {
		_ = EnrichSemanticTree(input.Tree, input.PageMap, input.Chunks)
	}
	return nil
}
