package api //nolint:revive,nolintlint

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

var (
	ErrInvalidValue = NewError(http.StatusBadRequest, "value is invalid")
)

type InputReader struct {
	vars map[string]string
}

func NewInputReader(request *http.Request) *InputReader {
	return &InputReader{vars: mux.Vars(request)}
}

func (r *InputReader) ReadUint(key string) (uint, error) {
	value, err := strconv.Atoi(r.vars[key])
	if err != nil {
		return 0, errors.WithMessage(err, "failed to convert to int")
	}
	if value < 0 {
		return 0, ErrInvalidValue
	}

	return uint(value), nil
}

func (r *InputReader) ReadString(key string) (string, error) {
	return r.vars[key], nil
}

func (r *InputReader) ReadList(_ string) ([]string, error) {
	return []string{}, nil
}

type QueryReader struct {
	query map[string][]string
}

func NewQueryReader(request *http.Request) *QueryReader {
	return &QueryReader{query: request.URL.Query()}
}

func (r *QueryReader) ReadString(key string) (string, error) {
	res := r.query[key]

	if len(res) == 0 {
		return "", nil
	}

	return res[0], nil
}

func (r *QueryReader) ReadList(key string) ([]string, error) {
	res := r.query[key]

	if len(res) == 0 {
		res = r.query[key+"[]"]
	}

	result := make([]string, 0, len(res))
	for _, item := range res {
		if strings.Contains(item, ",") {
			parts := strings.Split(item, ",")
			result = append(result, parts...)
		} else {
			result = append(result, item)
		}
	}

	return result, nil
}

func (r *QueryReader) ReadIntList(key string) ([]int, error) {
	list, err := r.ReadList(key)
	if err != nil {
		return nil, err
	}

	res := make([]int, 0, len(list))
	for _, item := range list {
		if item == "" {
			continue
		}
		value, err := strconv.Atoi(item)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to convert to int")
		}
		res = append(res, value)
	}

	return res, nil
}

func (r *QueryReader) ReadUintList(key string) ([]uint, error) {
	list, err := r.ReadList(key)
	if err != nil {
		return nil, err
	}

	res := make([]uint, 0, len(list))
	for _, item := range list {
		if item == "" {
			continue
		}
		value, err := strconv.Atoi(item)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to convert to int")
		}
		if value < 0 {
			return nil, ErrInvalidValue
		}
		res = append(res, uint(value))
	}

	return res, nil
}

// FormReader reads values submitted in a request body form. It is meant for
// multipart uploads that carry flags next to the file, so it must be created
// after the form has been parsed (ParseForm / ParseMultipartForm) — before
// that the request carries no values and every read returns the zero value.
type FormReader struct {
	values map[string][]string
}

func NewFormReader(request *http.Request) *FormReader {
	values := request.Form
	if request.MultipartForm != nil {
		values = request.MultipartForm.Value
	}

	return &FormReader{values: values}
}

func (r *FormReader) ReadString(key string) (string, error) {
	res := r.values[key]

	if len(res) == 0 {
		return "", nil
	}

	return res[0], nil
}

// ReadBool treats a missing or empty value as false so an optional flag can be
// omitted entirely. Anything strconv.ParseBool does not understand is an
// explicit error rather than a silent false.
func (r *FormReader) ReadBool(key string) (bool, error) {
	value, err := r.ReadString(key)
	if err != nil {
		return false, err
	}

	if value == "" {
		return false, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, ErrInvalidValue
	}

	return parsed, nil
}
