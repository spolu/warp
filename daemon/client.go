package daemon

import (
	"context"
	"encoding/gob"
	"net"

	"github.com/hashicorp/yamux"
)

// client represents a client connected to the wrp
type client struct {
	key wrp.Key

	conn    net.Conn
	session *yamux.Session

	username string
	mode     wrp.Mode

	stateC net.Conn
	stateW *gob.Encoder

	updateC net.Conn
	udpateR *gob.Decoder

	dataC net.Conn

	ctx    context.Context
	cancel func()
}
