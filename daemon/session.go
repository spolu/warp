package daemon

import (
	"context"
	"encoding/gob"
	"fmt"
	"net"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/spolu/warp"
	"github.com/spolu/warp/lib/errors"
	"github.com/spolu/warp/lib/logging"
)

// Session represents a client session connected to the warp.
type Session struct {
	session warp.Session

	warp        string
	sessionType warp.SessionType

	username string

	conn net.Conn
	mux  *yamux.Session

	stateC  net.Conn
	stateW  *gob.Encoder
	updateC net.Conn
	updateR *gob.Decoder
	errorC  net.Conn
	errorW  *gob.Encoder
	dataC   net.Conn

	tornDown bool
	ctx      context.Context
	cancel   func()
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
		conn:     conn,
		mux:      mux,
		tornDown: false,
		ctx:      ctx,
		cancel:   cancel,
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

	var hello warp.SessionHello
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

	// Opens error channel errorC.
	ss.errorC, err = mux.Accept()
	if err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("Error channel open error: %v", err),
		)
	}
	ss.errorW = gob.NewEncoder(ss.errorC)

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
	if !ss.tornDown {
		ss.tornDown = true
		ss.cancel()
		go func() {
			// Sleep for 500ms before killing the session to give a chance to
			// the bufffers to flush.
			time.Sleep(500 * time.Millisecond)
			// Closes stateC, updateC, errorC, dataC, mux and conn.
			ss.mux.Close()
		}()
	}
}

// SendError sends an error to the client which should trigger a disconnection
// on its end.
func (ss *Session) SendError(
	ctx context.Context,
	code string,
	message string,
) {
	if ss.tornDown {
		return
	}
	logging.Logf(ctx,
		"Sending session error: session=%s code=%s message=%s",
		ss.ToString(), code, message,
	)
	if err := ss.errorW.Encode(warp.Error{
		Code:    code,
		Message: message,
	}); err != nil {
		logging.Logf(ctx,
			"Error sending session error: session=%s error=%v",
			ss.ToString(), err,
		)
	}
}

// SendInternalError sends an internal error to the client which should trigger
// a disconnection on its end.
func (ss *Session) SendInternalError(
	ctx context.Context,
) {
	ss.SendError(ctx,
		"internal_error",
		fmt.Sprintf(
			"The warp experienced an internal error (session: %s).",
			ss.ToString(),
		),
	)
}
