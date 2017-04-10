package daemon

import (
	"context"
	"encoding/gob"
	"fmt"
	"net"
	"sync"

	"github.com/spolu/wrp"
	"github.com/spolu/wrp/lib/logging"
)

// UserState represents the state of a user along with a list of all his
// sessions.
type UserState struct {
	token    string
	username string
	mode     wrp.Mode
	sessions map[string]*Session
}

// HostState represents the state of the host, in particular the host session,
// along with its UserState.
type HostState struct {
	UserState
	session *Session
	hostC   net.Conn
	hostR   *gob.Decoder
}

// Client returns a wrp.Client from the current UserState.
func (u *UserState) Client(
	ctx context.Context,
) wrp.Client {
	return wrp.Client{
		Username: u.username,
		Mode:     u.mode,
	}
}

// Warp represents a pty served from a remote host attached to a token.
type Warp struct {
	token string

	windowSize wrp.Size

	host    *HostState
	clients map[string]*UserState

	data chan []byte

	mutex *sync.Mutex
}

// State computes a wrp.State from the current session. It acquires the session
// lock.
func (w *Warp) State(
	ctx context.Context,
) wrp.State {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	state := wrp.State{
		Warp:       w.token,
		WindowSize: w.windowSize,
		Host:       w.host.UserState.Client(ctx),
		Clients:    map[string]wrp.Client{},
	}
	for token, user := range w.clients {
		state.Clients[token] = user.Client(ctx)
	}

	return state
}

// Sessions return all connected sessions that are not the host session.
func (w *Warp) Sessions(
	ctx context.Context,
) []*Session {
	clients := []*Session{}
	w.mutex.Lock()
	for _, user := range w.clients {
		for _, c := range user.sessions {
			clients = append(clients, c)
		}
	}
	for _, c := range w.host.UserState.sessions {
		clients = append(clients, c)
	}
	w.mutex.Unlock()
	return clients
}

// updateClients updates all clients with the current state.
func (w *Warp) updateClients(
	ctx context.Context,
) {
	st := w.State(ctx)
	sessions := w.Sessions(ctx)
	for _, ss := range sessions {
		logging.Logf(ctx,
			"[%s] Sending (client) state: warp=%s cols=%d rows=%d",
			ss.session.String(),
			st.Warp, st.WindowSize.Rows, st.WindowSize.Cols,
		)

		ss.stateW.Encode(st)
	}
}

// updateHost updates all clients with the current state.
func (w *Warp) updateHost(
	ctx context.Context,
) {
	st := w.State(ctx)

	logging.Logf(ctx,
		"[%s] Sending (host) state: warp=%s cols=%d rows=%d",
		w.host.session.session.String(),
		st.Warp, st.WindowSize.Rows, st.WindowSize.Cols,
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
	var mode wrp.Mode
	w.mutex.Lock()
	mode = w.clients[ss.session.User].mode
	w.mutex.Unlock()

	if mode&wrp.ModeWrite != 0 {
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
			"[%s] Sending data to session: warp=%s size=%d",
			s.session.String(), w.token, len(data),
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
			var st wrp.HostUpdate
			if err := w.host.hostR.Decode(&st); err != nil {
				ss.SendError(ctx,
					"invalid_host_update",
					fmt.Sprintf("Host update decoding failed: %v", err),
				)
				break HOSTLOOP
			}

			if st.Warp != w.token {
				ss.SendError(ctx,
					"invalid_host_update",
					fmt.Sprintf(
						"Host update warp mismatch: %s", st.Warp,
					),
				)
				break HOSTLOOP
			}
			if st.From.String() != ss.session.String() {
				ss.SendError(ctx,
					"invalid_host_update",
					fmt.Sprintf(
						"Host update host mismatch: %s", st.From.String(),
					),
				)
				break HOSTLOOP
			}

			for token, _ := range st.Modes {
				_, ok := w.clients[token]
				if !ok {
					ss.SendError(ctx,
						"invalid_host_update",
						fmt.Sprintf(
							"Host update unknown client: %s", token,
						),
					)
					break HOSTLOOP
				}
			}

			w.mutex.Lock()
			w.windowSize = st.WindowSize
			for token, mode := range st.Modes {
				w.clients[token].mode = mode
			}
			w.mutex.Unlock()

			logging.Logf(ctx,
				"[%s] Received host update: warp=%s cols=%d rows=%d",
				ss.session.String(),
				w.token, st.WindowSize.Rows, st.WindowSize.Cols,
			)

			w.updateClients(ctx)
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
					"[%s] Received data from host: warp=%s size=%d",
					ss.session.String(), w.token, nr,
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
					"[%s] Sending to host: warp=%s size=%d",
					ss.session.String(), w.token, len(buf),
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

	logging.Logf(ctx,
		"[%s] Host session running: warp=%s",
		ss.session.String(), w.token,
	)

	<-ss.ctx.Done()

	// Cancel all clients.
	logging.Logf(ctx,
		"[%s] Cancelling all clients: warp=%s",
		ss.session.String(), w.token,
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
			delete(w.host.UserState.sessions, ss.session.Token)
		}
		w.host.UserState.sessions[ss.session.Token] = ss
	} else {
		if _, ok := w.clients[ss.session.User]; !ok {
			w.clients[ss.session.User] = &UserState{
				token:    ss.session.User,
				username: ss.username,
				mode:     wrp.ModeRead,
				sessions: map[string]*Session{},
			}
		}
		// If we have a session conflict, let's kill the old one.
		if s, ok := w.clients[ss.session.User].sessions[ss.session.Token]; ok {
			s.cancel()
			delete(w.clients[ss.session.User].sessions, ss.session.Token)
		}
		w.clients[ss.session.User].sessions[ss.session.Token] = ss
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

				fmt.Printf(
					"[%s] Received data from client: warp=%s size=%d\n",
					ss.session.String(), w.token, nr,
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

	// Send initial state.
	st := w.State(ctx)
	logging.Logf(ctx,
		"[%s] Sending initial state: warp=%s cols=%d rows=%d",
		ss.session.String(), st.Warp, st.WindowSize.Rows, st.WindowSize.Cols,
	)
	ss.stateW.Encode(st)

	// Update host and clients.
	w.updateHost(ctx)
	w.updateClients(ctx)

	logging.Logf(ctx,
		"[%s] Client session running: warp=%s",
		ss.session.String(), w.token,
	)

	<-ss.ctx.Done()

	// Clean-up client.
	logging.Logf(ctx,
		"[%s] Cleaning-up client: warp=%s",
		ss.session.String(), w.token,
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

	// Update remaining clients
	w.updateClients(ctx)
	w.updateHost(ctx)

	return nil
}
