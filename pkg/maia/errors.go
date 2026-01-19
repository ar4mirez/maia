package maia

import (
	"fmt"
)

// APIError represents an error returned by the MAIA API.
type APIError struct {
	StatusCode int    `json:"-"`
	Message    string `json:"error"`
	Code       string `json:"code,omitempty"`
	Details    string `json:"details,omitempty"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s (%s)", e.Message, e.Code)
	}
	return e.Message
}

// IsNotFound returns true if the error is a not found error.
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == 404 || e.Code == "NOT_FOUND"
}

// IsAlreadyExists returns true if the error is an already exists error.
func (e *APIError) IsAlreadyExists() bool {
	return e.StatusCode == 409 || e.Code == "ALREADY_EXISTS"
}

// IsInvalidInput returns true if the error is an invalid input error.
func (e *APIError) IsInvalidInput() bool {
	return e.StatusCode == 400 || e.Code == "INVALID_INPUT"
}

// IsServerError returns true if the error is a server error.
func (e *APIError) IsServerError() bool {
	return e.StatusCode >= 500
}

// ErrNotFound is returned when a resource is not found.
type ErrNotFound struct {
	Resource string
	ID       string
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Resource, e.ID)
}

// IsNotFoundError checks if the error is a not found error.
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.IsNotFound()
	}
	if _, ok := err.(*ErrNotFound); ok {
		return true
	}
	return false
}

// IsAlreadyExistsError checks if the error is an already exists error.
func IsAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.IsAlreadyExists()
	}
	return false
}
