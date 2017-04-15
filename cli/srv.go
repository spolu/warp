package cli

import (
	"context"
	"encoding/gob"
	"fmt"
	"net"
	"os"
	"path"
	"sync"

	"github.com/hashicorp/yamux"
	"github.com/spolu/warp"
	"github.com/spolu/warp/lib/errors"
)

type Srv struct {
	host *Session
	path string

	mutex *sync.Mutex
}

// Path returns the unix socket path.
func (s *Srv) Path() string {
	return s.path
}

// NewSrv constructs a Srv ready to start serving local requests.
func NewSrv(
	ctx context.Context,
	host *Session,
) *Srv {
	return &Srv{
		host: host,
		path: path.Join(
			os.TempDir(),
			fmt.Sprintf("_warp_%s.sock", host.State().token),
		),
		mutex: &sync.Mutex{},
	}
}

// Run starts the local server.
func (s *Srv) Run(
	ctx context.Context,
) error {
	ln, err := net.Listen("unix", s.path)

	if err != nil {
		return errors.Trace(err)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go func() {
			s.handle(ctx, conn)
		}()
	}
}

// handle an incoming local connection.
func (s *Srv) handle(
	ctx context.Context,
	conn net.Conn,
) error {
	mux, err := yamux.Client(conn, nil)
	if err != nil {
		return errors.Trace(
			errors.Newf("Session error: %v", err),
		)
	}
	defer mux.Close()

	// Opens command channel commandC
	commandC, err := mux.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("Command channel open error: %v", err),
		)
	}
	commandR := gob.NewDecoder(commandC)
	commandW := gob.NewEncoder(commandC)

	var cmd warp.Command
	if err := commandR.Decode(&cmd); err != nil {
		return errors.Trace(
			errors.Newf("Failed to receive command: %v", err),
		)
	}

	var result warp.CommandResult

	switch cmd.Type {
	case warp.CmdTpState:
		result = s.executeState(ctx, cmd)
	default:
		result.Error.Code = "command_unknown"
		result.Error.Message = fmt.Sprintf(
			"Invalid command: %s",
			cmd.Type,
		)
	}

	// Always return the current state of the warp.
	result.State = s.host.State().State(ctx)

	if err := commandW.Encode(result); err != nil {
		return errors.Trace(
			errors.Newf("Failed to send command result: %v", err),
		)
	}

	return nil
}

// executeState executes the *state* command.
func (s *Srv) executeState(
	ctx context.Context,
	cmd warp.Command,
) warp.CommandResult {
	// NO-OP State is automatically appended to all results.
	return warp.CommandResult{
		Type: warp.CmdTpState,
	}
}
