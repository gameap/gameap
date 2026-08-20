package certificates

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPEMContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "valid_PEM_with_BEGIN_marker",
			data:     []byte("-----BEGIN CERTIFICATE-----\nMIIB..."),
			expected: true,
		},
		{
			name:     "valid_PEM_private_key",
			data:     []byte("-----BEGIN PRIVATE KEY-----\nMIIE..."),
			expected: true,
		},
		{
			name:     "valid_PEM_RSA_private_key",
			data:     []byte("-----BEGIN RSA PRIVATE KEY-----\nMIIE..."),
			expected: true,
		},
		{
			name:     "starts_with_dash",
			data:     []byte("----- SOME CONTENT -----"),
			expected: true,
		},
		{
			name:     "empty_data",
			data:     []byte{},
			expected: false,
		},
		{
			name:     "nil_data",
			data:     nil,
			expected: false,
		},
		{
			name:     "random_binary_data",
			data:     []byte{0x00, 0x01, 0x02, 0x03},
			expected: false,
		},
		{
			name:     "plain_text",
			data:     []byte("some random text without PEM markers"),
			expected: false,
		},
		{
			name:     "contains_BEGIN_but_not_at_start",
			data:     []byte("prefix -----BEGIN CERTIFICATE-----"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsPEMContent(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDecodePossibleBase64(t *testing.T) {
	t.Parallel()

	samplePEM := "-----BEGIN CERTIFICATE-----\nMIIBkTCB+wIJAKHBfpEgcMFvMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl\nc3RDQTAeFw0yMzAxMDEwMDAwMDBaFw0yNDAxMDEwMDAwMDBaMBExDzANBgNVBAMM\nBnRlc3RDQTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQC5r4pzLyD6YF+SBP4OUMGO\nS2N7ewTNOl6EI7M8W5P2W7U8f5UUhFBli7P9LRlhrmS8N8JG3WqWmJJVZ7E1HQRX\nAgMBAAGjUzBRMB0GA1UdDgQWBBTYKaHXreWqZnTYT0q4r8wmqWYwMTAfBgNVHSME\nGDAWgBTYKaHXreWqZnTYT0q4r8wmqWYwMTAPBgNVHRMBAf8EBTADAQH/MA0GCSqG\nSIb3DQEBCwUAA0EA\n-----END CERTIFICATE-----"

	tests := []struct {
		name     string
		input    string
		expected []byte
	}{
		{
			name:     "plain_PEM_content",
			input:    samplePEM,
			expected: []byte(samplePEM),
		},
		{
			name:     "base64_encoded_PEM",
			input:    base64.StdEncoding.EncodeToString([]byte(samplePEM)),
			expected: []byte(samplePEM),
		},
		{
			name:     "plain_text_not_base64",
			input:    "hello world",
			expected: []byte("hello world"),
		},
		{
			name:     "empty_string",
			input:    "",
			expected: []byte(""),
		},
		{
			name:     "invalid_base64_returns_original",
			input:    "not valid base64!!!",
			expected: []byte("not valid base64!!!"),
		},
		{
			name:     "base64_encoded_non_PEM_returns_original",
			input:    base64.StdEncoding.EncodeToString([]byte("not PEM content")),
			expected: []byte(base64.StdEncoding.EncodeToString([]byte("not PEM content"))),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := DecodePossibleBase64(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDecodePossibleBase64_RealCertificate(t *testing.T) {
	t.Parallel()

	certPEM := `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHBfpEgcMFvMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
c3RDQTAeFw0yMzAxMDEwMDAwMDBaFw0yNDAxMDEwMDAwMDBaMBExDzANBgNVBAMM
BnRlc3RDQTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQC5r4pzLyD6YF+SBP4OUMGO
S2N7ewTNOl6EI7M8W5P2W7U8f5UUhFBli7P9LRlhrmS8N8JG3WqWmJJVZ7E1HQRX
AgMBAAGjUzBRMB0GA1UdDgQWBBTYKaHXreWqZnTYT0q4r8wmqWYwMTAfBgNVHSME
GDAWgBTYKaHXreWqZnTYT0q4r8wmqWYwMTAPBgNVHRMBAf8EBTADAQH/MA0GCSqG
SIb3DQEBCwUAA0EA
-----END CERTIFICATE-----`

	base64Cert := base64.StdEncoding.EncodeToString([]byte(certPEM))

	result := DecodePossibleBase64(base64Cert)

	assert.Equal(t, []byte(certPEM), result)
	assert.Contains(t, string(result), "-----BEGIN CERTIFICATE-----")
	assert.Contains(t, string(result), "-----END CERTIFICATE-----")
}

func TestDecodePossibleBase64_PrivateKey(t *testing.T) {
	t.Parallel()

	keyPEM := `-----BEGIN PRIVATE KEY-----
MIIBVQIBADANBgkqhkiG9w0BAQEFAASCAT8wggE7AgEAAkEAua+Kcy8g+mBfkgT+
DlDBjktje3sEzTpehCOzPFuT9lu1PH+VFIRQZYuz/S0ZYa5kvDfCRt1qlpiSVWex
NR0EVwIDAQABAkA7eMk0lDdJR0gm8CvNwP5s5g3KvXKfL8Q3T9Qk8WXq5P1g5X0F
-----END PRIVATE KEY-----`

	base64Key := base64.StdEncoding.EncodeToString([]byte(keyPEM))

	result := DecodePossibleBase64(base64Key)

	assert.Equal(t, []byte(keyPEM), result)
	assert.Contains(t, string(result), "-----BEGIN PRIVATE KEY-----")
}
