package postsuspend

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

// maxBodySize leaves room for a reason of MaxSuspendReasonLength characters
// of up to four bytes each, escaped.
const maxBodySize = 16 << 10

var ErrReasonTooLong = api.NewValidationError(
	fmt.Sprintf("reason must not exceed %d characters", domain.MaxSuspendReasonLength),
)

type suspendServerInput struct {
	Reason *string `json:"reason"`
}

// readInput reads the optional body: a suspension without a reason is sent
// without one.
func readInput(r *http.Request) (*suspendServerInput, error) {
	in := &suspendServerInput{}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		return nil, api.WrapHTTPError(errors.Wrap(err, "failed to read request body"), http.StatusBadRequest)
	}

	if len(body) == 0 {
		return in, nil
	}

	if err := json.Unmarshal(body, in); err != nil {
		return nil, api.WrapHTTPError(errors.Wrap(err, "invalid request body"), http.StatusBadRequest)
	}

	if in.Reason != nil {
		in.Reason = new(strings.TrimSpace(*in.Reason))

		if utf8.RuneCountInString(*in.Reason) > domain.MaxSuspendReasonLength {
			return nil, ErrReasonTooLong
		}
	}

	return in, nil
}
