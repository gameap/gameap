package flexible

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
)

var (
	ErrInvalidIntType = errors.New("cannot convert type to int")
	ErrIntOverflow    = errors.New("integer overflow")
)

type Int int

func (i *Int) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	val, err := anyToInt(v)
	if err != nil {
		return err
	}

	*i = Int(val)

	return nil
}

func (i Int) MarshalJSON() ([]byte, error) {
	return json.Marshal(int(i))
}

func (i *Int) Int() int {
	if i == nil {
		return 0
	}

	return int(*i)
}

func anyToInt(v any) (int, error) {
	switch val := v.(type) {
	case string:
		if val == "" {
			return 0, nil
		}
		parsed, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot parse string as int: %w", err)
		}

		return int(parsed), nil
	case float64:
		return int(val), nil
	case int:
		return val, nil
	case int8:
		return int(val), nil
	case int16:
		return int(val), nil
	case int32:
		return int(val), nil
	case int64:
		return int(val), nil
	case uint:
		if val > math.MaxInt {
			return 0, fmt.Errorf("%w: uint value %d", ErrIntOverflow, val)
		}

		return int(val), nil
	case uint8:
		return int(val), nil
	case uint16:
		return int(val), nil
	case uint32:
		return int(val), nil
	case uint64:
		if val > math.MaxInt {
			return 0, fmt.Errorf("%w: uint64 value %d", ErrIntOverflow, val)
		}

		return int(val), nil
	case float32:
		return int(val), nil
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("%w: %T", ErrInvalidIntType, v)
	}
}
