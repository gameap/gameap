package i18n_test

import (
	"testing"
	"testing/fstest"

	"github.com/gameap/gameap/internal/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func file(content string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(content)}
}

func TestListLanguages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fsys fstest.MapFS
		want []i18n.Language
	}{
		{
			name: "reads_labels_from_language_meta_and_sorts_by_code",
			fsys: fstest.MapFS{
				"ru.json": file(`{"_language":{"name":"Russian","native_name":"Русский"},"auth":{}}`),
				"en.json": file(`{"_language":{"name":"English","native_name":"English"}}`),
			},
			want: []i18n.Language{
				{Code: "en", Name: "English", NativeName: "English"},
				{Code: "ru", Name: "Russian", NativeName: "Русский"},
			},
		},
		{
			name: "falls_back_to_code_when_meta_absent",
			fsys: fstest.MapFS{
				"de.json": file(`{"auth":{"sign_in":"Anmelden"}}`),
			},
			want: []i18n.Language{
				{Code: "de", Name: "de", NativeName: "de"},
			},
		},
		{
			name: "ignores_non_json_and_directories",
			fsys: fstest.MapFS{
				"en.json":       file(`{"_language":{"name":"English","native_name":"English"}}`),
				"readme.txt":    file("not a locale"),
				"assets/x.json": file(`{"_language":{"name":"Nested","native_name":"Nested"}}`),
			},
			want: []i18n.Language{
				{Code: "en", Name: "English", NativeName: "English"},
			},
		},
		{
			name: "partial_meta_falls_back_per_field",
			fsys: fstest.MapFS{
				"es.json": file(`{"_language":{"native_name":"Español"}}`),
			},
			want: []i18n.Language{
				{Code: "es", Name: "es", NativeName: "Español"},
			},
		},
		{
			name: "invalid_json_falls_back_to_code",
			fsys: fstest.MapFS{
				"broken.json": file(`{not valid json`),
			},
			want: []i18n.Language{
				{Code: "broken", Name: "broken", NativeName: "broken"},
			},
		},
		{
			name: "empty_filesystem_returns_empty_list",
			fsys: fstest.MapFS{},
			want: []i18n.Language{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := i18n.ListLanguages(tt.fsys)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestListLanguages_RealEmbeddedFiles(t *testing.T) {
	t.Parallel()

	got, err := i18n.ListLanguages(i18n.GetFS())
	require.NoError(t, err)

	byCode := make(map[string]i18n.Language, len(got))
	for _, lang := range got {
		byCode[lang.Code] = lang
	}

	require.Contains(t, byCode, "en")
	require.Contains(t, byCode, "ru")
	assert.Equal(t, i18n.Language{Code: "en", Name: "English", NativeName: "English"}, byCode["en"])
	assert.Equal(t, i18n.Language{Code: "ru", Name: "Russian", NativeName: "Русский"}, byCode["ru"])
}
