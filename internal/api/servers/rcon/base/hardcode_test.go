package base

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetermineProtocol(t *testing.T) {
	tests := []struct {
		name     string
		engine   string
		want     string
		wantErr  bool
		errorMsg string
	}{
		{
			name:    "source_engine",
			engine:  "source",
			want:    "source",
			wantErr: false,
		},
		{
			name:    "goldsource_engine",
			engine:  "goldsource",
			want:    "goldsource",
			wantErr: false,
		},
		{
			name:     "unsupported_engine",
			engine:   "unreal",
			wantErr:  true,
			errorMsg: "unsupported engine",
		},
		{
			name:     "empty_engine",
			engine:   "",
			wantErr:  true,
			errorMsg: "unsupported engine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protocol, err := DetermineProtocolByEngine(tt.engine)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, string(protocol))
			}
		})
	}
}
