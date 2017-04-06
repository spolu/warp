package daemon

import (
	"context"
	"encoding/gob"
	"net"

	"github.com/hashicorp/yamux"
	"github.com/spolu/wrp"
)

// Client represents a client connected to the wrp
type Client struct {
	user wrp.User

	conn net.Conn
	mux  *yamux.Session

	username string
	mode     wrp.Mode

	stateC net.Conn
	stateW *gob.Encoder

	updateC net.Conn
	updateR *gob.Decoder

	dataC net.Conn

	ctx    context.Context
	cancel func()
}
