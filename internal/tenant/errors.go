package tenant

import "fmt"

// ErrNotFound is returned when a tenant is not found.
type ErrNotFound struct {
	ID   string
	Name string
}

func (e *ErrNotFound) Error() string {
	if e.Name != "" {
		return fmt.Sprintf("tenant not found: name=%s", e.Name)
	}
	return fmt.Sprintf("tenant not found: id=%s", e.ID)
}

// ErrAlreadyExists is returned when a tenant already exists.
type ErrAlreadyExists struct {
	Name string
}

func (e *ErrAlreadyExists) Error() string {
	return fmt.Sprintf("tenant already exists: name=%s", e.Name)
}

// ErrInvalidInput is returned when input validation fails.
type ErrInvalidInput struct {
	Field   string
	Message string
}

func (e *ErrInvalidInput) Error() string {
	return fmt.Sprintf("invalid input: %s - %s", e.Field, e.Message)
}

// ErrQuotaExceeded is returned when a tenant exceeds their quota.
type ErrQuotaExceeded struct {
	TenantID string
	Resource string
	Limit    int64
	Current  int64
}

func (e *ErrQuotaExceeded) Error() string {
	return fmt.Sprintf("quota exceeded for tenant %s: %s (limit=%d, current=%d)",
		e.TenantID, e.Resource, e.Limit, e.Current)
}

// ErrTenantSuspended is returned when an operation is attempted on a suspended tenant.
type ErrTenantSuspended struct {
	TenantID string
}

func (e *ErrTenantSuspended) Error() string {
	return fmt.Sprintf("tenant is suspended: %s", e.TenantID)
}

// ErrTenantPendingDeletion is returned when an operation is attempted on a tenant pending deletion.
type ErrTenantPendingDeletion struct {
	TenantID string
}

func (e *ErrTenantPendingDeletion) Error() string {
	return fmt.Sprintf("tenant is pending deletion: %s", e.TenantID)
}

// ErrAPIKeyNotFound is returned when an API key is not found.
type ErrAPIKeyNotFound struct {
	Key string
}

func (e *ErrAPIKeyNotFound) Error() string {
	return fmt.Sprintf("API key not found: %s", e.Key)
}

// ErrAPIKeyExpired is returned when an API key has expired.
type ErrAPIKeyExpired struct {
	Key string
}

func (e *ErrAPIKeyExpired) Error() string {
	return fmt.Sprintf("API key expired: %s", e.Key)
}

// ErrInsufficientScope is returned when an API key lacks the required scope.
type ErrInsufficientScope struct {
	Required string
	Provided []string
}

func (e *ErrInsufficientScope) Error() string {
	return fmt.Sprintf("insufficient scope: required=%s, provided=%v", e.Required, e.Provided)
}
