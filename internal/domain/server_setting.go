package domain

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
)

type ServerSetting struct {
	ID       uint               `db:"id"`
	Name     string             `db:"name"`
	ServerID uint               `db:"server_id"`
	Value    ServerSettingValue `db:"value"`
}

type serverSettingType int8

const (
	boolTrueText  = "true"
	boolFalseText = "false"
)

const (
	serverSettingTypeUnknown serverSettingType = iota
	serverSettingTypeString
	serverSettingTypeBool
	serverSettingTypeInt
	serverSettingTypeFloat
)

// ServerSettingValue keeps both a guessed Go value and the exact text it came
// from. The guess is what the built-in settings (autostart, update_before_start)
// rely on; the raw text is what game mod variables need, because guessing turns
// "007" into 7 and "0x10" into 16.
type ServerSettingValue struct {
	value any
	raw   string
	tp    serverSettingType
}

func NewServerSettingValue(value any) ServerSettingValue {
	switch v := value.(type) {
	case string:
		return ServerSettingValue{value: v, raw: v, tp: serverSettingTypeString}
	case bool:
		return ServerSettingValue{value: v, raw: strconv.FormatBool(v), tp: serverSettingTypeBool}
	case int:
		return ServerSettingValue{value: v, raw: strconv.Itoa(v), tp: serverSettingTypeInt}
	case int64:
		return ServerSettingValue{value: int(v), raw: strconv.FormatInt(v, 10), tp: serverSettingTypeInt}
	case float64:
		return ServerSettingValue{value: v, raw: formatFloat(v), tp: serverSettingTypeFloat}
	case nil:
		return ServerSettingValue{value: nil, tp: serverSettingTypeUnknown}
	default:
		str := fmt.Sprintf("%v", value)

		return ServerSettingValue{value: str, raw: str, tp: serverSettingTypeString}
	}
}

// formatFloat renders a float in plain decimal notation, never in exponent form,
// so the value can be substituted into a command template as-is.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func (s ServerSettingValue) MarshalJSON() ([]byte, error) {
	switch s.tp {
	case serverSettingTypeString:
		return json.Marshal(s.value.(string))
	case serverSettingTypeBool:
		return json.Marshal(s.value.(bool))
	case serverSettingTypeInt:
		return json.Marshal(s.value.(int))
	case serverSettingTypeFloat:
		return json.Marshal(s.value.(float64))
	default:
		return json.Marshal(nil)
	}
}

func (s *ServerSettingValue) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as different types
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.value = str
		s.raw = str
		s.tp = serverSettingTypeString

		return nil
	}

	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		s.value = b
		s.raw = strconv.FormatBool(b)
		s.tp = serverSettingTypeBool

		return nil
	}

	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		s.value = i
		s.raw = strconv.Itoa(i)
		s.tp = serverSettingTypeInt

		return nil
	}

	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		s.value = f
		s.raw = formatFloat(f)
		s.tp = serverSettingTypeFloat

		return nil
	}

	return nil
}

func (s ServerSettingValue) Any() any {
	return s.value
}

// Raw returns the value exactly as it is stored, without the type guessing that
// String applies. Game mod variables must be read through it: their canonical
// form is a string and any coercion would corrupt values like "007".
func (s ServerSettingValue) Raw() (string, bool) {
	if s.tp == serverSettingTypeUnknown {
		return "", false
	}

	return s.raw, true
}

func (s ServerSettingValue) String() (string, bool) {
	switch s.tp {
	case serverSettingTypeString:
		if str, ok := s.value.(string); ok {
			return str, true
		}
	case serverSettingTypeInt:
		if intVal, ok := s.value.(int); ok {
			return strconv.Itoa(intVal), true
		}
	case serverSettingTypeFloat:
		if floatVal, ok := s.value.(float64); ok {
			return formatFloat(floatVal), true
		}
	case serverSettingTypeBool:
		if boolVal, ok := s.value.(bool); ok {
			if boolVal {
				return boolTrueText, true
			}

			return boolFalseText, true
		}
	}

	return "", false
}

func (s ServerSettingValue) Bool() (bool, bool) {
	if b, ok := s.value.(bool); ok {
		return b, true
	}

	if intVal, ok := s.value.(int); ok {
		return intVal != 0, true
	}

	if strVal, ok := s.value.(string); ok {
		if strVal == boolTrueText {
			return true, true
		}

		if strVal == boolFalseText {
			return false, true
		}
	}

	return false, false
}

func (s ServerSettingValue) Int() (int, bool) {
	if s.tp != serverSettingTypeInt {
		return 0, false
	}

	if i, ok := s.value.(int); ok {
		return i, true
	}

	return 0, false
}

func (s ServerSettingValue) Float() (float64, bool) {
	if s.tp == serverSettingTypeFloat {
		if f, ok := s.value.(float64); ok {
			return f, true
		}
	}

	if s.tp == serverSettingTypeInt {
		if i, ok := s.value.(int); ok {
			return float64(i), true
		}
	}

	return 0, false
}

// Scan implements sql.Scanner interface.
//

func (s *ServerSettingValue) Scan(value any) error {
	// A NULL column is an absent value, exactly like the "null" literal below:
	// leaving it typed as a string would make Raw report a value that is not
	// there and make MarshalJSON assert a nil into a string.
	if value == nil {
		s.value = nil
		s.raw = ""
		s.tp = serverSettingTypeUnknown

		return nil
	}

	// Handle []byte from database
	if b, ok := value.([]byte); ok {
		s.raw = string(b)

		switch {
		case bytes.Equal(b, []byte(boolTrueText)):
			s.value = true
			s.tp = serverSettingTypeBool

			return nil
		case bytes.Equal(b, []byte(boolFalseText)):
			s.value = false
			s.tp = serverSettingTypeBool

			return nil

		case bytes.Equal(b, []byte("null")):
			s.value = nil
			s.raw = ""
			s.tp = serverSettingTypeUnknown

			return nil
		}

		if intVal, err := strconv.ParseInt(string(b), 0, 64); err == nil {
			s.value = int(intVal)
			s.tp = serverSettingTypeInt

			return nil
		}

		// Raw string
		s.value = string(b)
		s.tp = serverSettingTypeString

		return nil
	}

	// Handle direct values
	switch v := value.(type) {
	case string:
		s.value = v
		s.raw = v
		s.tp = serverSettingTypeString
	case bool:
		s.value = v
		s.raw = strconv.FormatBool(v)
		s.tp = serverSettingTypeBool
	case int:
		s.value = v
		s.raw = strconv.Itoa(v)
		s.tp = serverSettingTypeInt
	case int64:
		s.value = int(v)
		s.raw = strconv.FormatInt(v, 10)
		s.tp = serverSettingTypeInt
	case float64:
		s.value = v
		s.raw = formatFloat(v)
		s.tp = serverSettingTypeFloat
	default:
		str := fmt.Sprintf("%v", value)
		s.value = str
		s.raw = str
		s.tp = serverSettingTypeString
	}

	return nil
}

// Value implements driver.Valuer interface.
func (s ServerSettingValue) Value() (driver.Value, error) {
	if s.value == nil {
		return nil, nil
	}

	// The raw text is authoritative: it survives a read-modify-write cycle
	// without the type guessing in Scan rewriting "007" as "7".
	if s.raw != "" {
		return s.raw, nil
	}

	v, _ := s.String()

	return v, nil
}
