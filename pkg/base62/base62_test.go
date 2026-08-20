package base62

import (
	"bytes"
	"crypto/rand"
	mathrand "math/rand"
	"strings"
	"testing"
)

func Test_EncodeDecode(t *testing.T) {
	t.Parallel()

	src := []byte("Hello, 世界！")
	dst := Encode(src)
	got, err := Decode(dst)
	if err != nil {
		t.Fatalf("failed decode, err = %v", err)
	}
	if !bytes.Equal(src, got) {
		t.Fatalf("failed decode, got = %v, want = %v", got, src)
	}

	dstStr := EncodeToString(src)
	got, _ = DecodeString(dstStr)
	if !bytes.Equal(src, got) {
		t.Fatalf("failed decode string, got = %v, want = %v", got, src)
	}
}

func Test_EncodeDecode_Zeros(t *testing.T) {
	t.Parallel()

	for i := range 1000 {
		src := make([]byte, i)
		dst := Encode(src)
		got, err := Decode(dst)
		if err != nil {
			t.Fatalf("failed decode: err = %v", err)
		}
		if !bytes.Equal(src, got) {
			t.Fatalf("failed decode, got = %v, want = %v", got, src)
		}
	}
}

func Test_EncodeDecode_0xFF(t *testing.T) {
	t.Parallel()

	for i := range 1000 {
		src := make([]byte, i)
		for i := range src {
			src[i] = 0xff
		}
		dst := Encode(src)
		got, err := Decode(dst)
		if err != nil {
			t.Fatalf("failed decode: err = %v", err)
		}
		if !bytes.Equal(src, got) {
			t.Fatalf("failed decode, got = %v, want = %v", got, src)
		}
	}
}

func Test_EncodeDecode_RandomBytes(t *testing.T) {
	t.Parallel()

	for range 1000000 {
		src := make([]byte, 32+mathrand.Intn(32))
		_, _ = rand.Read(src)
		dst := Encode(src)
		got, err := Decode(dst)
		if err != nil {
			t.Fatalf("failed decode, err = %v", err)
		}
		if !bytes.Equal(src, got) {
			t.Fatalf("failed decode, got = %v, want = %v", got, src)
		}
	}
}

func Test_EncodeToBuf(t *testing.T) {
	t.Parallel()

	buf := make([]byte, 0, 1000)
	for range 10000 {
		src := make([]byte, 32+mathrand.Intn(100))
		_, _ = rand.Read(src)
		want := Encode(src)

		got1 := EncodeToBuf(make([]byte, 0, 2), src)
		if !bytes.Equal(want, got1) {
			t.Fatal("incorrect result from EncodeToBuf")
		}

		got2 := EncodeToBuf(buf, src)
		if !bytes.Equal(want, got2) {
			t.Fatal("incorrect result from EncodeToBuf")
		}
	}
}

func TestDecodeToBuf(t *testing.T) {
	t.Parallel()

	buf := make([]byte, 0, 1000)
	for range 10000 {
		src := make([]byte, 32+mathrand.Intn(100))
		_, _ = rand.Read(src)
		encoded := Encode(src)

		got1, err := DecodeToBuf(make([]byte, 0, 2), encoded)
		if err != nil {
			t.Fatalf("failed DecodeToBuf, err = %v", err)
		}
		if !bytes.Equal(src, got1) {
			t.Fatalf("incorrect result from DecodeToBuf, encoded = %v", encoded)
		}

		got2, err := DecodeToBuf(buf, encoded)
		if err != nil {
			t.Fatalf("failed DecodeToBuf, err = %v", err)
		}
		if !bytes.Equal(src, got2) {
			t.Fatalf("incorrect result from DecodeToBuf, encoded = %v", encoded)
		}
	}
}

func Test_NewEncoding_panic(t *testing.T) {
	t.Parallel()

	func() {
		encoder := "abcdef"
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("NewEncoding did not panic with encoder %q", encoder)
			}
		}()
		_ = NewEncoding(encoder)
	}()

	func() {
		encoder := []byte(encodeStd)
		encoder[1] = '\n'
		defer func() {
			if r := recover(); r == nil {
				t.Error("NewEncoding did not panic with encoder contains \\n")
			}
		}()
		_ = NewEncoding(string(encoder))
	}()

	func() {
		encoder := []byte(encodeStd)
		encoder[1] = '\r'
		defer func() {
			if r := recover(); r == nil {
				t.Error("NewEncoding did not panic with encoder contains \\r")
			}
		}()
		_ = NewEncoding(string(encoder))
	}()
}

func Test_Decode_CorruptInputError(t *testing.T) {
	t.Parallel()

	src := make([]byte, 256)
	for i := range src {
		src[i] = byte(i)
	}
	_, err := StdEncoding.Decode(src)
	if err == nil || !strings.Contains(err.Error(), "illegal base62 data at input byte") {
		t.Fatal("decoding invalid data did not return CorruptInputError")
	}
}
