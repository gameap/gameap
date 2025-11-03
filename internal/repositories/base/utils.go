package base

import (
	"reflect"
	"time"
)

func allFields(v any) []string {
	fields := make([]string, 0)
	val := reflect.ValueOf(v)
	for i := 0; i < val.NumField(); i++ {
		tagValue := val.Type().Field(i).Tag.Get("db")
		if tagValue == "-" {
			continue
		}

		fields = append(fields, tagValue)
	}

	return fields
}

func ParseTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05.999999999",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Parse(time.RFC3339, s)
}
