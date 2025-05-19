package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnexport(t *testing.T) {
	assert.Equal(t, "a", unexport("A"))
	assert.Equal(t, "ab", unexport("Ab"))
}
