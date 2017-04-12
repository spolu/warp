package plex

import (
	"context"
	"io"
)

// Run pipes src to a funtion and aborts if the context gets canceled.
func Run(
	ctx context.Context,
	dst func([]byte),
	src io.Reader,
) {
	buf := make([]byte, 1024)
	for {
		nr, err := src.Read(buf)
		if nr > 0 {
			cpy := make([]byte, nr)
			copy(cpy, buf)
			dst(cpy)
		}
		if err != nil {
			break
		}
		select {
		case <-ctx.Done():
			break
		default:
		}
	}
}
