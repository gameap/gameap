package flexible

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrNegativeValue = errors.New("cannot convert negative value to uint")
	ErrInvalidType   = errors.New("cannot convert type to uint")
)

type Uint uint

func (u *Uint) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	val, err := anyToUint(v)
	if err != nil {
		return err
	}

	*u = Uint(val)

	return nil
}

func (u Uint) MarshalJSON() ([]byte, error) {
	return json.Marshal(uint(u))
}

func (u *Uint) Uint() uint {
	if u == nil {
		return 0
	}

	return uint(*u)
}

func anyToUint(v any) (uint, error) {
	switch val := v.(type) {
	case string:
		if val == "" {
			return 0, nil
		}
		parsed, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot parse string as uint: %w", err)
		}

		return uint(parsed), nil
	case float64:
		if val < 0 {
			return 0, fmt.Errorf("%w: %f", ErrNegativeValue, val)
		}

		return uint(val), nil
	case int:
		if val < 0 {
			return 0, fmt.Errorf("%w: %d", ErrNegativeValue, val)
		}

		return uint(val), nil
	case int8:
		if val < 0 {
			return 0, fmt.Errorf("%w: %d", ErrNegativeValue, val)
		}

		return uint(val), nil
	case int16:
		if val < 0 {
			return 0, fmt.Errorf("%w: %d", ErrNegativeValue, val)
		}

		return uint(val), nil
	case int32:
		if val < 0 {
			return 0, fmt.Errorf("%w: %d", ErrNegativeValue, val)
		}

		return uint(val), nil
	case int64:
		if val < 0 {
			return 0, fmt.Errorf("%w: %d", ErrNegativeValue, val)
		}

		return uint(val), nil
	case uint:
		return val, nil
	case uint8:
		return uint(val), nil
	case uint16:
		return uint(val), nil
	case uint32:
		return uint(val), nil
	case uint64:
		return uint(val), nil
	case float32:
		if val < 0 {
			return 0, fmt.Errorf("%w: %f", ErrNegativeValue, val)
		}

		return uint(val), nil
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("%w: %T", ErrInvalidType, v)
	}
}
