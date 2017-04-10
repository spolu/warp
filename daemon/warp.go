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
	sessions map[string]*Client
}

// HostState represents the state of the host, in particular the host session,
// along with its UserState.
type HostState struct {
	UserState
	session *Client
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

// Clients return all connected clients that are not the host client.
func (w *Warp) Clients(
	ctx context.Context,
) []*Client {
	clients := []*Client{}
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
	clients := w.Clients(ctx)
	for _, c := range clients {
		logging.Logf(ctx,
			"[%s] Sending (client) state: warp=%s cols=%d rows=%d",
			c.session.String(),
			st.Warp, st.WindowSize.Rows, st.WindowSize.Cols,
		)

		c.stateW.Encode(st)
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
	c *Client,
	data []byte,
) {
	var mode wrp.Mode
	w.mutex.Lock()
	mode = w.clients[c.session.User].mode
	w.mutex.Unlock()

	if mode&wrp.ModeWrite != 0 {
		w.data <- data
	}
}

func (w *Warp) rcvHostData(
	ctx context.Context,
	client *Client,
	data []byte,
) {
	clients := w.Clients(ctx)
	for _, cc := range clients {
		logging.Logf(ctx,
			"[%s] Sending data to client: warp=%s size=%d",
			cc.session.String(), w.token, len(data),
		)
		_, err := cc.dataC.Write(data)
		if err != nil {
			cc.SendError(ctx,
				"data_send_failed",
				fmt.Sprintf("Error sending data: %v", err),
			)
			// This will disconnect the client and clean it up from the
			// session
			cc.cancel()
		}
	}
}

func (w *Warp) handleHost(
	ctx context.Context,
	c *Client,
) error {
	// run state updates
	go func() {
	HOSTLOOP:
		for {
			var st wrp.HostUpdate
			if err := w.host.hostR.Decode(&st); err != nil {
				c.SendError(ctx,
					"invalid_host_update",
					fmt.Sprintf("Host update decoding failed: %v", err),
				)
				break HOSTLOOP
			}

			if st.Warp != w.token {
				c.SendError(ctx,
					"invalid_host_update",
					fmt.Sprintf(
						"Host update warp mismatch: %s", st.Warp,
					),
				)
				break HOSTLOOP
			}
			if st.From.String() != c.session.String() {
				c.SendError(ctx,
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
					c.SendError(ctx,
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
				c.session.String(),
				w.token, st.WindowSize.Rows, st.WindowSize.Cols,
			)

			w.updateClients(ctx)
		}
		c.cancel()
	}()

	// Receive host data.
	go func() {
		buf := make([]byte, 1024)
		for {
			nr, err := c.dataC.Read(buf)
			if nr > 0 {
				cpy := make([]byte, nr)
				copy(cpy, buf)

				logging.Logf(ctx,
					"[%s] Received data from host: warp=%s size=%d",
					c.session.String(), w.token, nr,
				)
				w.rcvHostData(ctx, c, cpy)
			}
			if err != nil {
				c.SendError(ctx,
					"data_receive_failed",
					fmt.Sprintf("Error receiving data: %v", err),
				)
				break
			}
			select {
			case <-c.ctx.Done():
				break
			default:
			}
		}
		c.cancel()
	}()

	// Send data to host.
	go func() {
		for {
			select {
			case buf := <-w.data:

				logging.Logf(ctx,
					"[%s] Sending to host: warp=%s size=%d",
					c.session.String(), w.token, len(buf),
				)

				_, err := c.dataC.Write(buf)
				if err != nil {
					c.SendError(ctx,
						"data_send_failed",
						fmt.Sprintf("Error sending data: %v", err),
					)
					break
				}
			case <-c.ctx.Done():
				break
			default:
			}
		}
		c.cancel()
	}()

	logging.Logf(ctx,
		"[%s] Host running: warp=%s",
		c.session.String(), w.token,
	)

	<-c.ctx.Done()

	// Cancel all clients.
	logging.Logf(ctx,
		"[%s] Cancelling all clients: warp=%s",
		c.session.String(), w.token,
	)
	clients := w.Clients(ctx)
	for _, cc := range clients {
		cc.cancel()
	}

	return nil
}

func (w *Warp) handleClient(
	ctx context.Context,
	c *Client,
) error {
	// Add the client.
	w.mutex.Lock()
	isHostSession := false
	if c.session.User == w.host.UserState.token {
		isHostSession = true
		// If we have a session conflict, let's kill the old one.
		if c, ok := w.host.UserState.sessions[c.session.Token]; ok {
			c.cancel()
			delete(w.host.UserState.sessions, c.session.Token)
		}
		w.host.UserState.sessions[c.session.Token] = c
	} else {
		if _, ok := w.clients[c.session.User]; !ok {
			w.clients[c.session.User] = &UserState{
				token:    c.session.User,
				username: c.username,
				mode:     wrp.ModeRead,
				sessions: map[string]*Client{},
			}
		}
		// If we have a session conflict, let's kill the old one.
		if c, ok := w.clients[c.session.User].sessions[c.session.Token]; ok {
			c.cancel()
			delete(w.clients[c.session.User].sessions, c.session.Token)
		}
		w.clients[c.session.User].sessions[c.session.Token] = c
	}
	w.mutex.Unlock()

	// Receive client data.
	go func() {
		buf := make([]byte, 1024)
		for {
			nr, err := c.dataC.Read(buf)
			if nr > 0 {
				cpy := make([]byte, nr)
				copy(cpy, buf)

				fmt.Printf(
					"[%s] Received data from client: warp=%s size=%d\n",
					c.session.String(), w.token, nr,
				)
				w.rcvClientData(ctx, c, cpy)
			}
			if err != nil {
				c.SendError(ctx,
					"data_receive_failed",
					fmt.Sprintf("Error receiving data: %v", err),
				)
				break
			}
			select {
			case <-c.ctx.Done():
				break
			default:
			}
		}
		c.cancel()
	}()

	// Send initial state.
	st := w.State(ctx)
	logging.Logf(ctx,
		"[%s] Sending initial state: warp=%s cols=%d rows=%d",
		c.session.String(), st.Warp, st.WindowSize.Rows, st.WindowSize.Cols,
	)
	c.stateW.Encode(st)

	// Update host and clients.
	w.updateHost(ctx)
	w.updateClients(ctx)

	logging.Logf(ctx,
		"[%s] Client running: warp=%s",
		c.session.String(), w.token,
	)

	<-c.ctx.Done()

	// Clean-up client.
	logging.Logf(ctx,
		"[%s] Cleaning-up client: warp=%s",
		c.session.String(), w.token,
	)
	w.mutex.Lock()
	if isHostSession {
		delete(w.host.sessions, c.session.Token)
	} else {
		delete(w.clients[c.session.User].sessions, c.session.Token)
		if len(w.clients[c.session.User].sessions) == 0 {
			delete(w.clients, c.session.User)
		}
	}
	w.mutex.Unlock()

	// Update remaining clients
	w.updateClients(ctx)
	w.updateHost(ctx)

	return nil
}
