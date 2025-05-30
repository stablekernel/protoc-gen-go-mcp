package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKindToCastType(t *testing.T) {
	type caseType struct {
		kind   string
		isList bool
		expect string
	}

	// Note: Only need one case that is not a special cast type.
	cases := []caseType{{"string", false, "string"}}

	for key, val := range specialCastTypes {
		cases = append(cases, caseType{key, false, val})
		cases = append(cases, caseType{key, true, "[]" + val})
	}

	for _, c := range cases {
		assert.Equal(t, c.expect, kindToCastType(c.kind, c.isList), c)
	}
}

func TestUnexport(t *testing.T) {
	assert.Equal(t, "a", unexport("A"))
	assert.Equal(t, "ab", unexport("Ab"))
}
