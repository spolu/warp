package cli

import (
	"context"
	"encoding/gob"
	"fmt"
	"net"
	"os"
	"path"
	"sync"
	"syscall"

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
	// Start by unlinking the unix socket (the open command ensures warp
	// uniqueness).
	syscall.Unlink(s.path)

	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return errors.Trace(err)
	}
	defer ln.Close()

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
	defer conn.Close()

	commandR := gob.NewDecoder(conn)
	commandW := gob.NewEncoder(conn)

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
	case warp.CmdTpAuthorize:
		result = s.executeAuthorize(ctx, cmd)
	case warp.CmdTpRevoke:
		result = s.executeRevoke(ctx, cmd)
	default:
		result.Error.Code = "command_unknown"
		result.Error.Message = fmt.Sprintf(
			"Invalid command %s.", cmd.Type,
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

// executeAuthorize executes the *authorize* command.
func (s *Srv) executeAuthorize(
	ctx context.Context,
	cmd warp.Command,
) warp.CommandResult {
	if len(cmd.Args) != 1 {
		return warp.CommandResult{
			Type: warp.CmdTpAuthorize,
			Error: warp.CommandError{
				Code:    "user_token_required",
				Message: "User token to authorize is required.",
			},
		}
	}

	mode, err := s.host.State().GetMode(cmd.Args[0])
	if err != nil {
		return warp.CommandResult{
			Type: warp.CmdTpAuthorize,
			Error: warp.CommandError{
				Code:    "user_unknown",
				Message: err.Error() + ".",
			},
		}
	}

	err = s.host.State().SetMode(cmd.Args[0], *mode|warp.ModeShellWrite)
	if err != nil {
		return warp.CommandResult{
			Type: warp.CmdTpAuthorize,
			Error: warp.CommandError{
				Code:    "user_unknown",
				Message: err.Error() + ".",
			},
		}
	}

	if err := s.host.SendHostUpdate(ctx, warp.HostUpdate{
		Warp:       s.host.Warp(),
		From:       s.host.Session(),
		WindowSize: s.host.State().WindowSize(),
		Modes:      s.host.State().Modes(),
	}); err != nil {
		return warp.CommandResult{
			Type: warp.CmdTpAuthorize,
			Error: warp.CommandError{
				Code:    "update_failed",
				Message: "Failed to apply update to warp.",
			},
		}
	}

	// NO-OP State is automatically appended to all results.
	return warp.CommandResult{
		Type: warp.CmdTpAuthorize,
	}
}

// executeRevoke executes the *revoke* command.
func (s *Srv) executeRevoke(
	ctx context.Context,
	cmd warp.Command,
) warp.CommandResult {
	for _, user := range cmd.Args {
		mode, err := s.host.State().GetMode(user)
		if err != nil {
			return warp.CommandResult{
				Type: warp.CmdTpRevoke,
				Error: warp.CommandError{
					Code:    "user_unknown",
					Message: err.Error() + ".",
				},
			}
		}

		err = s.host.State().SetMode(user, *mode-*mode&warp.ModeShellWrite)
		if err != nil {
			return warp.CommandResult{
				Type: warp.CmdTpRevoke,
				Error: warp.CommandError{
					Code:    "user_unknown",
					Message: err.Error() + ".",
				},
			}
		}
	}

	if err := s.host.SendHostUpdate(ctx, warp.HostUpdate{
		Warp:       s.host.Warp(),
		From:       s.host.Session(),
		WindowSize: s.host.State().WindowSize(),
		Modes:      s.host.State().Modes(),
	}); err != nil {
		return warp.CommandResult{
			Type: warp.CmdTpRevoke,
			Error: warp.CommandError{
				Code:    "update_failed",
				Message: "Failed to apply update to warp.",
			},
		}
	}

	// NO-OP State is automatically appended to all results.
	return warp.CommandResult{
		Type: warp.CmdTpRevoke,
	}
}
