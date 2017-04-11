package daemon

import (
	"context"
	"encoding/gob"
	"fmt"
	"net"

	"github.com/hashicorp/yamux"
	"github.com/spolu/wrp"
	"github.com/spolu/wrp/lib/errors"
	"github.com/spolu/wrp/lib/logging"
)

// Session represents a client session connected to the warp.
type Session struct {
	session wrp.Session

	warp        string
	sessionType wrp.SessionType

	username string

	conn net.Conn
	mux  *yamux.Session

	stateC net.Conn
	stateW *gob.Encoder

	updateC net.Conn
	updateR *gob.Decoder

	dataC net.Conn

	ctx    context.Context
	cancel func()
}

// NewSession sets up a session, opens the associated channels and return a
// Session object.
func NewSession(
	ctx context.Context,
	cancel func(),
	conn net.Conn,
) (*Session, error) {
	mux, err := yamux.Server(conn, nil)
	if err != nil {
		return nil, errors.Trace(
			errors.Newf("Mux error: %v", err),
		)
	}

	ss := &Session{
		conn:   conn,
		mux:    mux,
		ctx:    ctx,
		cancel: cancel,
	}

	// Opens state channel stateC.
	ss.stateC, err = mux.Accept()
	if err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("State channel open error: %v", err),
		)
	}
	ss.stateW = gob.NewEncoder(ss.stateC)

	// Open update channel updateC.
	ss.updateC, err = mux.Accept()
	if err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("Update channel open error: %v", err),
		)
	}
	ss.updateR = gob.NewDecoder(ss.updateC)

	var hello wrp.SessionHello
	if err := ss.updateR.Decode(&hello); err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("Initial client update error: %v", err),
		)
	}
	ss.session = hello.From
	ss.warp = hello.Warp
	ss.sessionType = hello.Type
	ss.username = hello.Username

	logging.Logf(ctx,
		"Session hello received: session=%s type=%s username=%s",
		ss.ToString(), hello.Warp, hello.Type, hello.Username,
	)

	// Open data channel dataC.
	ss.dataC, err = mux.Accept()
	if err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("Data channel open error: %v", err),
		)
	}

	return ss, nil
}

// ToStering returns a string that identifies the session for logging.
func (ss *Session) ToString() string {
	return fmt.Sprintf(
		"%s/%s:%s", ss.warp, ss.session.User, ss.session.Token,
	)
}

// TearDown tears down a session, closing and reclaiming channels.
func (ss *Session) TearDown() {
	// Closes stateC, updateC, dataC, mux and conn.
	ss.mux.Close()
	ss.cancel()
}

// SendError sends an error to the client which should trigger a disconnection
// on its end.
func (ss *Session) SendError(
	ctx context.Context,
	code string,
	message string,
) {
	// TODO actually send error
	logging.Logf(ctx,
		"Session error: session=%s code=%s message=%s",
		ss.ToString(), code, message,
	)
}
