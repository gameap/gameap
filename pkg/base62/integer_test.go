package base62

import (
	"bytes"
	"math"
	"testing"
)

func Test_FormatUint_ParseUint(t *testing.T) {
	t.Parallel()

	x := uint64(math.MaxUint64)
	dst := FormatUint(x)

	got, err := ParseUint(dst)
	if err != nil {
		t.Fatalf("failed parse uint, err = %v", err)
	}
	if got != x {
		t.Fatalf("failed parse uint, got = %v, want = %v", got, x)
	}
}

func Test_FormatUint_ParseInt(t *testing.T) {
	t.Parallel()

	x := int64(math.MaxInt64)
	dst := FormatInt(x)

	got, err := ParseInt(dst)
	if err != nil {
		t.Fatalf("failed parse int, err = %v", err)
	}
	if got != x {
		t.Fatalf("failed parse int, got = %v, want = %v", got, x)
	}
}

func Test_AppendInt_AppendUint(t *testing.T) {
	t.Parallel()

	x := int64(math.MaxInt64)

	dst1 := AppendInt(nil, x)
	dst2 := AppendUint(nil, uint64(x))

	if !bytes.Equal(dst1, dst2) {
		t.Fatal("integer append result not equal")
	}
}

func Test_FormatInt_Zero(t *testing.T) {
	t.Parallel()

	dst := FormatInt(0)
	if len(dst) != 1 || dst[0] != encodeStd[0] {
		t.Fatalf("failed format zero int, got = %v", string(dst))
	}

	got, err := ParseInt(dst)
	if err != nil {
		t.Fatalf("failed parse zero int, err = %v", err)
	}
	if got != 0 {
		t.Fatalf("failed parse zero int, got = %v, want = 0", got)
	}
}
