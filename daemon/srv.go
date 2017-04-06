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

	sessions map[string]*Session
	mutex    *sync.Mutex
}

// NewSrv constructs a Srv ready to start serving requests.
func NewSrv(
	ctx context.Context,
	address string,
) *Srv {
	return &Srv{
		address:  address,
		sessions: map[string]*Session{},
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
	mux, err := yamux.Server(conn, nil)
	if err != nil {
		return errors.Trace(
			errors.Newf("Session error: %v", err),
		)
	}
	// Closes stateC, updateC, dataC, [hostC,] mux and conn.
	defer mux.Close()

	c := &Client{
		conn: conn,
		mux:  mux,
	}

	// Opens state channel stateC.
	c.stateC, err = mux.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("State channel open error: %v", err),
		)
	}
	c.stateW = gob.NewEncoder(c.stateC)

	// Open update channel updateC.
	c.updateC, err = mux.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("Update channel open error: %v", err),
		)
	}
	c.updateR = gob.NewDecoder(c.updateC)

	var initial wrp.ClientUpdate
	if err := c.updateR.Decode(&initial); err != nil {
		return errors.Trace(
			errors.Newf("Initial client update error: %v", err),
		)
	}
	logging.Logf(ctx,
		"[%s] Initial client update received: "+
			"session=%s user=%s hosting=%t username=%s\n",
		conn.RemoteAddr().String(),
		initial.Session, initial.From.Token,
		initial.Hosting, initial.Username,
	)

	c.user = initial.From
	c.username = initial.Username

	// Open data channel dataC.
	c.dataC, err = mux.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("Data channel open error: %v", err),
		)
	}

	if initial.Hosting {
		// Initialize the host as read/write.
		c.mode = wrp.ModeRead | wrp.ModeWrite
		err = s.handleHost(ctx, initial.Session, c)
	} else {
		// Initialize clients as read only.
		c.mode = wrp.ModeRead
		err = s.handleClient(ctx, initial.Session, c)
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
	session string,
	c *Client,
) error {
	// Open host channel host.
	hostC, err := c.mux.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("Host channel open error: %v", err),
		)
	}
	hostR := gob.NewDecoder(hostC)

	var initial wrp.HostUpdate
	if err := hostR.Decode(&initial); err != nil {
		return errors.Trace(
			errors.Newf("Initial host update error: %v", err),
		)
	}
	logging.Logf(ctx,
		"[%s] Initial host update received: "+
			"session=%s user=%s\n",
		c.conn.RemoteAddr().String(),
		initial.Session, initial.From.Token,
	)

	// TODO create session
	s.mutex.Lock()
	_, ok := s.sessions[session]

	if ok {
		s.mutex.Unlock()
		return errors.Trace(
			errors.Newf("Host error: session already in use: %s", session),
		)
	}

	s.sessions[session] = &Session{
		token:      session,
		windowSize: initial.WindowSize,
		host:       c.user.Token,
		clients:    map[string]*Client{},
		hostC:      hostC,
		hostR:      hostR,
		data:       make(chan []byte),
		mutex:      &sync.Mutex{},
	}

	s.mutex.Unlock()

	err = s.sessions[session].handleHost(ctx, c)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// handleClient handles a client connecting, retrieving the required session or
// erroring accordingly.
func (s *Srv) handleClient(
	ctx context.Context,
	session string,
	c *Client,
) error {
	s.mutex.Lock()
	_, ok := s.sessions[session]
	s.mutex.Unlock()

	if !ok {
		return errors.Trace(
			errors.Newf("Client error: unknown session %s", session),
		)
	}

	err := s.sessions[session].handleClient(ctx, c)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}
