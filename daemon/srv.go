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

	warps map[string]*Warp
	mutex *sync.Mutex
}

// NewSrv constructs a Srv ready to start serving requests.
func NewSrv(
	ctx context.Context,
	address string,
) *Srv {
	return &Srv{
		address: address,
		warps:   map[string]*Warp{},
		mutex:   &sync.Mutex{},
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
				"Error accepting connection: remote=%s error=%v",
				conn.RemoteAddr().String(), err,
			)
			continue
		}
		go func() {
			err := s.handle(ctx, conn)
			if err != nil {
				logging.Logf(ctx,
					"Error handling connection: remote=%s error=%v",
					conn.RemoteAddr().String(), err,
				)
			} else {
				logging.Logf(ctx,
					"Done handling connection: remote=%s",
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
	logging.Logf(ctx,
		"Handling new connection: remote=%s",
		conn.RemoteAddr().String(),
	)

	mux, err := yamux.Server(conn, nil)
	if err != nil {
		return errors.Trace(
			errors.Newf("Session error: %v", err),
		)
	}
	// Closes stateC, updateC, dataC, [hostC,] mux and conn.
	defer mux.Close()

	// Create a new context for this client with its own cancelation function.
	ctx, cancel := context.WithCancel(ctx)

	c := &Client{
		conn:   conn,
		mux:    mux,
		ctx:    ctx,
		cancel: cancel,
	}

	// Opens state channel stateC.
	c.stateC, err = mux.Accept()
	if err != nil {
		return errors.Trace(
			errors.Newf("State channel open error: %v", err),
		)
	}
	c.stateW = gob.NewEncoder(c.stateC)

	// Open update channel updateC.
	c.updateC, err = mux.Accept()
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
	c.session = initial.From
	c.username = initial.Username

	logging.Logf(ctx,
		"[%s] Initial client update received: "+
			"warp=%s hosting=%t username=%s\n",
		c.session.String(), initial.Warp, initial.Hosting, initial.Username,
	)

	// Open data channel dataC.
	c.dataC, err = mux.Accept()
	if err != nil {
		return errors.Trace(
			errors.Newf("Data channel open error: %v", err),
		)
	}

	if initial.Hosting {
		// Initialize the host as read/write.
		err = s.handleHost(ctx, initial.Warp, c)
	} else {
		// Initialize clients as read only.
		err = s.handleClient(ctx, initial.Warp, c)
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// handleHost handles an host connecting, creating the warp if it does not
// exists or erroring accordingly.
func (s *Srv) handleHost(
	ctx context.Context,
	warp string,
	c *Client,
) error {
	// Open host channel host.
	hostC, err := c.mux.Accept()
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
		"[%s] Initial host update received: warp=%s\n",
		c.session.String(), initial.Warp,
	)

	s.mutex.Lock()
	_, ok := s.warps[warp]

	if ok {
		s.mutex.Unlock()
		return errors.Trace(
			errors.Newf("Host error: warp already in use: %s", warp),
		)
	}

	s.warps[warp] = &Warp{
		token:      warp,
		windowSize: initial.WindowSize,
		host: &HostState{
			UserState: UserState{
				token:    c.session.Token,
				username: c.username,
				mode:     wrp.ModeRead | wrp.ModeWrite,
				// Initialize host sessions as empty as the current client is
				// the host session and does not act as "client". Subsequent
				// client session coming from the host would be added to this
				// list.
				sessions: map[string]*Client{},
			},
			session: c,
			hostC:   hostC,
			hostR:   hostR,
		},
		clients: map[string]*UserState{},
		data:    make(chan []byte),
		mutex:   &sync.Mutex{},
	}

	s.mutex.Unlock()

	err = s.warps[warp].handleHost(ctx, c)
	if err != nil {
		return errors.Trace(err)
	}

	// Clean-up warp.
	logging.Logf(ctx,
		"[%s] Cleaning-up warp: warp=%s",
		c.session.String(), warp,
	)
	s.mutex.Lock()
	delete(s.warps, warp)
	s.mutex.Unlock()

	return nil
}

// handleClient handles a client connecting, retrieving the required warp or
// erroring accordingly.
func (s *Srv) handleClient(
	ctx context.Context,
	warp string,
	c *Client,
) error {
	s.mutex.Lock()
	_, ok := s.warps[warp]
	s.mutex.Unlock()

	if !ok {
		return errors.Trace(
			errors.Newf("Client error: unknown warp %s", warp),
		)
	}

	err := s.warps[warp].handleClient(ctx, c)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}
