package cli

import (
	"context"
	"encoding/gob"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/spolu/warp"
	"github.com/spolu/warp/lib/errors"
	"github.com/spolu/warp/lib/out"
)

type Session struct {
	session warp.Session

	warp        string
	sessionType warp.SessionType
	username    string

	conn net.Conn
	mux  *yamux.Session

	stateC  net.Conn
	stateR  *gob.Decoder
	updateC net.Conn
	updateW *gob.Encoder
	errorC  net.Conn
	errorR  *gob.Decoder
	dataC   net.Conn

	state *Warp

	tornDown bool
	cancel   func()

	mutex *sync.Mutex
}

// State returns the session warp state.
func (ss *Session) State() *Warp {
	return ss.state
}

// DataC returns the session data channel.
func (ss *Session) DataC() net.Conn {
	return ss.dataC
}

// TornDown returns the session tornDown value.
func (ss *Session) TornDown() bool {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.tornDown
}

// NewSession sets up a session, opens the associated channels and return a
// Session object.
func NewSession(
	ctx context.Context,
	session warp.Session,
	w string,
	sessionType warp.SessionType,
	username string,
	cancel func(),
	conn net.Conn,
) (*Session, error) {
	mux, err := yamux.Client(conn, nil)
	if err != nil {
		return nil, errors.Trace(
			errors.Newf("Failed to open session to warpd: %v", err),
		)
	}

	ss := &Session{
		session:     session,
		warp:        w,
		sessionType: sessionType,
		username:    username,
		conn:        conn,
		mux:         mux,
		cancel:      cancel,
		mutex:       &sync.Mutex{},
	}

	// Opens state channel stateC.
	ss.stateC, err = mux.Open()
	if err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("State channel open error: %v", err),
		)
	}
	ss.stateR = gob.NewDecoder(ss.stateC)

	// Open update channel updateC.
	ss.updateC, err = mux.Open()
	if err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("Update channel open error: %v", err),
		)
	}
	ss.updateW = gob.NewEncoder(ss.updateC)

	// Send initial SessionHello.
	hello := warp.SessionHello{
		Warp:     ss.warp,
		From:     ss.session,
		Type:     ss.sessionType,
		Username: ss.username,
	}
	if err := ss.updateW.Encode(hello); err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("Send hello error: %v", err),
		)
	}

	// Opens error channel errorC.
	ss.errorC, err = mux.Open()
	if err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("Error channel open error: %v", err),
		)
	}
	ss.errorR = gob.NewDecoder(ss.errorC)

	// Open data channel dataC.
	ss.dataC, err = mux.Open()
	if err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("Data channel open error: %v", err),
		)
	}

	// Setup warp state.
	ss.state = NewWarp(hello)

	return ss, nil
}

// TearDown tears down a session, closing and reclaiming channels.
func (ss *Session) TearDown() {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	if !ss.tornDown {
		ss.tornDown = true
		ss.cancel()
		// Closes stateC, updateC, errorC, dataC, mux and conn.
		ss.mux.Close()
	}
}

// ErrorOut is used to print an error with a slight delay to let the terminal
// be resoted from raw modoe.
func (ss *Session) ErrorOut(
	message string,
	err error,
) {
	go func() {
		// Sleep for 50ms to give time to the terminal to be restored. The
		// program will sleep for 100ms before existing to give us a chance to
		// execute.
		time.Sleep(50 * time.Millisecond)
		out.Errof(
			"\n[Error] %s: %v\n",
			message, err,
		)
	}()
}

// SendHostUpdate is used to safely concurrently sending host updates.
func (ss *Session) SendHostUpdate(
	ctx context.Context,
	update warp.HostUpdate,
) error {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	if err := ss.updateW.Encode(update); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// DecodeError attempts to decode an error from the errorC. This method is not
// thread-safe.
func (ss *Session) DecodeError(
	ctx context.Context,
) (*warp.Error, error) {
	var e warp.Error
	if err := ss.errorR.Decode(&e); err != nil {
		return nil, errors.Trace(err)
	}
	return &e, nil
}

// DecodeState attempts to decode state from the sateC. This method is not
// thread-safe.
func (ss *Session) DecodeState(
	ctx context.Context,
) (*warp.State, error) {
	var st warp.State
	if err := ss.stateR.Decode(&st); err != nil {
		return nil, errors.Trace(err)
	}
	return &st, nil
}
