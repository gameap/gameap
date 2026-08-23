package api_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestStrongETag(t *testing.T) {
	t.Parallel()

	etag := api.StrongETag([]byte("bundle"))

	assert.Regexp(t, regexp.MustCompile(`^"[0-9a-f]{64}"$`), etag)
	assert.Equal(t, etag, api.StrongETag([]byte("bundle")), "stable for the same body")
	assert.NotEqual(t, etag, api.StrongETag([]byte("bundle2")))
}

func TestIfNoneMatchSatisfied(t *testing.T) {
	t.Parallel()
	const etag = `"abc123"`

	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "empty_header", header: "", want: false},
		{name: "exact_match", header: `"abc123"`, want: true},
		{name: "weak_prefix_matches", header: `W/"abc123"`, want: true},
		{name: "list_with_spaces_matches_second", header: `"other", "abc123"`, want: true},
		{name: "wildcard", header: "*", want: true},
		{name: "mismatch", header: `"def456"`, want: false},
		{name: "unquoted_does_not_match", header: `abc123`, want: false},
		{name: "wildcard_in_list", header: `"other", *`, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/plugins.js", nil)
			if tt.header != "" {
				req.Header.Set("If-None-Match", tt.header)
			}

			assert.Equal(t, tt.want, api.IfNoneMatchSatisfied(req, etag))
		})
	}
}

func TestIfNoneMatchSatisfied_multiple_header_lines(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/plugins.js", nil)
	req.Header.Add("If-None-Match", `"first"`)
	req.Header.Add("If-None-Match", `"second"`)

	assert.True(t, api.IfNoneMatchSatisfied(req, `"second"`), "every field line is considered")
	assert.True(t, api.IfNoneMatchSatisfied(req, `"first"`))
	assert.False(t, api.IfNoneMatchSatisfied(req, `"third"`))
}
