package cli

import (
	"context"
	"io"
)

// Multiplex multiplexes src to []dst array and aborts if the context gets
// canceled.
func Multiplex(
	ctx context.Context,
	dst []io.Writer,
	src io.Reader,
) {
	buf := make([]byte, 1024)
	for {
		nr, err := src.Read(buf)
		if nr > 0 {
			cpy := make([]byte, nr)
			copy(cpy, buf)
			for _, d := range dst {
				d.Write(cpy)
			}
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
