package i18n

import (
	"encoding/json"
	"io/fs"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

// Language describes an available UI locale for the /lang listing.
type Language struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	NativeName string `json:"native_name"`
}

// languageMeta is the optional "_language" object embedded in a translation
// file, carrying its display labels (the English name and the language's own
// name). Files without it fall back to the locale code.
type languageMeta struct {
	Language struct {
		Name       string `json:"name"`
		NativeName string `json:"native_name"`
	} `json:"_language"`
}

// ListLanguages enumerates the available locales in fsys — one per top-level
// *.json file — with display labels read from each file's "_language" metadata.
// A file without metadata falls back to its code for both labels. The result is
// sorted by code. fsys is expected to be the merged i18n filesystem, so
// plugin-contributed locales are included.
func ListLanguages(fsys fs.FS) ([]Language, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, errors.Wrap(err, "read i18n directory")
	}

	languages := make([]Language, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		languages = append(languages, readLanguage(fsys, entry.Name()))
	}

	sort.Slice(languages, func(i, j int) bool {
		return languages[i].Code < languages[j].Code
	})

	return languages, nil
}

func readLanguage(fsys fs.FS, filename string) Language {
	code := strings.TrimSuffix(filename, ".json")
	lang := Language{Code: code, Name: code, NativeName: code}

	data, err := fs.ReadFile(fsys, filename)
	if err != nil {
		return lang
	}

	var meta languageMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return lang
	}

	if meta.Language.Name != "" {
		lang.Name = meta.Language.Name
	}

	if meta.Language.NativeName != "" {
		lang.NativeName = meta.Language.NativeName
	}

	return lang
}
