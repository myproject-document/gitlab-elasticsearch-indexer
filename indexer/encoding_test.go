package indexer

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEncodeStringHandlesSlashedO(t *testing.T) {
	str := "Møle"

	result, err := encodeString(str)

	assert.NoError(t, err)
	assert.Equal(t, result, "Møle")
}
