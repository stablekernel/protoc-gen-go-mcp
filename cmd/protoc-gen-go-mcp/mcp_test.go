package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnexport(t *testing.T) {
	assert.Equal(t, "a", unexport("A"))
	assert.Equal(t, "ab", unexport("Ab"))
}

func TestQuoteBacktickString(t *testing.T) {
	assert.Equal(t, "`abc`", quoteBacktickString("abc"))
	assert.Equal(t, `"a`+"`"+`b"`, quoteBacktickString("a`b"))
}
