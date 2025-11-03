package filters

type FindPersonalAccessToken struct {
	IDs            []uint
	Tokens         []string
	TokenableTypes []string
	TokenableIDs   []uint
}

// FilterCount returns the number of different filter types being used (not the total number of values).
// Useful for determining if the filter is specific enough for caching (e.g., FilterCount() == 1 means
// only one filter type is used, making it suitable for single-key caching).
// Example: filter with IDs=[1,2,3] and Tokens=["token1"] returns 2, not 4.
func (f *FindPersonalAccessToken) FilterCount() int {
	count := 0
	if len(f.IDs) > 0 {
		count++
	}

	if len(f.Tokens) > 0 {
		count++
	}

	if len(f.TokenableTypes) > 0 {
		count++
	}

	if len(f.TokenableIDs) > 0 {
		count++
	}

	return count
}

func FindPersonalAccessTokenByIDs(ids ...uint) *FindPersonalAccessToken {
	return &FindPersonalAccessToken{
		IDs: ids,
	}
}
