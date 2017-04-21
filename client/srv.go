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
	warp    string
	session *Session
	path    string
	mutex   *sync.Mutex
}

// Path returns the unix socket path.
func (s *Srv) Path() string {
	return s.path
}

// NewSrv constructs a Srv ready to start serving local requests.
func NewSrv(
	ctx context.Context,
	warp string,
) *Srv {
	return &Srv{
		warp:    warp,
		session: nil,
		path: path.Join(
			os.TempDir(),
			fmt.Sprintf("_warp_%s.sock", warp),
		),
		mutex: &sync.Mutex{},
	}
}

// SetSession sets the session the srv should use. It is set to nil if the warp
// is currently disconnected. The write to the session variable is protected by
// a mutex that is locked when comands are executed (to avoid accessing a niled
// out session).
func (s *Srv) SetSession(
	ctx context.Context,
	session *Session,
) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.session = session
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

	// Always return the current state of the warp if connected or an
	// indication of the disconnection otherwise.
	s.mutex.Lock()
	if s.session != nil {
		result.SessionState = s.session.ProtocolState()
	} else {
		result.SessionState.Warp = s.warp
		result.Disconnected = true
	}
	s.mutex.Unlock()

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
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.session == nil {
		return warp.CommandResult{
			Type: warp.CmdTpAuthorize,
			Error: warp.Error{
				Code:    "disconnected",
				Message: "The warp is currently disconnected.",
			},
		}
	}

	if len(cmd.Args) != 1 {
		return warp.CommandResult{
			Type: warp.CmdTpAuthorize,
			Error: warp.Error{
				Code:    "user_token_required",
				Message: "User token to authorize is required.",
			},
		}
	}

	mode, err := s.session.GetMode(cmd.Args[0])
	if err != nil {
		return warp.CommandResult{
			Type: warp.CmdTpAuthorize,
			Error: warp.Error{
				Code:    "user_unknown",
				Message: err.Error() + ".",
			},
		}
	}

	err = s.session.SetMode(cmd.Args[0], *mode|warp.ModeShellWrite)
	if err != nil {
		return warp.CommandResult{
			Type: warp.CmdTpAuthorize,
			Error: warp.Error{
				Code:    "user_unknown",
				Message: err.Error() + ".",
			},
		}
	}

	if err := s.session.SendHostUpdate(ctx, warp.HostUpdate{
		Warp:       s.session.Warp(),
		From:       s.session.Session(),
		WindowSize: s.session.WindowSize(),
		Modes:      s.session.Modes(),
	}); err != nil {
		return warp.CommandResult{
			Type: warp.CmdTpAuthorize,
			Error: warp.Error{
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
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.session == nil {
		return warp.CommandResult{
			Type: warp.CmdTpRevoke,
			Error: warp.Error{
				Code:    "disconnected",
				Message: "The warp is currently disconnected.",
			},
		}
	}

	for _, user := range cmd.Args {
		mode, err := s.session.GetMode(user)
		if err != nil {
			return warp.CommandResult{
				Type: warp.CmdTpRevoke,
				Error: warp.Error{
					Code:    "user_unknown",
					Message: err.Error() + ".",
				},
			}
		}

		err = s.session.SetMode(user, *mode-*mode&warp.ModeShellWrite)
		if err != nil {
			return warp.CommandResult{
				Type: warp.CmdTpRevoke,
				Error: warp.Error{
					Code:    "user_unknown",
					Message: err.Error() + ".",
				},
			}
		}
	}

	if err := s.session.SendHostUpdate(ctx, warp.HostUpdate{
		Warp:       s.session.Warp(),
		From:       s.session.Session(),
		WindowSize: s.session.WindowSize(),
		Modes:      s.session.Modes(),
	}); err != nil {
		return warp.CommandResult{
			Type: warp.CmdTpRevoke,
			Error: warp.Error{
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
