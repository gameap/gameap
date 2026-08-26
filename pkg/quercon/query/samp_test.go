package query

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuerySAMP_IPv6Rejected(t *testing.T) {
	t.Parallel()
	result, err := querySAMP(context.Background(), "::1", 7777)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "only supports IPv4")
	assert.False(t, result.Online)
}

func TestQuerySAMP_HostnameRejected(t *testing.T) {
	t.Parallel()
	result, err := querySAMP(context.Background(), "localhost", 7777)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires IP address")
	assert.False(t, result.Online)
}

func TestBuildSAMPPacket(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		ip     net.IP
		port   int
		opcode byte
		want   []byte
	}{
		{
			name:   "standard_packet",
			ip:     net.IPv4(192, 168, 1, 100).To4(),
			port:   7777,
			opcode: 'i',
			want: []byte{
				'S', 'A', 'M', 'P',
				192, 168, 1, 100,
				0x61, 0x1E, // 7777 in LE
				'i',
			},
		},
		{
			name:   "different_port",
			ip:     net.IPv4(10, 0, 0, 1).To4(),
			port:   8080,
			opcode: 'd',
			want: []byte{
				'S', 'A', 'M', 'P',
				10, 0, 0, 1,
				0x90, 0x1F, // 8080 in LE
				'd',
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			packet := buildSAMPPacket(tt.ip, tt.port, tt.opcode)
			assert.Equal(t, tt.want, packet)
		})
	}
}

func TestParseSAMPResponse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		response       []byte
		expectedHeader []byte
		wantResult     *Result
		wantError      string
	}{
		{
			name:           "valid_response",
			response:       buildSAMPInfoResponse("192.168.1.1", 7777, "My SA-MP Server", "Freeroam", "English"),
			expectedHeader: buildSAMPPacket(net.IPv4(192, 168, 1, 1).To4(), 7777, 'i'),
			wantResult: &Result{
				Name:          "My SA-MP Server",
				Map:           "Freeroam",
				PlayersNum:    10,
				MaxPlayersNum: 100,
			},
		},
		{
			name:           "empty_hostname",
			response:       buildSAMPInfoResponse("10.0.0.1", 7777, "", "DM", "RU"),
			expectedHeader: buildSAMPPacket(net.IPv4(10, 0, 0, 1).To4(), 7777, 'i'),
			wantResult: &Result{
				Name:          "",
				Map:           "DM",
				PlayersNum:    10,
				MaxPlayersNum: 100,
			},
		},
		{
			name:           "response_too_short",
			response:       []byte{0x00, 0x01, 0x02},
			expectedHeader: []byte{0x00, 0x01, 0x02},
			wantError:      "response too short",
		},
		{
			name:           "header_mismatch",
			response:       buildSAMPInfoResponse("192.168.1.1", 7777, "Server", "Mode", "EN"),
			expectedHeader: buildSAMPPacket(net.IPv4(10, 0, 0, 1).To4(), 8080, 'i'),
			wantError:      "invalid response header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}
			err := parseSAMPResponse(tt.response, tt.expectedHeader, result)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantResult.Name, result.Name)
			assert.Equal(t, tt.wantResult.Map, result.Map)
			assert.Equal(t, tt.wantResult.PlayersNum, result.PlayersNum)
			assert.Equal(t, tt.wantResult.MaxPlayersNum, result.MaxPlayersNum)
		})
	}
}

func TestReadSAMPString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     []byte
		want      string
		wantError string
	}{
		{
			name:  "normal_string",
			input: makeSAMPString("Hello World"),
			want:  "Hello World",
		},
		{
			name:  "empty_string",
			input: makeSAMPString(""),
			want:  "",
		},
		{
			name:  "unicode_string",
			input: makeSAMPString("Привет"),
			want:  "Привет",
		},
		{
			name:      "truncated_data",
			input:     []byte{0x10, 0x00, 0x00, 0x00, 'H', 'i'},
			wantError: "short read",
		},
		{
			name:      "length_too_large",
			input:     []byte{0xFF, 0xFF, 0xFF, 0xFF},
			wantError: "string length too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reader := bytes.NewReader(tt.input)
			result, err := readSAMPString(reader)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func buildSAMPInfoResponse(ip string, port int, hostname, gametype, language string) []byte {
	var buf bytes.Buffer

	parsedIP := net.ParseIP(ip).To4()
	buf.WriteString("SAMP")
	buf.Write(parsedIP)

	portBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(portBytes, uint16(port))
	buf.Write(portBytes)

	buf.WriteByte('i')

	buf.WriteByte(0) // password

	playersBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(playersBytes, 10) // numplayers
	buf.Write(playersBytes)

	maxPlayersBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(maxPlayersBytes, 100) // maxplayers
	buf.Write(maxPlayersBytes)

	buf.Write(makeSAMPString(hostname))
	buf.Write(makeSAMPString(gametype))
	buf.Write(makeSAMPString(language))

	return buf.Bytes()
}

func makeSAMPString(s string) []byte {
	lenBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBytes, uint32(len(s)))

	result := make([]byte, 0, 4+len(s))
	result = append(result, lenBytes...)
	result = append(result, []byte(s)...)

	return result
}
