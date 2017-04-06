package daemon

import (
	"context"
	"encoding/gob"
	"log"
	"net"
	"sync"

	"github.com/hashicorp/yamux"
	"github.com/spolu/wrp"
	"github.com/spolu/wrp/lib/errors"
	"github.com/spolu/wrp/lib/logging"
)

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
			err := s.handle(ctx, conn)
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

// handle an incoming connection.
func (s *Srv) handle(
	ctx context.Context,
	conn net.Conn,
) error {
	session, err := yamux.Server(conn, nil)
	if err != nil {
		return errors.Trace(
			errors.Newf("Session error: %v", err),
		)
	}
	// Closes stateC, updateC, dataC, [hostC,] session and conn.
	defer session.Close()

	c := &client{
		conn:    conn,
		session: session,
	}

	// Opens state channel stateC.
	c.stateC, err = session.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("State channel open error: %v", err),
		)
	}
	c.stateW = gob.NewEncoder(c.stateC)

	// Open update channel updateC.
	c.updateC, err = session.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("Update channel open error: %v", err),
		)
	}
	c.updateR = gob.NewDecoder(c.updateC)

	var initial wrp.ClientUpdate
	if err := updateR.Decode(&initial); err != nil {
		return errors.Trace(
			errors.Newf("Handshake error: %v", err),
		)
	}
	logging.Logf(
		"[%s] Initial received: id=%s key=%s is_host=%t username=%s mode=%d\n",
		conn.RemoteAddr().String(),
		initial.ID, initial.Key, initial.IsHost,
		initial.Username, initial.Mode,
	)

	c.key = initial.Key
	c.username = initial.Username
	c.mode = initial.Mode

	// Open data channel dataC.
	c.dataC, err = session.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("Data channel open error: %v", err),
		)
	}

	if initial.IsHost {
		err = s.handleHost(ctx, initial.ID, client)
	} else {
		err = s.handleClient(ctx, initial.ID, client)
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// handleHost handles an host connecting, creating the session if it does not
// exists or erroring accordingly.
func (s *Srv) handleHost(
	ctx context.Context,
	id string,
	client *client,
) error {
	return nil
}

// handleClient handles a client connecting, retrieving the required session or
// erroring accordingly.
func (s *Srv) handleClient(
	ctx context.Context,
	id string,
	client *client,
) error {
	s.mutex.Lock()
	session, ok := s.sessions[id]
	s.mutex.Unlock()

	if !ok {
		return errors.Trace(
			errors.Newf("Client error: unknown session %s", id),
		)
	}

	err := session.handleClient(ctx, client)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}
