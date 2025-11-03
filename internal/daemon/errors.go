package daemon

import (
	"strconv"
	"strings"

	"github.com/gameap/gameap/internal/daemon/binnapi"
)

type ResponseError struct {
	StatusCode binnapi.StatusCode
	Msg        string
}

func NewDaemonResponseError(statusCode binnapi.StatusCode, msg string) error {
	return &ResponseError{
		StatusCode: statusCode,
		Msg:        msg,
	}
}

func (e *ResponseError) Error() string {
	sb := strings.Builder{}
	sb.Grow(64)

	sb.WriteString("daemon error: ")
	sb.WriteString("(")
	sb.WriteString(strconv.Itoa(int(e.StatusCode)))
	sb.WriteString(") ")
	sb.WriteString(e.Msg)

	return sb.String()
}
