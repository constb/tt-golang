package utils

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/bwmarrin/snowflake"
	"github.com/stretchr/testify/assert"
)

type testStruct struct {
	A int
	B string
	C []byte
	D testNested
}

type testNested struct {
	X string
	Y time.Time
	Z snowflake.ID
}

func TestCursorEncoding(t *testing.T) {
	RegisterCursorType(testStruct{})
	t.Parallel()

	someBytes := [256]byte{}
	_, _ = io.ReadFull(rand.Reader, someBytes[:])

	tests := []struct {
		name          string
		arg           any
		decodeAndFail bool
	}{
		{"cursor from int", 1251, false},
		{"cursor from string", "test string", false},
		{"cursor from bytes", []byte{1, 2, 3, 4, 5, 6}, false},
		{"cursor from struct", testStruct{91, "91", []byte{91}, testNested{"nested", time.UnixMilli(1669814594522), 2188749003104256}}, false},

		{"fail from empty", "", true},
		{"fail from random", base64.RawURLEncoding.EncodeToString(someBytes[:]), true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !tt.decodeAndFail {
				str := MarshalCursor(tt.arg)
				dec := UnmarshalCursor(str)
				assert.Truef(t, reflect.DeepEqual(tt.arg, dec), "want: %v, got %v", tt.arg, dec)
			} else {
				dec := UnmarshalCursor(tt.arg.(string))
				assert.Emptyf(t, dec, "want: nil, got: %v", dec)
			}
		})
	}
}
