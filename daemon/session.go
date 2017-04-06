package daemon

import (
	"context"
	"encoding/gob"
	"net"
	"sync"
)

// session represents oa pty served from a remote client attached to an id.
type session struct {
	id string

	windowSize wrp.Size

	host    wrp.Key
	clients map[wrp.Key]client

	hostC net.Conn
	hostR *gob.Decoder

	data chan []byte

	mutex *sync.Mutex
}

func (s *session) handleHost(
	ctx context.Context,
	id string,
	client *client,
) error {
	return nil
}

func (s *session) handleClient(
	ctx context.Context,
	id string,
	client *client,
) error {
	return nil
}
