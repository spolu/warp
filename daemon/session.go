package daemon

import (
	"context"
	"encoding/gob"
	"net"

	"github.com/hashicorp/yamux"
	"github.com/spolu/wrp"
	"github.com/spolu/wrp/lib/logging"
)

// Session represents a client session connected to the warp.
type Session struct {
	session wrp.Session

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

func (ss *Session) SendError(
	ctx context.Context,
	code string,
	message string,
) {
	// TODO actually send error
	logging.Logf(ctx,
		"[%s] Session error: code=%s message=%s",
		ss.session.String(), code, message,
	)
}
