package binnapi

import (
	"github.com/et-nik/binngo"
	"github.com/et-nik/binngo/decode"
	"github.com/pkg/errors"
)

type CommandExecRequestMessage struct {
	Command string
	WorkDir string
	Kind    uint8
}

func (msg CommandExecRequestMessage) MarshalBINN() ([]byte, error) {
	req := []any{msg.Kind, msg.Command, msg.WorkDir}

	return binngo.Marshal(&req)
}

func (msg *CommandExecRequestMessage) UnmarshalBINN(bytes []byte) error {
	var v []any

	err := decode.Unmarshal(bytes, &v)
	if err != nil {
		return err
	}
	if len(v) < 3 {
		return NewInvalidBINNValueError("not enough values in binn slice")
	}

	kind, ok := v[0].(uint8)
	if !ok {
		return NewInvalidBINNValueError("kind is not uint8")
	}

	command, ok := v[1].(string)
	if !ok {
		return NewInvalidBINNValueError("command is not a string")
	}

	workDir, ok := v[2].(string)
	if !ok {
		return NewInvalidBINNValueError("workDir is not a string")
	}

	msg.Kind = kind
	msg.Command = command
	msg.WorkDir = workDir

	return nil
}

type CommandExecResponseMessage struct {
	Output   string
	ExitCode int
	Code     StatusCode
}

func (r CommandExecResponseMessage) MarshalBINN() ([]byte, error) {
	resp := []any{r.Code, r.ExitCode, r.Output}

	return binngo.Marshal(&resp)
}

func (r *CommandExecResponseMessage) UnmarshalBINN(bytes []byte) error {
	var v []any

	err := decode.Unmarshal(bytes, &v)
	if err != nil {
		return errors.Wrap(err, "unmarshal binn")
	}

	return r.FillFromSlice(v)
}

func (r *CommandExecResponseMessage) FillFromSlice(v []any) error {
	if len(v) < 3 {
		return NewInvalidBINNValueError("not enough values in binn slice")
	}

	code, err := convertInt(v[0])
	if err != nil {
		return err
	}

	if code < 0 {
		return NewInvalidBINNValueError("code cannot be negative")
	}

	if code > 255 {
		return NewInvalidBINNValueError("code value overflow")
	}

	r.Code = StatusCode(code)

	exitCode, err := convertInt(v[1])
	if err != nil {
		return err
	}

	r.ExitCode = exitCode

	output, ok := v[2].(string)
	if !ok {
		return NewInvalidBINNValueError("invalid output value type")
	}

	r.Output = output

	return nil
}

func convertInt(value any) (int, error) {
	switch v := value.(type) {
	case int8:
		return int(v), nil
	case int16:
		return int(v), nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case int:
		return v, nil
	case uint8:
		return int(v), nil
	case uint16:
		return int(v), nil
	case uint32:
		return int(v), nil
	case uint64:
		if v > uint64(^uint(0)>>1) {
			return 0, NewInvalidBINNValueError("integer value overflow")
		}

		return int(v), nil
	case uint:
		if v > (^uint(0) >> 1) {
			return 0, NewInvalidBINNValueError("integer value overflow")
		}

		return int(v), nil
	default:
		return 0, NewInvalidBINNValueError("invalid integer value type")
	}
}
