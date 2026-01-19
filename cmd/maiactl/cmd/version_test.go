package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetVersionInfo(t *testing.T) {
	SetVersionInfo("1.0.0", "abc123", "2026-01-19")

	assert.Equal(t, "1.0.0", version)
	assert.Equal(t, "abc123", commit)
	assert.Equal(t, "2026-01-19", buildTime)
}
