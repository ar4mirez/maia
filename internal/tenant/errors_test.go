package tenant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrNotFound_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ErrNotFound
		expected string
	}{
		{
			name:     "with ID",
			err:      &ErrNotFound{ID: "test-id"},
			expected: "tenant not found: id=test-id",
		},
		{
			name:     "with name",
			err:      &ErrNotFound{Name: "test-name"},
			expected: "tenant not found: name=test-name",
		},
		{
			name:     "with both (name preferred)",
			err:      &ErrNotFound{ID: "test-id", Name: "test-name"},
			expected: "tenant not found: name=test-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestErrAlreadyExists_Error(t *testing.T) {
	err := &ErrAlreadyExists{Name: "test-tenant"}
	assert.Equal(t, "tenant already exists: name=test-tenant", err.Error())
}

func TestErrInvalidInput_Error(t *testing.T) {
	err := &ErrInvalidInput{Field: "name", Message: "cannot be empty"}
	assert.Equal(t, "invalid input: name - cannot be empty", err.Error())
}

func TestErrQuotaExceeded_Error(t *testing.T) {
	err := &ErrQuotaExceeded{
		TenantID: "tenant-1",
		Resource: "memories",
		Limit:    1000,
		Current:  1001,
	}
	expected := "quota exceeded for tenant tenant-1: memories (limit=1000, current=1001)"
	assert.Equal(t, expected, err.Error())
}

func TestErrTenantSuspended_Error(t *testing.T) {
	err := &ErrTenantSuspended{TenantID: "tenant-1"}
	assert.Equal(t, "tenant is suspended: tenant-1", err.Error())
}

func TestErrTenantPendingDeletion_Error(t *testing.T) {
	err := &ErrTenantPendingDeletion{TenantID: "tenant-1"}
	assert.Equal(t, "tenant is pending deletion: tenant-1", err.Error())
}
