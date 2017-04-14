package daemon

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/spolu/warp"
	"github.com/spolu/warp/lib/errors"
	"github.com/spolu/warp/lib/logging"
)

// Srv represents a running warpd server.
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
		return errors.Trace(err)
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

	// Create a new context for this client with its own cancelation function.
	ctx, cancel := context.WithCancel(ctx)

	ss, err := NewSession(ctx, cancel, conn)
	if err != nil {
		return errors.Trace(err)
	}
	// Close and reclaims all session related state.
	defer ss.TearDown()

	switch ss.sessionType {
	case warp.SsTpHost:
		err = s.handleHost(ctx, ss)
	case warp.SsTpShellClient:
		err = s.handleShellClient(ctx, ss)
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
	ss *Session,
) error {
	var initial warp.HostUpdate
	if err := ss.updateR.Decode(&initial); err != nil {
		ss.SendInternalError(ctx)
		return errors.Trace(
			errors.Newf("Initial host update error: %v", err),
		)
	}
	logging.Logf(ctx,
		"Initial host update received: session=%s\n",
		ss.ToString(),
	)

	s.mutex.Lock()
	_, ok := s.warps[ss.warp]

	if ok {
		s.mutex.Unlock()
		ss.SendError(ctx,
			"warp_in_use",
			fmt.Sprintf(
				"The warp you attempted to open is already in use: %s.",
				ss.warp,
			),
		)
		return errors.Trace(
			errors.Newf("Host error: warp already in use: %s", ss.warp),
		)
	}

	s.warps[ss.warp] = &Warp{
		token:      ss.warp,
		windowSize: initial.WindowSize,
		host:       nil,
		clients:    map[string]*UserState{},
		data:       make(chan []byte),
		mutex:      &sync.Mutex{},
	}

	s.mutex.Unlock()

	s.warps[ss.warp].handleHost(ctx, ss)

	// Clean-up warp.
	logging.Logf(ctx,
		"Cleaning-up warp: session=%s",
		ss.ToString(),
	)
	s.mutex.Lock()
	delete(s.warps, ss.warp)
	s.mutex.Unlock()

	return nil
}

// handleShellClient handles a client connecting, retrieving the required warp or
// erroring accordingly.
func (s *Srv) handleShellClient(
	ctx context.Context,
	ss *Session,
) error {
	s.mutex.Lock()
	_, ok := s.warps[ss.warp]
	s.mutex.Unlock()

	if !ok {
		ss.SendError(ctx,
			"warp_unknown",
			fmt.Sprintf(
				"The warp you attempted to connect does not exist: %s.",
				ss.warp,
			),
		)
		return errors.Trace(
			errors.Newf("Client error: warp unknown %s", ss.warp),
		)
	}

	s.warps[ss.warp].handleShellClient(ctx, ss)

	return nil
}
