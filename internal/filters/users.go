package filters

type FindUser struct {
	IDs    []uint
	Logins []string
	Emails []string
}

// FilterCount returns the number of different filter types being used (not the total number of values).
// Useful for determining if the filter is specific enough for caching (e.g., FilterCount() == 1 means
// only one filter type is used, making it suitable for single-key caching).
// Example: filter with IDs=[1,2,3] and Logins=["user1"] returns 2, not 4.
func (f *FindUser) FilterCount() int {
	count := 0
	if len(f.IDs) > 0 {
		count++
	}

	if len(f.Logins) > 0 {
		count++
	}

	if len(f.Emails) > 0 {
		count++
	}

	return count
}

func FindUserByIDs(ids ...uint) *FindUser {
	return &FindUser{
		IDs: ids,
	}
}

func FindUserByLogins(logins ...string) *FindUser {
	return &FindUser{
		Logins: logins,
	}
}

func FindUserByEmails(emails ...string) *FindUser {
	return &FindUser{
		Emails: emails,
	}
}
