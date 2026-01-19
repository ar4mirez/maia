package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrNotFound_Error(t *testing.T) {
	err := &ErrNotFound{Type: "memory", ID: "test-123"}
	assert.Equal(t, "memory not found: test-123", err.Error())
}

func TestErrAlreadyExists_Error(t *testing.T) {
	err := &ErrAlreadyExists{Type: "namespace", ID: "test-ns"}
	assert.Equal(t, "namespace already exists: test-ns", err.Error())
}

func TestErrInvalidInput_Error(t *testing.T) {
	err := &ErrInvalidInput{Field: "content", Message: "cannot be empty"}
	assert.Equal(t, "invalid content: cannot be empty", err.Error())
}
