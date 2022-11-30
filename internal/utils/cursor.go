package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/gob"
	"io"
)

var key = [32]uint8{0x8d, 0x4f, 0x95, 0x48, 0x28, 0x5b, 0x83, 0xae, 0x2, 0x1e, 0xbe, 0x9a, 0xb4, 0x37, 0x62, 0x57, 0x70, 0x73, 0x41, 0x96, 0x1a, 0x5, 0x79, 0x2d, 0xd4, 0x51, 0x1, 0xbe, 0xaa, 0xc7, 0x5a, 0xfa}

func RegisterCursorType(val any) {
	gob.Register(val)
}

func MarshalCursor(input any) string {
	var buf bytes.Buffer

	// encode
	err := gob.NewEncoder(&buf).Encode(&input)
	if err != nil {
		return ""
	}

	// encrypt
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return ""
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}

	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return ""
	}

	nonce = gcm.Seal(nonce, nonce, buf.Bytes(), nil)

	// stringify
	return base64.RawURLEncoding.EncodeToString(nonce)
}

func UnmarshalCursor(input string) any {
	// de-stringify
	ciphertext, err := base64.RawURLEncoding.DecodeString(input)
	if err != nil {
		return nil
	}

	// decrypt
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil
	}

	plain, err := gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
	if err != nil {
		return nil
	}

	// decode
	var dst any
	err = gob.NewDecoder(bytes.NewReader(plain)).Decode(&dst)
	if err != nil {
		return nil
	}

	return dst
}
