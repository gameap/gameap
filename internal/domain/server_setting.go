package domain

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
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
	serverSettingTypeUnknown serverSettingType = iota
	serverSettingTypeString
	serverSettingTypeBool
	serverSettingTypeInt
)

type ServerSettingValue struct {
	value any
	tp    serverSettingType
}

func NewServerSettingValue(value any) ServerSettingValue {
	switch v := value.(type) {
	case string:
		return ServerSettingValue{value: v, tp: serverSettingTypeString}
	case bool:
		return ServerSettingValue{value: v, tp: serverSettingTypeBool}
	case int:
		return ServerSettingValue{value: v, tp: serverSettingTypeInt}
	case int64:
		return ServerSettingValue{value: int(v), tp: serverSettingTypeInt}
	case float64:
		return ServerSettingValue{value: int(v), tp: serverSettingTypeInt}
	case nil:
		return ServerSettingValue{value: nil, tp: serverSettingTypeUnknown}
	default:
		// Fallback to string representation
		str := ""
		if v != nil {
			str = strconv.FormatInt(int64(v.(int)), 10)
		}

		return ServerSettingValue{value: str, tp: serverSettingTypeString}
	}
}

func (s ServerSettingValue) MarshalJSON() ([]byte, error) {
	switch s.tp {
	case serverSettingTypeString:
		return json.Marshal(s.value.(string))
	case serverSettingTypeBool:
		return json.Marshal(s.value.(bool))
	case serverSettingTypeInt:
		return json.Marshal(s.value.(int))
	default:
		return json.Marshal(nil)
	}
}

func (s *ServerSettingValue) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as different types
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.value = str

		return nil
	}

	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		s.value = b

		return nil
	}

	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		s.value = i

		return nil
	}

	return nil
}

func (s ServerSettingValue) Any() any {
	return s.value
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
	case serverSettingTypeBool:
		if boolVal, ok := s.value.(bool); ok {
			if boolVal {
				return "true", true
			}

			return "false", true
		}
	default:
		return "", false
	}

	return "", false
}

func (s ServerSettingValue) Bool() (bool, bool) {
	if s.tp == serverSettingTypeUnknown {
		return false, false
	}

	if b, ok := s.value.(bool); ok {
		return b, true
	}

	if intVal, ok := s.value.(int); ok {
		return intVal != 0, true
	}

	if strVal, ok := s.value.(string); ok {
		if strVal == "true" {
			return true, true
		}

		if strVal == "false" {
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

// Scan implements sql.Scanner interface.
//

func (s *ServerSettingValue) Scan(value any) error {
	if value == nil {
		s.value = nil
		s.tp = serverSettingTypeString

		return nil
	}

	// Handle []byte from database
	if b, ok := value.([]byte); ok {
		switch {
		case bytes.Equal(b, []byte("true")):
			s.value = true
			s.tp = serverSettingTypeBool

			return nil
		case bytes.Equal(b, []byte("false")):
			s.value = false
			s.tp = serverSettingTypeBool

			return nil

		case bytes.Equal(b, []byte("null")):
			s.value = nil
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
		s.tp = serverSettingTypeString
	case bool:
		s.value = v
		s.tp = serverSettingTypeBool
	case int:
		s.value = v
		s.tp = serverSettingTypeInt
	case int64:
		s.value = int(v)
		s.tp = serverSettingTypeInt
	default:
		s.value = value
		s.tp = serverSettingTypeString
	}

	return nil
}

// Value implements driver.Valuer interface.
func (s ServerSettingValue) Value() (driver.Value, error) {
	if s.value == nil {
		return nil, nil
	}

	switch s.tp {
	case serverSettingTypeString:
		v, _ := s.String()

		return v, nil
	case serverSettingTypeBool:
		v, _ := s.Bool()

		return v, nil
	case serverSettingTypeInt:
		v, _ := s.Int()

		return v, nil
	default:
		return nil, nil
	}
}
