package daemon

import (
	"context"
	"fmt"
	"sync"

	"github.com/spolu/warp"
	"github.com/spolu/warp/lib/logging"
)

// UserState represents the state of a user along with a list of all his
// sessions.
type UserState struct {
	token    string
	username string
	mode     warp.Mode
	sessions map[string]*Session
}

// User returns a warp.User from the current UserState.
func (u *UserState) User(
	ctx context.Context,
) warp.User {
	return warp.User{
		Token:    u.token,
		Username: u.username,
		Mode:     u.mode,
		Hosting:  false,
	}
}

// HostState represents the state of the host, in particular the host session,
// along with its UserState.
type HostState struct {
	UserState
	session *Session
}

// User returns a warp.User from the current HostState.
func (h *HostState) User(
	ctx context.Context,
) warp.User {
	return warp.User{
		Token:    h.UserState.token,
		Username: h.UserState.username,
		Mode:     h.UserState.mode,
		Hosting:  true,
	}
}

// Warp represents a pty served from a remote host attached to a token.
type Warp struct {
	token string

	windowSize warp.Size

	host         *HostState
	shellClients map[string]*UserState

	data chan []byte

	mutex *sync.Mutex
}

// State computes a warp.State from the current session. It acquires the session
// lock.
func (w *Warp) State(
	ctx context.Context,
) warp.State {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	state := warp.State{
		Warp:       w.token,
		WindowSize: w.windowSize,
		Users:      map[string]warp.User{},
	}

	state.Users[w.host.session.session.User] = w.host.User(ctx)

	for token, user := range w.shellClients {
		state.Users[token] = user.User(ctx)
	}

	return state
}

// Sessions return all connected sessions that are not the host session.
func (w *Warp) Sessions(
	ctx context.Context,
) []*Session {
	sessions := []*Session{}
	w.mutex.Lock()
	for _, user := range w.shellClients {
		for _, c := range user.sessions {
			sessions = append(sessions, c)
		}
	}
	// The host user's shell client sessions, if any.
	for _, c := range w.host.UserState.sessions {
		sessions = append(sessions, c)
	}
	w.mutex.Unlock()
	return sessions
}

// updateShellClients updates all shell clients with the current warp state.
func (w *Warp) updateShellClients(
	ctx context.Context,
) {
	st := w.State(ctx)
	sessions := w.Sessions(ctx)
	for _, ss := range sessions {
		logging.Logf(ctx,
			"Sending (client) state: session=%s cols=%d rows=%d",
			ss.ToString(), st.WindowSize.Rows, st.WindowSize.Cols,
		)

		ss.stateW.Encode(st)
	}
}

// updateHost updates the host with the current warp state.
func (w *Warp) updateHost(
	ctx context.Context,
) {
	st := w.State(ctx)

	logging.Logf(ctx,
		"Sending (host) state: session=%s cols=%d rows=%d",
		w.host.session.ToString(), st.WindowSize.Rows, st.WindowSize.Cols,
	)

	w.host.session.stateW.Encode(st)
}

// rcvClientData handles incoming client data and commits it to the data
// channel if the client is authorized to do so.
func (w *Warp) rcvClientData(
	ctx context.Context,
	ss *Session,
	data []byte,
) {
	var mode warp.Mode
	w.mutex.Lock()
	mode = w.shellClients[ss.session.User].mode
	w.mutex.Unlock()

	if mode&warp.ModeShellWrite != 0 {
		w.data <- data
	}
}

func (w *Warp) rcvHostData(
	ctx context.Context,
	ss *Session,
	data []byte,
) {
	sessions := w.Sessions(ctx)
	for _, s := range sessions {
		logging.Logf(ctx,
			"Sending data to session: session=%s size=%d",
			s.ToString(), len(data),
		)
		_, err := s.dataC.Write(data)
		if err != nil {
			s.SendError(ctx,
				"data_send_failed",
				fmt.Sprintf("Error sending data: %v", err),
			)
			// This will disconnect the client and clean it up from the
			// session
			s.cancel()
		}
	}
}

