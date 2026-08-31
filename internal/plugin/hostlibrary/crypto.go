package hostlibrary

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"

	"github.com/gameap/gameap/pkg/plugin/sdk/crypto"
	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
	"golang.org/x/crypto/argon2"
)

const (
	defaultCharset = "abcdedfghijklmnopqrstABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	defaultArgon2Memory      = 19456
	defaultArgon2Time        = 2
	defaultArgon2Parallelism = 1
	defaultArgon2SaltLength  = 16
	defaultArgon2KeyLength   = 32

	maxStringLength = 1024 * 1024
	maxCharsetLen   = 256
	minArgon2Memory = 1024
	maxArgon2Memory = 4194304
)

var (
	errMemoryTooLow     = errors.New("memory must be at least 1024 KB")
	errMemoryTooHigh    = errors.New("memory exceeds maximum of 4194304 KB")
	errTimeTooLow       = errors.New("time must be at least 1")
	errParallelismLow   = errors.New("parallelism must be at least 1")
	errSaltTooShort     = errors.New("salt length must be at least 8 bytes")
	errKeyTooShort      = errors.New("key length must be at least 16 bytes")
	errInvalidPartCount = errors.New("invalid hash format: wrong number of parts")
	errBadAlgorithm     = errors.New("unsupported algorithm")
	errBadVersion       = errors.New("unsupported argon2 version")
)

type CryptoServiceImpl struct{}

func NewCryptoService() *CryptoServiceImpl {
	return &CryptoServiceImpl{}
}

func (s *CryptoServiceImpl) RandomUint64(
	_ context.Context,
	req *crypto.RandomUint64Request,
) (*crypto.RandomUint64Response, error) {
	if req.Max == 0 {
		return &crypto.RandomUint64Response{
			Error: new("max must be greater than 0"),
		}, nil
	}

	n, err := rand.Int(rand.Reader, big.NewInt(0).SetUint64(req.Max))
	if err != nil {
		return &crypto.RandomUint64Response{
			Error: new("failed to generate random number: " + err.Error()),
		}, nil
	}

	return &crypto.RandomUint64Response{
		Value: n.Uint64(),
	}, nil
}

func (s *CryptoServiceImpl) RandomString(
	_ context.Context,
	req *crypto.RandomStringRequest,
) (*crypto.RandomStringResponse, error) {
	length := int(req.Length)
	if length <= 0 {
		return &crypto.RandomStringResponse{
			Error: new("length must be greater than 0"),
		}, nil
	}
	if length > maxStringLength {
		return &crypto.RandomStringResponse{
			Error: new(fmt.Sprintf("length exceeds maximum of %d", maxStringLength)),
		}, nil
	}

	charset := defaultCharset
	if req.Charset != nil && *req.Charset != "" {
		charset = *req.Charset
		if len(charset) > maxCharsetLen {
			return &crypto.RandomStringResponse{
				Error: new(fmt.Sprintf("charset length exceeds maximum of %d", maxCharsetLen)),
			}, nil
		}
	}

	result := make([]byte, 0, length)
	m := big.NewInt(int64(len(charset)))

	for range length {
		n, err := rand.Int(rand.Reader, m)
		if err != nil {
			return &crypto.RandomStringResponse{
				Error: new("failed to generate random number: " + err.Error()),
			}, nil
		}
		result = append(result, charset[n.Int64()])
	}

	return &crypto.RandomStringResponse{
		Value: string(result),
	}, nil
}

func (s *CryptoServiceImpl) Argon2Hash(
	_ context.Context,
	req *crypto.Argon2HashRequest,
) (*crypto.Argon2HashResponse, error) {
	if req.Password == "" {
		return &crypto.Argon2HashResponse{
			Error: new("password cannot be empty"),
		}, nil
	}

	params := s.getArgon2Params(req.Params)

	if err := s.validateArgon2Params(params); err != nil {
		return &crypto.Argon2HashResponse{
			Error: new(err.Error()),
		}, nil
	}

	salt := make([]byte, params.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return &crypto.Argon2HashResponse{
			Error: new("failed to generate salt: " + err.Error()),
		}, nil
	}

	hash := argon2.IDKey(
		[]byte(req.Password),
		salt,
		params.time,
		params.memory,
		params.parallelism,
		params.keyLength,
	)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.memory,
		params.time,
		params.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)

	return &crypto.Argon2HashResponse{
		Hash: encoded,
	}, nil
}

