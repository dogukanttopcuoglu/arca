package model

// EntityType defines typed classification for named entities.
type EntityType string

const (
	EntityTypePerson       EntityType = "person"
	EntityTypeOrganization EntityType = "organization"
	EntityTypeLocation     EntityType = "location"
	EntityTypeProduct      EntityType = "product"
	EntityTypeEvent        EntityType = "event"
	EntityTypeMisc         EntityType = "miscellaneous"
)

// EntityMention represents a surface text occurrence of a typed entity.
type EntityMention struct {
	Text       string     `json:"text"`
	Type       EntityType `json:"type"`
	ChunkID    string     `json:"chunk_id"`
	Confidence float64    `json:"confidence"`
}

// Entity represents a document-level aggregated entity record.
type Entity struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Type     EntityType      `json:"type"`
	Aliases  []string        `json:"aliases,omitempty"`
	Mentions []EntityMention `json:"mentions,omitempty"`
	Score    float64         `json:"score"`
}
