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

// Client returns a wrp.Client from the current UserState.
func (u *UserState) Client(
	ctx context.Context,
) wrp.Client {
	return wrp.Client{
		Username: u.username,
		Mode:     u.mode,
	}
}

// Session represents a pty served from a remote host attached to a token.
type Session struct {
	token string

	windowSize wrp.Size

	host    *UserState
	clients map[string]*UserState

	hostC net.Conn
	hostR *gob.Decoder

	data chan []byte

	mutex *sync.Mutex
}

// State computes a wrp.State from the current session. It acquires the session
// lock.
func (s *Session) State(
	ctx context.Context,
) wrp.State {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	state := wrp.State{
		Session:    s.token,
		WindowSize: s.windowSize,
		Host:       s.host.Client(ctx),
		Clients:    map[string]wrp.Client{},
	}
	for token, user := range s.clients {
		state.Clients[token] = user.Client(ctx)
	}

	return state
}

// Clients return all connected clients that are not the host client.
func (s *Session) Clients(
	ctx context.Context,
) []*Client {
	clients := []*Client{}
	s.mutex.Lock()
	for _, user := range s.clients {
		for _, c := range user.sessions {
			clients = append(clients, c)
		}
	}
	for _, c := range s.host.sessions {
		clients = append(clients, c)
	}
	s.mutex.Unlock()
	return clients
}

// sendStateUpdate updates all clients with the current state.
func (s *Session) sendStateUpdate(
	ctx context.Context,
) {
	st := s.State(ctx)
	clients := s.Clients(ctx)
	for _, c := range clients {
		logging.Logf(ctx,
			"[%s] Sending state: session=%s cols=%d rows=%d",
			c.user.String(),
			st.Session, st.WindowSize.Rows, st.WindowSize.Cols,
		)

		c.stateW.Encode(st)
	}
}

// rcvClientData handles incoming client data and commits it to the data
// channel if the client is authorized to do so.
func (s *Session) rcvClientData(
	ctx context.Context,
	c *Client,
	data []byte,
) {
	var mode wrp.Mode
	s.mutex.Lock()
	mode = s.clients[c.user.Token].mode
	s.mutex.Unlock()

	if mode&wrp.ModeWrite != 0 {
		s.data <- data
	}
}

func (s *Session) rcvHostData(
	ctx context.Context,
	client *Client,
	data []byte,
) {
	clients := s.Clients(ctx)
	for _, cc := range clients {
		logging.Logf(ctx,
			"[%s] Sending data to client: session=%s size=%d",
			cc.user.String(), s.token, len(data),
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

func (s *Session) handleHost(
	ctx context.Context,
	c *Client,
) error {
	// run state updates
	go func() {
	HOSTLOOP:
		for {
			var st wrp.HostUpdate
			if err := s.hostR.Decode(&st); err != nil {
				c.SendError(ctx,
					"invalid_host_update",
					fmt.Sprintf("Host update decoding failed: %v", err),
				)
				break HOSTLOOP
			}

			if st.Session != s.token {
				c.SendError(ctx,
					"invalid_host_update",
					fmt.Sprintf(
						"Host update session mismatch: %s", st.Session,
					),
				)
				break HOSTLOOP
			}
			if st.From.String() != c.user.String() {
				c.SendError(ctx,
					"invalid_host_update",
					fmt.Sprintf(
						"Host update host mismatch: %s", st.From.String(),
					),
				)
				break HOSTLOOP
			}

			for token, _ := range st.Modes {
				_, ok := s.clients[token]
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

			s.mutex.Lock()
			s.windowSize = st.WindowSize
			for token, mode := range st.Modes {
				s.clients[token].mode = mode
			}
			s.mutex.Unlock()

			logging.Logf(ctx,
				"[%s] Received host update: session=%s cols=%d rows=%d",
				c.user.String(),
				s.token, st.WindowSize.Rows, st.WindowSize.Cols,
			)

			s.sendStateUpdate(ctx)
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
					"[%s] Received data from host: session=%s size=%d",
					c.user.String(), s.token, nr,
				)
				s.rcvHostData(ctx, c, cpy)
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
			case buf := <-s.data:

				logging.Logf(ctx,
					"[%s] Sending to host: session=%s size=%d",
					c.user.String(), s.token, len(buf),
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
		"[%s] Host running: session=%s",
		c.user.String(), s.token,
	)

	<-c.ctx.Done()

	// Cancel all clients.
	logging.Logf(ctx,
		"[%s] Cancelling all clients: session=%s",
		c.user.String(), s.token,
	)
	clients := s.Clients(ctx)
	for _, cc := range clients {
		cc.cancel()
	}

	return nil
}

func (s *Session) handleClient(
	ctx context.Context,
	c *Client,
) error {
	// Add the client.
	s.mutex.Lock()
	isHostSession := false
	if c.user.Token == s.host.token {
		isHostSession = true
		// If we have a session conflict, let's kill the old one.
		if c, ok := s.host.sessions[c.user.Session]; ok {
			c.cancel()
			delete(s.host.sessions, c.user.Session)
		}
		s.host.sessions[c.user.Session] = c
	} else {
		if _, ok := s.clients[c.user.Token]; !ok {
			s.clients[c.user.Token] = &UserState{
				token:    c.user.Token,
				username: c.username,
				mode:     wrp.ModeRead,
				sessions: map[string]*Client{},
			}
		}
		// If we have a session conflict, let's kill the old one.
		if c, ok := s.clients[c.user.Token].sessions[c.user.Session]; ok {
			c.cancel()
			delete(s.clients[c.user.Token].sessions, c.user.Session)
		}
		s.clients[c.user.Token].sessions[c.user.Session] = c
	}
	s.mutex.Unlock()

	// Receive client data.
	go func() {
		buf := make([]byte, 1024)
		for {
			nr, err := c.dataC.Read(buf)
			if nr > 0 {
				cpy := make([]byte, nr)
				copy(cpy, buf)

				fmt.Printf(
					"[%s] Received data from client: session=%s size=%d\n",
					c.user.String(), s.token, nr,
				)
				s.rcvClientData(ctx, c, cpy)
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
	st := s.State(ctx)

	logging.Logf(ctx,
		"[%s] Sending initial state: session=%s cols=%d rows=%d",
		c.user.String(), st.Session, st.WindowSize.Rows, st.WindowSize.Cols,
	)

	c.stateW.Encode(st)

	logging.Logf(ctx,
		"[%s] Client running: session=%s",
		c.user.String(), s.token,
	)

	<-c.ctx.Done()

	// Clean-up client.
	logging.Logf(ctx,
		"[%s] Cleaning-up client: session=%s",
		c.user.String(), s.token,
	)
	s.mutex.Lock()
	if isHostSession {
		delete(s.host.sessions, c.user.Session)
	} else {
		delete(s.clients[c.user.Token].sessions, c.user.Session)
		if len(s.clients[c.user.Token].sessions) == 0 {
			delete(s.clients, c.user.Token)
		}
	}
	s.mutex.Unlock()

	// Update remaining clients
	s.sendStateUpdate(ctx)

	return nil
}
