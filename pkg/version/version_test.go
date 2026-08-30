package version_test

import (
	"testing"

	"github.com/gameap/gameap/pkg/version"
	"github.com/stretchr/testify/assert"
)

func TestNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want string
	}{
		{name: "bare_version_gets_v_prefix", give: "4.1.2", want: "v4.1.2"},
		{name: "tag_stays_as_is", give: "v4.4.2", want: "v4.4.2"},
		{name: "surrounding_spaces_trimmed", give: "  v4.4.2  ", want: "v4.4.2"},
		{name: "prerelease_preserved", give: "4.5.0-beta.1", want: "v4.5.0-beta.1"},
		{name: "build_metadata_dropped", give: "4.4.2+build.5", want: "v4.4.2"},
		{name: "two_segment_version_accepted", give: "4.4", want: "v4.4"},
		{name: "development_build_is_not_a_version", give: "development", want: ""},
		{name: "dev_build_with_hash_is_not_a_version", give: "dev-a1b2c3", want: ""},
		{name: "empty_string", give: "", want: ""},
		{name: "spaces_only", give: "   ", want: ""},
		{name: "garbage", give: "not.a.version", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, version.Normalize(tt.give))
		})
	}
}

func TestIsNewer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		current   string
		candidate string
		want      bool
	}{
		{name: "patch_bump", current: "4.1.0", candidate: "v4.1.2", want: true},
		{name: "minor_bump_across_prefixes", current: "v4.4.1", candidate: "4.5.0", want: true},
		{name: "equal_versions", current: "4.4.2", candidate: "v4.4.2", want: false},
		{name: "older_candidate", current: "4.4.2", candidate: "4.4.1", want: false},
		{name: "release_beats_own_prerelease", current: "4.5.0-beta.1", candidate: "4.5.0", want: true},
		{name: "prerelease_of_next_minor_beats_stable", current: "4.4.2", candidate: "4.5.0-beta.1", want: true},
		{name: "stable_does_not_beat_newer_prerelease", current: "4.5.0-beta.1", candidate: "4.4.2", want: false},
		{name: "build_metadata_ignored", current: "4.4.2+build.5", candidate: "4.4.2", want: false},
		{name: "development_current_never_outdated", current: "development", candidate: "4.4.2", want: false},
		{name: "unparsable_candidate", current: "4.4.2", candidate: "latest", want: false},
		{name: "both_empty", current: "", candidate: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, version.IsNewer(tt.current, tt.candidate))
		})
	}
}

func TestIsRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want bool
	}{
		{name: "stable_tag", give: "v4.4.2", want: true},
		{name: "stable_bare_version", give: "4.4.2", want: true},
		{name: "stable_with_build_metadata", give: "4.4.2+build.5", want: true},
		{name: "beta", give: "4.5.0-beta.1", want: false},
		{name: "release_candidate", give: "v4.1.0-rc.2", want: false},
		{name: "development_build", give: "development", want: false},
		{name: "empty_string", give: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, version.IsRelease(tt.give))
		})
	}
}
