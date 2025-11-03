package binnapi

import (
	"github.com/et-nik/binngo"
	"github.com/et-nik/binngo/decode"
)

type StatusRequestMessage uint8

const (
	StatusRequestVersion       StatusRequestMessage = 1
	StatusRequestStatusBase    StatusRequestMessage = 2
	StatusRequestStatusDetails StatusRequestMessage = 3
)

func (msg StatusRequestMessage) MarshalBINN() ([]byte, error) {
	return binngo.Marshal([]uint8{uint8(msg)})
}

func (msg *StatusRequestMessage) UnmarshalBINN(bytes []byte) error {
	var v []uint8

	err := decode.Unmarshal(bytes, &v)
	if err != nil {
		return err
	}
	if len(v) < 1 {
		return NewInvalidBINNValueError("status request requires at least 1 field")
	}

	*msg = StatusRequestMessage(v[0])

	return nil
}

// StatusVersionResponseMessage represents a daemon version response.
type StatusVersionResponseMessage struct {
	Version   string
	BuildDate string
}

// UnmarshalBINN deserializes a StatusVersionResponseMessage from BINN format.
func (msg *StatusVersionResponseMessage) UnmarshalBINN(bytes []byte) error {
	var m []any

	err := decode.Unmarshal(bytes, &m)
	if err != nil {
		return err
	}

	if len(m) < 3 {
		return NewInvalidBINNValueError("version response requires at least 3 fields")
	}

	// Validate status code exists
	_, err = convertToCode(m[0])
	if err != nil {
		return NewInvalidBINNValueError("invalid status code")
	}

	version, ok := m[1].(string)
	if !ok {
		return NewInvalidBINNValueError("version must be string")
	}

	buildDate, ok := m[2].(string)
	if !ok {
		return NewInvalidBINNValueError("build date must be string")
	}

	msg.Version = version
	msg.BuildDate = buildDate

	return nil
}

// MarshalBINN serializes the version response to BINN format.
func (msg *StatusVersionResponseMessage) MarshalBINN() ([]byte, error) {
	resp := []any{
		StatusCodeOK,
		msg.Version,
		msg.BuildDate,
	}

	return binngo.Marshal(&resp)
}

// StatusInfoBaseResponseMessage represents basic daemon information response.
type StatusInfoBaseResponseMessage struct {
	Uptime        string
	WorkingTasks  string
	WaitingTasks  string
	OnlineServers string
}

// UnmarshalBINN deserializes an StatusInfoBaseResponseMessage from BINN format.
func (r *StatusInfoBaseResponseMessage) UnmarshalBINN(bytes []byte) error {
	var m []any

	err := decode.Unmarshal(bytes, &m)
	if err != nil {
		return err
	}

	if len(m) < 5 {
		return NewInvalidBINNValueError("info base response requires at least 5 fields")
	}

	// Validate status code exists
	_, err = convertToCode(m[0])
	if err != nil {
		return NewInvalidBINNValueError("invalid status code")
	}

	uptime, ok := m[1].(string)
	if !ok {
		return NewInvalidBINNValueError("uptime must be string")
	}

	workingTasks, ok := m[2].(string)
	if !ok {
		return NewInvalidBINNValueError("working tasks must be string")
	}

	waitingTasks, ok := m[3].(string)
	if !ok {
		return NewInvalidBINNValueError("waiting tasks must be string")
	}

	onlineServers, ok := m[4].(string)
	if !ok {
		return NewInvalidBINNValueError("online servers must be string")
	}

	r.Uptime = uptime
	r.WorkingTasks = workingTasks
	r.WaitingTasks = waitingTasks
	r.OnlineServers = onlineServers

	return nil
}

// MarshalBINN serializes the info base response to BINN format.
func (r *StatusInfoBaseResponseMessage) MarshalBINN() ([]byte, error) {
	resp := []any{
		StatusCodeOK,
		r.Uptime,
		r.WorkingTasks,
		r.WaitingTasks,
		r.OnlineServers,
	}

	return binngo.Marshal(&resp)
}
