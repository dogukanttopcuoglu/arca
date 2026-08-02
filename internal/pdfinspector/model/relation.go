package model

// RelationSource defines the typed provenance of an extracted relationship.
type RelationSource string

const (
	RelationSourceRuleBased RelationSource = "rule_based"
	RelationSourceLLM       RelationSource = "llm"
	RelationSourceHybrid    RelationSource = "hybrid"
)

// RelationType defines typed predicates for directed Knowledge Graph edges.
type RelationType string

const (
	RelationTypeFoundedBy  RelationType = "founded_by"
	RelationTypeLocatedIn  RelationType = "located_in"
	RelationTypePartOf     RelationType = "part_of"
	RelationTypeRelatesTo  RelationType = "relates_to"
	RelationTypeAuthorOf   RelationType = "author_of"
	RelationTypeAssociated RelationType = "associated_with"
)

// Relation represents a minimal directed Subject-Predicate-Object relationship.
type Relation struct {
	ID         string         `json:"id"`
	SubjectID  string         `json:"subject_id"`
	Predicate  RelationType   `json:"predicate"`
	ObjectID   string         `json:"object_id"`
	Confidence float64        `json:"confidence"`
	ChunkID    string         `json:"chunk_id,omitempty"`
	Source     RelationSource `json:"source"`
}
