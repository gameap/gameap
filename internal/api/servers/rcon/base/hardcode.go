package base

import (
	"strings"

	"github.com/gameap/gameap/pkg/quercon/rcon"
	"github.com/pkg/errors"
)

func DetermineProtocolByEngine(engine string) (rcon.Protocol, error) {
	switch strings.ToLower(engine) {
	case "goldsource", "goldsrc":
		return rcon.ProtocolGoldSrc, nil
	case "source":
		return rcon.ProtocolSource, nil
	default:
		return "", errors.Errorf("unsupported engine: %s", engine)
	}
}