func (w *Warp) handleHost(
	ctx context.Context,
	ss *Session,
) error {
	// run state updates
	go func() {
	HOSTLOOP:
		for {
			var st warp.HostUpdate
			if err := w.host.session.updateR.Decode(&st); err != nil {
				ss.SendError(ctx,
					"invalid_host_update",
					fmt.Sprintf("Host update decoding failed: %v", err),
				)
				break HOSTLOOP
			}

			// Check that the warp token is the same.
			if st.Warp != w.token {
				ss.SendError(ctx,
					"invalid_host_update",
					fmt.Sprintf(
						"Host update warp mismatch: %s", st.Warp,
					),
				)
				break HOSTLOOP
			}

			// Check that the session is the same in particular the secret to
			// protect against spoofing attempts.
			if st.From.Token != ss.session.Token ||
				st.From.User != ss.session.User ||
				st.From.Secret != ss.session.Secret {
				ss.SendError(ctx,
					"invalid_host_update",
					fmt.Sprintf(
						"Host update host mismatch: %s", st.From.Token,
					),
				)
				break HOSTLOOP
			}

			w.mutex.Lock()
			w.windowSize = st.WindowSize
			for user, mode := range st.Modes {
				if _, ok := w.shellClients[user]; ok {
					w.shellClients[user].mode = mode
				} else {
					logging.Logf(ctx,
						"Unknown user from host update: session=%s user=%s",
						ss.ToString(), user,
					)
				}
			}
			w.mutex.Unlock()

			logging.Logf(ctx,
				"Received host update: session=%s cols=%d rows=%d",
				ss.ToString(), st.WindowSize.Rows, st.WindowSize.Cols,
			)

			w.updateShellClients(ctx)
		}
		ss.cancel()
	}()

	// Receive host data.
	go func() {
		buf := make([]byte, 1024)
		for {
			nr, err := ss.dataC.Read(buf)
			if nr > 0 {
				cpy := make([]byte, nr)
				copy(cpy, buf)

				logging.Logf(ctx,
					"Received data from host: session=%s size=%d",
					ss.ToString(), nr,
				)
				w.rcvHostData(ctx, ss, cpy)
			}
			if err != nil {
				ss.SendError(ctx,
					"data_receive_failed",
					fmt.Sprintf("Error receiving data: %v", err),
				)
				break
			}
			select {
			case <-ss.ctx.Done():
				break
			default:
			}
		}
		ss.cancel()
	}()

	// Send data to host.
	go func() {
		for {
			select {
			case buf := <-w.data:

				logging.Logf(ctx,
					"Sending data to host: session=%s size=%d",
					ss.ToString(), len(buf),
				)

				_, err := ss.dataC.Write(buf)
				if err != nil {
					ss.SendError(ctx,
						"data_send_failed",
						fmt.Sprintf("Error sending data: %v", err),
					)
					break
				}
			case <-ss.ctx.Done():
				break
			default:
			}
		}
		ss.cancel()
	}()

	// Update host and clients (should be no client).
	w.updateHost(ctx)
	w.updateShellClients(ctx)

	logging.Logf(ctx,
		"Host session running: session=%s",
		ss.ToString(),
	)

	<-ss.ctx.Done()

	// Cancel all clients.
	logging.Logf(ctx,
		"Cancelling all clients: session=%s",
		ss.ToString(),
	)
	sessions := w.Sessions(ctx)
	for _, s := range sessions {
		s.cancel()
	}

	return nil
}

func (w *Warp) handleClient(
	ctx context.Context,
	ss *Session,
) error {
	// Add the client.
	w.mutex.Lock()
	isHostSession := false
	if ss.session.User == w.host.UserState.token {
		isHostSession = true
		// If we have a session conflict, let's kill the old one.
		if s, ok := w.host.UserState.sessions[ss.session.Token]; ok {
			s.cancel()
		}
		w.host.UserState.sessions[ss.session.Token] = ss
	} else {
		if _, ok := w.shellClients[ss.session.User]; !ok {
			w.shellClients[ss.session.User] = &UserState{
				token:    ss.session.User,
				username: ss.username,
				mode:     warp.ModeShellRead,
				sessions: map[string]*Session{},
			}
		}
		// If we have a session conflict, let's kill the old one.
		if s, ok := w.shellClients[ss.session.User].sessions[ss.session.Token]; ok {
			s.cancel()
		}
		w.shellClients[ss.session.User].sessions[ss.session.Token] = ss
	}
	w.mutex.Unlock()

	// Receive client data.
	go func() {
		buf := make([]byte, 1024)
		for {
			nr, err := ss.dataC.Read(buf)
			if nr > 0 {
				cpy := make([]byte, nr)
				copy(cpy, buf)

				logging.Logf(ctx,
					"Received data from client: session=%s size=%d",
					ss.ToString(), nr,
				)
				w.rcvClientData(ctx, ss, cpy)
			}
			if err != nil {
				ss.SendError(ctx,
					"data_receive_failed",
					fmt.Sprintf("Error receiving data: %v", err),
				)
				break
			}
			select {
			case <-ss.ctx.Done():
				break
			default:
			}
		}
		ss.cancel()
	}()

	// Update host and clients (including the new session).
	w.updateHost(ctx)
	w.updateShellClients(ctx)

	logging.Logf(ctx,
		"Client session running: session=%s",
		ss.ToString(),
	)

	<-ss.ctx.Done()

	// Clean-up client.
	logging.Logf(ctx,
		"Cleaning-up client: session=%s",
		ss.ToString(),
	)
	w.mutex.Lock()
	if isHostSession {
		delete(w.host.sessions, ss.session.Token)
	} else {
		delete(w.shellClients[ss.session.User].sessions, ss.session.Token)
		if len(w.shellClients[ss.session.User].sessions) == 0 {
			delete(w.shellClients, ss.session.User)
		}
	}
	w.mutex.Unlock()

	// Update host and remaining clients
	w.updateHost(ctx)
	w.updateShellClients(ctx)

	return nil
}
