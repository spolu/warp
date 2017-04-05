package token

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log"
)

// Tokens

const (
	tokenLength = 16
	a62         = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789__"
)

var tokens = make(tokenFountain, 512)

type tokenFountain chan string

func (f tokenFountain) Write(
	buf []byte,
) (int, error) {
	var token [tokenLength]byte
	var i int
	for _, b := range buf {
		if b != '_' {
			token[i] = b
			i++
		}
		if i == tokenLength {
			f <- string(token[:])
			i = 0
		}
	}

	return len(buf), nil
}

func init() {
	buf := bufio.NewWriterSize(tokens, 1024)
	enc := base64.NewEncoder(base64.NewEncoding(a62), buf)

	go func() {
		_, err := io.Copy(enc, rand.Reader)
		// If rand.Reader ever ends or throws an error, we're going to have a
		// bad time, and there's really not much we can do about it.
		log.Panicln("utils.rand: token creation ran out of entropy", err)
	}()
}

// New generates a random token prefixed by prefix
func New(
	name string,
) string {
	return name + "_" + RandStr()
}

// RandStr generates a random string
func RandStr() string {
	return <-tokens
}
