package daemon

import (
	"context"
	"encoding/gob"
	"net"
	"sync"

	"github.com/spolu/wrp"
)

// Session represents a pty served from a remote host attached to a token.
type Session struct {
	token string

	windowSize wrp.Size

	host    string
	clients map[string]*Client

	hostC net.Conn
	hostR *gob.Decoder

	data chan []byte

	mutex *sync.Mutex
}

func (s *Session) handleHost(
	ctx context.Context,
	client *Client,
) error {
	return nil
}

func (s *Session) handleClient(
	ctx context.Context,
	client *Client,
) error {
	return nil
}
