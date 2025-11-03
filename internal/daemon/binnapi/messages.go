package binnapi

import (
	"github.com/et-nik/binngo"
	"github.com/et-nik/binngo/decode"
)

type BaseResponseMessage struct {
	Data any
	Info string
	Code StatusCode
}

func (r BaseResponseMessage) MarshalBINN() ([]byte, error) {
	response := []any{r.Code, r.Info}

	if r.Data != nil {
		response = append(response, r.Data)
	}

	return binngo.Marshal(&response)
}

func (r *BaseResponseMessage) UnmarshalBINN(bytes []byte) error {
	var v []any

	err := decode.Unmarshal(bytes, &v)
	if err != nil {
		return err
	}

	return r.FillFromSlice(v)
}

func (r *BaseResponseMessage) FillFromSlice(v []any) error {
	if len(v) < 2 {
		return ErrUnknownBINNValue
	}

	var code StatusCode

	switch val := v[0].(type) {
	case uint8:
		code = StatusCode(val)
	case uint16:
		if val > 255 {
			return ErrUnknownBINNValue
		}
		code = StatusCode(uint8(val))
	case uint32:
		if val > 255 {
			return ErrUnknownBINNValue
		}
		code = StatusCode(uint8(val))
	default:
		return ErrUnknownBINNValue
	}

	info, ok := v[1].(string)
	if !ok {
		return ErrUnknownBINNValue
	}

	r.Code = code
	r.Info = info

	if len(v) > 2 {
		r.Data = v[2]
	}

	return nil
}

type LoginRequestMessage struct {
	Login    string
	Password string
	Mode     Mode
}

func (lm *LoginRequestMessage) UnmarshalBINN(bytes []byte) error {
	var v []any

	err := decode.Unmarshal(bytes, &v)
	if err != nil {
		return err
	}

	if len(v) < 4 {
		return NewInvalidBINNValueError("not enough values for LoginRequestMessage")
	}

	login, ok := v[1].(string)
	if !ok {
		return NewInvalidBINNValueError("login is not a string")
	}

	password, ok := v[2].(string)
	if !ok {
		return NewInvalidBINNValueError("password is not a string")
	}

	modeVal, ok := v[3].(uint8)
	if !ok {
		return NewInvalidBINNValueError("mode is not a uint8")
	}
	mode := Mode(modeVal)

	lm.Login = login
	lm.Password = password
	lm.Mode = mode

	return nil
}
