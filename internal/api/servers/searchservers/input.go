package searchservers

import (
	"net/http"

	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

func readInput(r *http.Request) (string, error) {
	queryReader := api.NewQueryReader(r)

	query, err := queryReader.ReadString("q")
	if err != nil {
		return "", errors.WithMessage(err, "failed to read query parameter 'q'")
	}

	return query, nil
}
