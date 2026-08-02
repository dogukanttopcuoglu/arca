package model

import (
	"crypto/sha256"
	"fmt"
)

// CalculateIndexSignature generates a deterministic SHA-256 composite hash signature for differential re-indexing comparison.
func CalculateIndexSignature(contentHash, provider, modelName, version, schemaVer string) string {
	raw := fmt.Sprintf("%s:%s:%s:%s:%s", contentHash, provider, modelName, version, schemaVer)
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h)
}
