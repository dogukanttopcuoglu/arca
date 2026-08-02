package security

import (
	"context"
)

type securityContextKey struct{}

// SecurityContext models an immutable tenant/user security boundary for enterprise multi-tenancy.
type SecurityContext struct {
	TenantID    string   `json:"tenant_id"`
	UserID      string   `json:"user_id"`
	Permissions []string `json:"permissions,omitempty"`
}

// NewSecurityContext constructs a SecurityContext instance.
func NewSecurityContext(tenantID, userID string, permissions []string) *SecurityContext {
	return &SecurityContext{
		TenantID:    tenantID,
		UserID:      userID,
		Permissions: permissions,
	}
}

// HasPermission checks if the given permission string is granted in this context.
func (s *SecurityContext) HasPermission(permission string) bool {
	for _, p := range s.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// WithSecurityContext attaches a SecurityContext to a Go context.Context.
func WithSecurityContext(ctx context.Context, sec *SecurityContext) context.Context {
	return context.WithValue(ctx, securityContextKey{}, sec)
}

// FromContext extracts the SecurityContext from a Go context.Context, if present.
func FromContext(ctx context.Context) (*SecurityContext, bool) {
	sec, ok := ctx.Value(securityContextKey{}).(*SecurityContext)
	return sec, ok
}
