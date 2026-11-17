package getquery

import (
	"testing"

	"github.com/gameap/gameap/pkg/quercon/query"
	"github.com/stretchr/testify/assert"
)

func TestQueryProtocolByEngine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		engine string
		want   query.Protocol
		wantOK bool
	}{
		{name: "source_engine", engine: "source", want: query.ProtocolSource, wantOK: true},
		{name: "rust_answers_a2s_on_its_query_port", engine: "rust", want: query.ProtocolSource, wantOK: true},
		{name: "minecraft_engine", engine: "minecraft", want: query.ProtocolMinecraft, wantOK: true},
		{name: "unknown_engine", engine: "valheim", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := QueryProtocolByEngine(tt.engine)

			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
