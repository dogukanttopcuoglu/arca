package enrichment

import (
	"context"
	"sort"
	"strings"
	"unicode"

	pdfmodel "arca/internal/pdfinspector/model"
)

var englishStopwords = map[string]bool{
	"a": true, "about": true, "above": true, "after": true, "again": true, "against": true,
	"all": true, "am": true, "an": true, "and": true, "any": true, "are": true, "aren't": true,
	"as": true, "at": true, "be": true, "because": true, "been": true, "before": true, "being": true,
	"below": true, "between": true, "both": true, "but": true, "by": true, "can": true, "cannot": true,
	"could": true, "did": true, "do": true, "does": true, "doing": true, "down": true, "during": true,
	"each": true, "few": true, "for": true, "from": true, "further": true, "had": true, "has": true,
	"have": true, "having": true, "he": true, "her": true, "here": true, "hers": true, "herself": true,
	"him": true, "himself": true, "his": true, "how": true, "i": true, "if": true, "in": true,
	"into": true, "is": true, "it": true, "its": true, "itself": true, "more": true, "most": true,
	"no": true, "nor": true, "not": true, "of": true, "off": true, "on": true, "once": true,
	"only": true, "or": true, "other": true, "ought": true, "our": true, "ours": true, "ourselves": true,
	"out": true, "over": true, "own": true, "same": true, "she": true, "should": true, "so": true,
	"some": true, "such": true, "than": true, "that": true, "the": true, "their": true, "theirs": true,
	"them": true, "themselves": true, "then": true, "there": true, "these": true, "they": true,
	"this": true, "those": true, "through": true, "to": true, "too": true, "under": true, "until": true,
	"up": true, "very": true, "was": true, "we": true, "were": true, "what": true, "when": true,
	"where": true, "which": true, "while": true, "who": true, "whom": true, "why": true, "with": true,
	"would": true, "you": true, "your": true, "yours": true, "yourself": true, "yourselves": true,
}

var turkishStopwords = map[string]bool{
	"acaba": true, "ama": true, "aslında": true, "az": true, "bazı": true, "belki": true,
	"biri": true, "birkaç": true, "birşey": true, "biz": true, "bu": true, "buna": true,
	"bunda": true, "bundan": true, "burada": true, "böyle": true, "da": true, "daha": true,
	"dahi": true, "de": true, "defa": true, "değil": true, "diğer": true, "diye": true,
	"en": true, "gibi": true, "hem": true, "hep": true, "hepsi": true, "her": true,
	"hiç": true, "için": true, "ile": true, "ise": true, "katrilyon": true, "kendi": true,
	"kendine": true, "kendini": true, "ki": true, "kim": true, "mı": true, "mu": true,
	"mü": true, "nasıl": true, "ne": true, "neden": true, "nerde": true, "nerede": true,
	"nereye": true, "niçin": true, "niye": true, "o": true, "onlar": true, "onlara": true,
	"onlardan": true, "onları": true, "onların": true, "orada": true, "oysa": true,
	"pek": true, "sadece": true, "sanki": true, "sen": true, "siz": true, "sonra": true,
	"tüm": true, "ve": true, "veya": true, "ya": true, "yani": true, "yine": true, "yok": true,
}

// KeywordExtractor defines the strategy seam for extracting keywords from chunks.
// Entities are provided so implementations can suppress entity-fragment tokens from the lexical index.
type KeywordExtractor interface {
	Extract(ctx context.Context, chunks []pdfmodel.KnowledgeChunk, lang string, entities []pdfmodel.Entity) ([]pdfmodel.Keyword, error)
}

// RuleBasedKeywordExtractor implements KeywordExtractor using term-frequency and stopword filtering.
type RuleBasedKeywordExtractor struct{}

// NewRuleBasedKeywordExtractor constructs a RuleBasedKeywordExtractor instance.
func NewRuleBasedKeywordExtractor() *RuleBasedKeywordExtractor {
	return &RuleBasedKeywordExtractor{}
}

// Extract executes term-frequency keyword extraction and returns sorted, deterministic keywords.
// Entity fragment tokens (sub-tokens of any extracted entity name) are suppressed from the output.
func (e *RuleBasedKeywordExtractor) Extract(ctx context.Context, chunks []pdfmodel.KnowledgeChunk, lang string, entities []pdfmodel.Entity) ([]pdfmodel.Keyword, error) {
	if len(chunks) == 0 {
		return []pdfmodel.Keyword{}, nil
	}

	entityFragments := buildEntityFragmentBlocklist(entities)

	freqMap := make(map[string]float64)
	chunkMap := make(map[string][]string)

	stopwords := englishStopwords
	if strings.ToLower(lang) == "tr" {
		stopwords = turkishStopwords
	}

	for _, ch := range chunks {
		words := tokenizeAndNormalize(ch.ContentMarkdown)
		seenInChunk := make(map[string]bool)

		for _, w := range words {
			if len(w) <= 2 || isDigitOnly(w) {
				continue
			}
			if stopwords[w] || englishStopwords[w] || turkishStopwords[w] {
				continue
			}
			// Entity-aware filtering: suppress sub-tokens of extracted entity names
			if entityFragments[w] {
				continue
			}

			freqMap[w] += 1.0

			if !seenInChunk[w] {
				seenInChunk[w] = true
				if ch.ChunkID != "" {
					chunkMap[w] = append(chunkMap[w], ch.ChunkID)
				}
			}
		}
	}

	if len(freqMap) == 0 {
		return []pdfmodel.Keyword{}, nil
	}

	// Normalize scores between 0.0 and 1.0 based on max frequency
	maxFreq := 0.0
	for _, f := range freqMap {
		if f > maxFreq {
			maxFreq = f
		}
	}

	var keywords []pdfmodel.Keyword
	for word, freq := range freqMap {
		score := freq / maxFreq
		keywords = append(keywords, pdfmodel.Keyword{
			Value:    word,
			Score:    score,
			Source:   pdfmodel.KeywordSourceRuleBased,
			ChunkIDs: chunkMap[word],
		})
	}

	// Deterministic sorting: Descending by Score, then Ascending alphabetically by Value
	sort.Slice(keywords, func(i, j int) bool {
		if keywords[i].Score != keywords[j].Score {
			return keywords[i].Score > keywords[j].Score
		}
		return keywords[i].Value < keywords[j].Value
	})

	// Limit top 15 keywords
	if len(keywords) > 15 {
		keywords = keywords[:15]
	}

	return keywords, nil
}

func tokenizeAndNormalize(text string) []string {
	lower := strings.ToLower(text)
	var buf strings.Builder

	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' {
			buf.WriteRune(r)
		} else {
			buf.WriteRune(' ')
		}
	}

	return strings.Fields(buf.String())
}

func isDigitOnly(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// buildEntityFragmentBlocklist splits each entity canonical name into lowercase tokens
// and returns them as a blocklist set. This prevents entity sub-tokens like "def", "jam",
// "new", "york" from leaking into the keyword lexical index when compound named entities
// like "Def Jam Recordings" or "New York" are present in the extraction output.
func buildEntityFragmentBlocklist(entities []pdfmodel.Entity) map[string]bool {
	blocklist := make(map[string]bool)
	for _, e := range entities {
		tokens := strings.Fields(strings.ToLower(e.Name))
		for _, tok := range tokens {
			// Only block tokens that are at least 3 chars to avoid over-suppression
			if len(tok) >= 3 {
				blocklist[tok] = true
			}
		}
	}
	return blocklist
}
