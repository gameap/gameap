package binnapi

import (
	"bytes"
	"context"
	"io"
	"net"

	"github.com/et-nik/binngo"
	"github.com/et-nik/binngo/decode"
	"github.com/et-nik/binngo/encode"
	"github.com/pkg/errors"
)

type Mode uint

const (
	ModeNoAuth Mode = 0
	ModeAuth   Mode = 1
	ModeCMD    Mode = 2
	ModeFiles  Mode = 3
	ModeStatus Mode = 4
)

type StatusCode uint8

const (
	StatusCodeError           StatusCode = 1
	StatusCodeCriticalError   StatusCode = 2
	StatusCodeUnknownCommand  StatusCode = 3
	StatusCodeOK              StatusCode = 100
	StatusCodeReadyToTransfer StatusCode = 101
)

var DaemonBinnEndBytes = []byte{0xFF, 0xFF, 0xFF, 0xFF}

var (
	ErrUnknownBINNValue = errors.New("unknown binn value")
	ErrInvalidEndBytes  = errors.New("invalid message end bytes")
)

type InvalidBINNValueError string

func NewInvalidBINNValueError(details string) error {
	return InvalidBINNValueError(details)
}

func (e InvalidBINNValueError) Error() string {
	return "invalid BINN value: " + string(e)
}

func Login(
	ctx context.Context, c net.Conn, mode Mode, username string, password string,
) error {
	req := []any{
		ModeAuth,
		username,
		password,
		mode,
	}

	message, err := binngo.Marshal(&req)
	if err != nil {
		return errors.Wrap(err, "failed to marshal login message")
	}

	_, err = c.Write(append(message, DaemonBinnEndBytes...))
	if err != nil {
		return errors.Wrap(err, "failed to write login message")
	}
	var resp BaseResponseMessage

	err = decode.NewDecoder(c).Decode(&resp)
	if err != nil {
		return errors.Wrap(err, "failed to decode login response")
	}

	err = ReadEndBytes(ctx, c)
	if err != nil {
		return errors.Wrap(err, "failed to read end bytes after login response")
	}

	if resp.Code != StatusCodeOK {
		return errors.Errorf("login failed: %v", resp.Info)
	}

	return nil
}

func WriteMessage(writer io.Writer, r encode.Marshaler) error {
	writeBytes, err := binngo.Marshal(&r)
	if err != nil {
		return errors.WithMessage(err, "failed to marshal response")
	}

	_, err = writer.Write(append(writeBytes, DaemonBinnEndBytes...))
	if err != nil {
		return errors.WithMessage(err, "failed to write response")
	}

	return nil
}

func ReadMessage(reader io.Reader, msg decode.Unmarshaler) error {
	err := decode.NewDecoder(reader).Decode(msg)
	if err != nil {
		return errors.WithMessage(err, "failed to decode message")
	}

	err = ReadEndBytes(context.Background(), reader)
	if err != nil {
		return errors.WithMessage(err, "failed to read end bytes after message")
	}

	return nil
}

func ReadMessageToSlice(ctx context.Context, reader io.Reader) ([]any, error) {
	var msgs []any

	err := decode.NewDecoder(reader).Decode(&msgs)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to decode message to slice")
	}

	err = ReadEndBytes(ctx, reader)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read end bytes after message")
	}

	return msgs, nil
}

func ReadEndBytes(ctx context.Context, reader io.Reader) error {
	type readResult struct {
		n   int
		err error
	}

	endBytes := make([]byte, 4)
	resultChan := make(chan readResult, 1)

	go func() {
		n, err := reader.Read(endBytes)
		resultChan <- readResult{n: n, err: err}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-resultChan:
		if errors.Is(result.err, io.EOF) {
			return nil
		}
		if result.err != nil {
			return result.err
		}

		if result.n != len(DaemonBinnEndBytes) {
			return ErrInvalidEndBytes
		}

		if !bytes.Equal(endBytes, DaemonBinnEndBytes) {
			return ErrInvalidEndBytes
		}

		return nil
	}
}
