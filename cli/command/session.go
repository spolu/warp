package command

import (
	"context"
	"encoding/gob"
	"net"

	"github.com/hashicorp/yamux"
	"github.com/spolu/warp"
	"github.com/spolu/warp/cli"
	"github.com/spolu/warp/lib/errors"
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

	state *cli.Warp

	cancel func()
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
			errors.Newf("Session error: %v", err),
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
	ss.state = cli.NewWarp(hello)

	return ss, nil
}

// TearDown tears down a session, closing and reclaiming channels.
func (ss *Session) TearDown() {
	// Closes stateC, updateC, dataC, mux and conn.
	ss.mux.Close()
	ss.cancel()
}
