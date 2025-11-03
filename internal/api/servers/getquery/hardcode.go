package getquery

import "github.com/gameap/gameap/pkg/quercon/query"

var queryProtocolsByEngine = map[string]query.Protocol{
	"source":     query.ProtocolSource,
	"goldsource": query.ProtocolSource,
	"goldsrc":    query.ProtocolSource,
	"minecraft":  query.ProtocolMinecraft,
}

func getQueryProtocolByEngine(engine string) (query.Protocol, bool) {
	protocol, ok := queryProtocolsByEngine[engine]

	return protocol, ok
}
