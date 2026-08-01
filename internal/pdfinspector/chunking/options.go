package chunking

// Options configures parameters for the chunking engine.
type Options struct {
	TargetMinTokens   int
	TargetMaxTokens   int
	SoftMaxTokens     int
	AbsoluteMaxTokens int
	DocumentID        string
	Sizer             ChunkSizer
}

// DefaultOptions returns the default production chunking options matching ADR 0004.
func DefaultOptions() Options {
	return Options{
		TargetMinTokens:   400,
		TargetMaxTokens:   700,
		SoftMaxTokens:     1000,
		AbsoluteMaxTokens: 1200,
		DocumentID:        "doc-1",
		Sizer:             NewHeuristicSizer(),
	}
}

// Option modifies Options.
type Option func(*Options)

// WithTargetBounds sets the min and target max token bounds.
func WithTargetBounds(minTokens, maxTokens int) Option {
	return func(o *Options) {
		if minTokens > 0 {
			o.TargetMinTokens = minTokens
		}
		if maxTokens > 0 {
			o.TargetMaxTokens = maxTokens
		}
	}
}

// WithMaxBounds sets the soft max and absolute max token limits.
func WithMaxBounds(softMax, absoluteMax int) Option {
	return func(o *Options) {
		if softMax > 0 {
			o.SoftMaxTokens = softMax
		}
		if absoluteMax > 0 {
			o.AbsoluteMaxTokens = absoluteMax
		}
	}
}

// WithDocumentID sets the document ID used in chunk IDs and fingerprints.
func WithDocumentID(docID string) Option {
	return func(o *Options) {
		if docID != "" {
			o.DocumentID = docID
		}
	}
}

// WithSizer sets the ChunkSizer implementation.
func WithSizer(sizer ChunkSizer) Option {
	return func(o *Options) {
		if sizer != nil {
			o.Sizer = sizer
		}
	}
}
