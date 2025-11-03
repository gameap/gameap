package flexible

import (
	"encoding/json"
	"time"
)

// Time is a time.Time wrapper that can unmarshal from multiple date formats.
type Time struct {
	time.Time
}

// supportedFormats lists all date/time formats that the API accepts.
var supportedFormats = []string{
	time.RFC3339,                // 2006-01-02T15:04:05Z07:00
	"2006-01-02 15:04:05",       // MySQL datetime format
	"2006-01-02T15:04:05",       // ISO 8601 without timezone
	"2006-01-02 15:04:05 -0700", // With timezone
	time.DateTime,               // 2006-01-02 15:04:05
}

// UnmarshalJSON implements json.Unmarshaler interface to accept multiple date formats.
func (ft *Time) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	for _, format := range supportedFormats {
		if t, err := time.Parse(format, str); err == nil {
			ft.Time = t

			return nil
		}
	}

	// If no format matched, try the default time.Time unmarshal
	return json.Unmarshal(data, &ft.Time)
}

// MarshalJSON implements json.Marshaler interface to output in RFC3339 format.
func (ft Time) MarshalJSON() ([]byte, error) {
	return json.Marshal(ft.Format(time.RFC3339))
}
