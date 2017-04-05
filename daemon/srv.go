package daemon

import (
	"context"
	"encoding/gob"
	"log"
	"net"
	"sync"

	"github.com/hashicorp/yamux"
	"github.com/spolu/wrp"
	"github.com/spolu/wrp/lib/logging"
)

// client represents a client connected to the wrp
type client struct {
	key wrp.Key

	conn    net.Conn
	session *yamux.Session

	username string
	mode     wrp.Mode

	stateC net.Conn
	stateW *gob.Encoder

	updateC net.Conn
	udpateR *gob.Decoder

	dataC net.Conn

	ctx    context.Context
	cancel func()
}

// session represents oa pty served from a remote client attached to an id.
type session struct {
	id string

	windowSize wrp.Size

	host    wrp.Key
	clients map[wrp.Key]client

	hostC net.Conn
	hostR *gob.Decoder

	mutex *sync.Mutex
	data  chan []byte
}

// Srv represents a running wrpd server.
type Srv struct {
	address string

	sessions map[string]*session
	mutex    *sync.Mutex
}

// NewSrv constructs a Srv ready to start serving requests.
func NewSrv(
	ctx context.Context,
	address string,
) *Srv {
	return &Srv{
		address:  address,
		sessions: map[string]*session{},
		mutex:    &sync.Mutex{},
	}
}

// Run starts the server.
func (s *Srv) Run(
	ctx context.Context,
) error {

	ln, err := net.Listen("tcp", s.address)
	if err != nil {
		log.Fatal(err)
	}
	logging.Logf(ctx, "Listening: address=%s", s.address)

	for {
		conn, err := ln.Accept()
		if err != nil {
			logging.Logf(ctx,
				"[%s] Error accepting connection: error=%v",
				conn.RemoteAddr().String(), err,
			)
			continue
		}
		go func() {
			err := s.Handle(ctx, conn)
			if err != nil {
				logging.Logf(ctx,
					"[%s] Error handling connection: error=%v",
					conn.RemoteAddr().String(), err,
				)
			} else {
				logging.Logf(ctx,
					"[%s] Done handling connection",
					conn.RemoteAddr().String(),
				)
			}
		}()
	}
}

// Handle an incoming connection.
func (s *Srv) Handle(
	ctx context.Context,
	conn net.Conn,
) error {
	return nil
}
