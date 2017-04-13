package daemon

import (
	"context"
	"sync"

	"github.com/spolu/warp"
	"github.com/spolu/warp/lib/logging"
	"github.com/spolu/warp/lib/plex"
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

	host    *HostState
	clients map[string]*UserState

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

	for token, user := range w.clients {
		state.Users[token] = user.User(ctx)
	}

	return state
}

// CientSessions return all connected sessions that are not the host session.
func (w *Warp) CientSessions(
	ctx context.Context,
) []*Session {
	sessions := []*Session{}
	w.mutex.Lock()
	for _, user := range w.clients {
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

// updateClientSessions updates all shell clients with the current warp state.
func (w *Warp) updateClientSessions(
	ctx context.Context,
) {
	st := w.State(ctx)
	sessions := w.CientSessions(ctx)
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

// rcvShellClientData handles incoming client data and commits it to the data
// channel if the client is authorized to do so.
func (w *Warp) rcvShellClientData(
	ctx context.Context,
	ss *Session,
	data []byte,
) {
	var mode warp.Mode
	w.mutex.Lock()
	mode = w.clients[ss.session.User].mode
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
	sessions := w.CientSessions(ctx)
	for _, s := range sessions {
		logging.Logf(ctx,
			"Sending data to session: session=%s size=%d",
			s.ToString(), len(data),
		)
		_, err := s.dataC.Write(data)
		if err != nil {
			// If we fail to write to a session, send an internal error there
			// and tear down the session. This will not impact the warp.
			s.SendInternalError(ctx)
			s.TearDown()
		}
	}
}

// handleHost is responsible for handling the host session. It is in charge of:
// - receiving and validating host update.
// - multiplexing host data to shell clients.
// - sending received (and authorized) data to the host session.
func (w *Warp) handleHost(
	ctx context.Context,
	ss *Session,
) {
	// Add the host.
	w.mutex.Lock()
	w.host = &HostState{
		UserState: UserState{
			token:    ss.session.User,
			username: ss.username,
			mode:     warp.DefaultHostMode,
			// Initialize host sessions as empty as the current client is
			// the host session and does not act as "client". Subsequent
			// client session coming from the host would be added to this
			// list.
			sessions: map[string]*Session{},
		},
		session: ss,
	}
	w.mutex.Unlock()

	// run state updates
	go func() {
	HOSTLOOP:
		for {
			var st warp.HostUpdate
			if err := w.host.session.updateR.Decode(&st); err != nil {
				logging.Logf(ctx,
					"Error receiving host udpate: session=%s error=%v",
					ss.ToString, err,
				)
				break HOSTLOOP
			}

			// Check that the warp token is the same.
			if st.Warp != w.token {
				logging.Logf(ctx,
					"Host update warp mismatch: session=%s "+
						"expected=% received=%s",
					ss.ToString, w.token, st.Warp,
				)
				break HOSTLOOP
			}

			// Check that the session is the same in particular the secret to
			// protect against spoofing attempts.
			if st.From.Token != ss.session.Token ||
				st.From.User != ss.session.User ||
				st.From.Secret != ss.session.Secret {
				logging.Logf(ctx,
					"Host credentials mismatch: session=%s",
					ss.ToString,
				)
				break HOSTLOOP
			}

			w.mutex.Lock()
			w.windowSize = st.WindowSize
			for user, mode := range st.Modes {
				if _, ok := w.clients[user]; ok {
					w.clients[user].mode = mode
				} else {
					logging.Logf(ctx,
						"Unknown user from host update: session=%s user=%s",
						ss.ToString(), user,
					)
					break HOSTLOOP
				}
			}
			w.mutex.Unlock()

			logging.Logf(ctx,
				"Received host update: session=%s cols=%d rows=%d",
				ss.ToString(), st.WindowSize.Rows, st.WindowSize.Cols,
			)

			w.updateClientSessions(ctx)
		}
		ss.SendInternalError(ctx)
		ss.TearDown()
	}()

	// Receive host data.
	go func() {
		plex.Run(ctx, func(data []byte) {
			logging.Logf(ctx,
				"Received data from host: session=%s size=%d",
				ss.ToString(), len(data),
			)
			w.rcvHostData(ctx, ss, data)
		}, ss.dataC)
		ss.SendInternalError(ctx)
		ss.TearDown()
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
					break
				}
			case <-ss.ctx.Done():
				break
			default:
			}
		}
		ss.SendInternalError(ctx)
		ss.TearDown()
	}()

	// Update host and clients (should be no client).
	w.updateHost(ctx)
	w.updateClientSessions(ctx)

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
	sessions := w.CientSessions(ctx)
	for _, s := range sessions {
		s.SendError(ctx,
			"host_disconnected",
			"The warp host disconnected.",
		)
		s.TearDown()
	}
}

// handleShellClient is responsible for handling the SsTpShellClient sessions.
// It is in charge of:
// - receiving shell client data and passing it to the host if authorized.
func (w *Warp) handleShellClient(
	ctx context.Context,
	ss *Session,
) {
	// Add the client.
	w.mutex.Lock()
	isHostSession := false
	if ss.session.User == w.host.UserState.token {
		isHostSession = true
		// If we have a session conflict, let's kill the old one.
		if s, ok := w.host.UserState.sessions[ss.session.Token]; ok {
			s.TearDown()
		}
		w.host.UserState.sessions[ss.session.Token] = ss
	} else {
		if _, ok := w.clients[ss.session.User]; !ok {
			w.clients[ss.session.User] = &UserState{
				token:    ss.session.User,
				username: ss.username,
				mode:     warp.DefaultUserMode,
				sessions: map[string]*Session{},
			}
		}
		// If we have a session conflict, let's kill the old one.
		if s, ok := w.clients[ss.session.User].sessions[ss.session.Token]; ok {
			s.TearDown()
		}
		w.clients[ss.session.User].sessions[ss.session.Token] = ss
	}
	w.mutex.Unlock()

	// Receive shell client data.
	go func() {
		plex.Run(ctx, func(data []byte) {
			logging.Logf(ctx,
				"Received data from client: session=%s size=%d",
				ss.ToString(), len(data),
			)
			w.rcvShellClientData(ctx, ss, data)
		}, ss.dataC)
		ss.SendInternalError(ctx)
		ss.TearDown()
	}()

	// Update host and clients (including the new session).
	w.updateHost(ctx)
	w.updateClientSessions(ctx)

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
		delete(w.clients[ss.session.User].sessions, ss.session.Token)
		if len(w.clients[ss.session.User].sessions) == 0 {
			delete(w.clients, ss.session.User)
		}
	}
	w.mutex.Unlock()

	// Update host and remaining clients
	w.updateHost(ctx)
	w.updateClientSessions(ctx)
}
