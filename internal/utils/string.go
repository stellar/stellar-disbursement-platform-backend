package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	letterBytes = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	NumberBytes = "0123456789"
)

func RandomString(size int, charSetOptions ...string) (string, error) {
	charSet := letterBytes
	if len(charSetOptions) > 0 {
		charSet = ""
		for _, cs := range charSetOptions {
			charSet += cs
		}
	}

	b := make([]byte, size)
	for i := range b {
		randInt, err := rand.Int(rand.Reader, big.NewInt(int64(len(charSet))))
		if err != nil {
			return "", fmt.Errorf("error generating random number in RandomString: %w", err)
		}

		b[i] = charSet[randInt.Int64()]
	}
	return string(b), nil
}

func TruncateString(str string, borderSizeToKeep int) string {
	if len(str) <= 2*borderSizeToKeep {
		return str
	}
	return str[:borderSizeToKeep] + "..." + str[len(str)-borderSizeToKeep:]
}
