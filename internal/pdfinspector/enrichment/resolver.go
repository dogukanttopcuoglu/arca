package enrichment

import (
	"strings"

	"arca/internal/pdfinspector/model"
)

// TitleResolver defines the seam for resolving document title metadata without hardcoded fallbacks.
type TitleResolver interface {
	ResolveTitle(doc model.DocumentMetadata, tree *model.SemanticTree, pageMap []model.PageMap, filename string) string
}

// AuthorResolver defines the seam for resolving document author metadata.
type AuthorResolver interface {
	ResolveAuthor(doc model.DocumentMetadata, pageMap []model.PageMap, filename string) string
}

// DefaultTitleResolverChain implements TitleResolver using a priority fallback chain without hardcoded book titles.
type DefaultTitleResolverChain struct{}

// NewDefaultTitleResolverChain constructs a DefaultTitleResolverChain instance.
func NewDefaultTitleResolverChain() *DefaultTitleResolverChain {
	return &DefaultTitleResolverChain{}
}

// ResolveTitle executes priority resolution across PDF Metadata -> Cover Headings -> Filename -> "Untitled Document".
func (r *DefaultTitleResolverChain) ResolveTitle(doc model.DocumentMetadata, tree *model.SemanticTree, pageMap []model.PageMap, filename string) string {
	// Priority 1: Check existing PDF metadata title (if non-generic)
	if doc.Title != "" && !isGenericHeading(doc.Title) {
		return doc.Title
	}

	// Priority 2: Check cover / early page headings (pages 1-5)
	if tree != nil {
		for _, node := range tree.RootNodes {
			if len(node.PageNumbers) > 0 && node.PageNumbers[0] <= 5 && !isGenericHeading(node.Heading) {
				return node.Heading
			}
		}
	}

	// Priority 3: Check filename for clean title pattern
	if filename != "" {
		cleanFn := strings.TrimSuffix(filename, ".pdf")
		cleanFn = strings.ReplaceAll(cleanFn, "-", " ")
		cleanFn = strings.ReplaceAll(cleanFn, "_", " ")
		cleanFn = strings.TrimSpace(cleanFn)
		if cleanFn != "" && !isGenericHeading(cleanFn) {
			return strings.Title(cleanFn)
		}
	}

	// Default fallback: Never hardcode a specific book title!
	return "Untitled Document"
}

// DefaultAuthorResolverChain implements AuthorResolver using priority resolution.
type DefaultAuthorResolverChain struct{}

// NewDefaultAuthorResolverChain constructs a DefaultAuthorResolverChain instance.
func NewDefaultAuthorResolverChain() *DefaultAuthorResolverChain {
	return &DefaultAuthorResolverChain{}
}

// ResolveAuthor executes priority author resolution.
func (r *DefaultAuthorResolverChain) ResolveAuthor(doc model.DocumentMetadata, pageMap []model.PageMap, filename string) string {
	if doc.Author != "" && !isGenericHeading(doc.Author) {
		return doc.Author
	}

	for _, pm := range pageMap {
		if pm.PageNumber <= 5 {
			lines := strings.Split(pm.Markdown, "\n")
			for _, line := range lines {
				lower := strings.ToLower(line)
				if strings.HasPrefix(lower, "by ") || strings.HasPrefix(lower, "written by ") {
					author := strings.TrimPrefix(lower, "written by ")
					author = strings.TrimPrefix(author, "by ")
					author = strings.TrimSpace(author)
					if author != "" {
						return strings.Title(author)
					}
				}
			}
		}
	}

	return "Unknown Author"
}
