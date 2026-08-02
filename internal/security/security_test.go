package security_test

import (
	"context"
	"testing"

	"arca/internal/security"
)

func TestSecurityContext(t *testing.T) {
	t.Run("creates valid SecurityContext for tenant isolation", func(t *testing.T) {
		sec := security.NewSecurityContext("tenant-acme", "user-123", []string{"read", "search"})

		if sec.TenantID != "tenant-acme" {
			t.Errorf("expected TenantID 'tenant-acme', got %s", sec.TenantID)
		}
		if sec.UserID != "user-123" {
			t.Errorf("expected UserID 'user-123', got %s", sec.UserID)
		}
		if !sec.HasPermission("read") {
			t.Error("expected HasPermission('read') to be true")
		}
		if sec.HasPermission("admin") {
			t.Error("expected HasPermission('admin') to be false")
		}
	})

	t.Run("binds SecurityContext to Go context", func(t *testing.T) {
		sec := security.NewSecurityContext("tenant-acme", "user-123", nil)
		ctx := security.WithSecurityContext(context.Background(), sec)

		retrieved, ok := security.FromContext(ctx)
		if !ok || retrieved == nil {
			t.Fatal("expected to retrieve SecurityContext from context")
		}
		if retrieved.TenantID != "tenant-acme" {
			t.Errorf("expected TenantID 'tenant-acme', got %s", retrieved.TenantID)
		}
	})
}
