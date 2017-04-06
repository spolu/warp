package daemon

import (
	"context"
	"encoding/gob"
	"net"

	"github.com/hashicorp/yamux"
	"github.com/spolu/wrp"
	"github.com/spolu/wrp/lib/logging"
)

// Client represents a client connected to the wrp
type Client struct {
	user wrp.User

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

func (c *Client) SendError(
	ctx context.Context,
	code string,
	message string,
) {
	// TODO actually send error
	logging.Logf(ctx,
		"[%s] Client error: code=%s message=%s",
		c.conn.RemoteAddr().String(), code, message,
	)
}
