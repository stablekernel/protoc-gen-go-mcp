package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/compiler/protogen"
)

func TestUnexport(t *testing.T) {
	assert.Equal(t, "a", unexport("A"))
	assert.Equal(t, "ab", unexport("Ab"))
}

func TestQuoteBacktickString(t *testing.T) {
	assert.Equal(t, "`abc`", quoteBacktickString("abc"))
	assert.Equal(t, `"a`+"`"+`b"`, quoteBacktickString("a`b"))
}

// TestProcessCommentToString covers the two bugs in issue #100: a
// backslash or backtick in a proto comment must come through completely
// untouched (callers, not this function, are responsible for escaping the
// text for whatever they embed it in), and a double quote must likewise
// come through unescaped rather than backslash-escaped here (which used to
// make json.Marshal double-escape it when schema.go embedded the result in
// a JSON Schema "description").
func TestProcessCommentToString(t *testing.T) {
	tests := []struct {
		name string
		in   protogen.Comments
		want string
	}{
		{
			name: "quote",
			in:   protogen.Comments(` Must match "chill" exactly.` + "\n"),
			want: `Must match "chill" exactly.`,
		},
		{
			name: "backslash",
			in:   protogen.Comments(` Must match \d+.` + "\n"),
			want: `Must match \d+.`,
		},
		{
			name: "backtick",
			in:   protogen.Comments(" Use the `vibe` field.\n"),
			want: "Use the `vibe` field.",
		},
		{
			name: "multiline joins with a single space",
			in:   protogen.Comments(" First line.\n Second line.\n"),
			want: "First line. Second line.",
		},
		{
			name: "block comment",
			in:   protogen.Comments("/* Block \"comment\" with \\backslash\\. */"),
			want: `Block "comment" with \backslash\.`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, processCommentToString(tt.in))
		})
	}
}