func (s *CryptoServiceImpl) Argon2Verify(
	_ context.Context,
	req *crypto.Argon2VerifyRequest,
) (*crypto.Argon2VerifyResponse, error) {
	if req.Password == "" {
		return &crypto.Argon2VerifyResponse{
			Match: false,
			Error: new("password cannot be empty"),
		}, nil
	}
	if req.Hash == "" {
		return &crypto.Argon2VerifyResponse{
			Match: false,
			Error: new("hash cannot be empty"),
		}, nil
	}

	params, salt, hash, err := s.parseArgon2Hash(req.Hash)
	if err != nil {
		return &crypto.Argon2VerifyResponse{
			Match: false,
			Error: new("invalid hash format: " + err.Error()),
		}, nil
	}

	hashLen := min(len(hash), 1024)

	computedHash := argon2.IDKey(
		[]byte(req.Password),
		salt,
		params.time,
		params.memory,
		params.parallelism,
		uint32(hashLen),
	)

	match := subtle.ConstantTimeCompare(hash, computedHash) == 1

	return &crypto.Argon2VerifyResponse{
		Match: match,
	}, nil
}

type argon2Params struct {
	memory      uint32
	time        uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

func (s *CryptoServiceImpl) getArgon2Params(p *crypto.Argon2Params) argon2Params {
	params := argon2Params{
		memory:      defaultArgon2Memory,
		time:        defaultArgon2Time,
		parallelism: defaultArgon2Parallelism,
		saltLength:  defaultArgon2SaltLength,
		keyLength:   defaultArgon2KeyLength,
	}

	if p == nil {
		return params
	}

	if p.Memory > 0 {
		params.memory = p.Memory
	}
	if p.Time > 0 {
		params.time = p.Time
	}
	if p.Parallelism > 0 && p.Parallelism <= 255 {
		params.parallelism = uint8(p.Parallelism)
	}
	if p.SaltLength > 0 {
		params.saltLength = p.SaltLength
	}
	if p.KeyLength > 0 {
		params.keyLength = p.KeyLength
	}

	return params
}

func (s *CryptoServiceImpl) validateArgon2Params(p argon2Params) error {
	if p.memory < minArgon2Memory {
		return errMemoryTooLow
	}
	if p.memory > maxArgon2Memory {
		return errMemoryTooHigh
	}
	if p.time < 1 {
		return errTimeTooLow
	}
	if p.parallelism < 1 {
		return errParallelismLow
	}
	if p.saltLength < 8 {
		return errSaltTooShort
	}
	if p.keyLength < 16 {
		return errKeyTooShort
	}

	return nil
}

func (s *CryptoServiceImpl) parseArgon2Hash(encoded string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return argon2Params{}, nil, nil, errInvalidPartCount
	}

	if parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, errors.WithMessagef(errBadAlgorithm, "got %s", parts[1])
	}

	var version int
	_, err := fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return argon2Params{}, nil, nil, errors.WithMessage(err, "failed to parse version")
	}
	if version != argon2.Version {
		return argon2Params{}, nil, nil, errors.WithMessagef(errBadVersion, "got %d", version)
	}

	var params argon2Params
	var parallelism uint32
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memory, &params.time, &parallelism)
	if err != nil {
		return argon2Params{}, nil, nil, errors.WithMessage(err, "failed to parse params")
	}
	if parallelism <= 255 {
		params.parallelism = uint8(parallelism)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2Params{}, nil, nil, errors.WithMessage(err, "failed to decode salt")
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2Params{}, nil, nil, errors.WithMessage(err, "failed to decode hash")
	}

	return params, salt, hash, nil
}

type CryptoHostLibrary struct {
	impl *CryptoServiceImpl
}

func NewCryptoHostLibrary() *CryptoHostLibrary {
	return &CryptoHostLibrary{
		impl: NewCryptoService(),
	}
}

func (l *CryptoHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return crypto.Instantiate(ctx, r, l.impl)
}
