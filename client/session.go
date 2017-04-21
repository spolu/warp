package cli

import (
	"context"
	"encoding/gob"
	"io/ioutil"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/spolu/warp"
	"github.com/spolu/warp/lib/errors"
)

// Session represents a session to warpd as part of a client or a host. All
// methods are thread-safe except the Decode* methods.
type Session struct {
	session warp.Session

	warp        string
	sessionType warp.SessionType
	username    string

	conn net.Conn
	mux  *yamux.Session

	stateC  net.Conn
	stateR  *gob.Decoder
	updateC net.Conn
	updateW *gob.Encoder
	errorC  net.Conn
	errorR  *gob.Decoder
	dataC   net.Conn

	state *WarpState

	tornDown bool
	cancel   func()

	mutex *sync.Mutex
}

// NewSession sets up a session, opens the associated channels and return a
// Session object.
func NewSession(
	ctx context.Context,
	session warp.Session,
	w string,
	sessionType warp.SessionType,
	username string,
	cancel func(),
	conn net.Conn,
) (*Session, error) {
	mux, err := yamux.Client(conn, &yamux.Config{
		AcceptBacklog:          256,
		EnableKeepAlive:        true,
		KeepAliveInterval:      2 * time.Second,
		ConnectionWriteTimeout: 10 * time.Second,
		MaxStreamWindowSize:    256 * 1024,
		LogOutput:              ioutil.Discard,
	})
	if err != nil {
		return nil, errors.Trace(
			errors.Newf("Failed to open session to warpd: %v", err),
		)
	}

	ss := &Session{
		session:     session,
		warp:        w,
		sessionType: sessionType,
		username:    username,
		conn:        conn,
		mux:         mux,
		cancel:      cancel,
		mutex:       &sync.Mutex{},
	}

	// Opens state channel stateC.
	ss.stateC, err = mux.Open()
	if err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("State channel open error: %v", err),
		)
	}
	ss.stateR = gob.NewDecoder(ss.stateC)

	// Open update channel updateC.
	ss.updateC, err = mux.Open()
	if err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("Update channel open error: %v", err),
		)
	}
	ss.updateW = gob.NewEncoder(ss.updateC)

	// Send initial SessionHello.
	hello := warp.SessionHello{
		Warp:     ss.warp,
		From:     ss.session,
		Version:  warp.Version,
		Type:     ss.sessionType,
		Username: ss.username,
	}
	if err := ss.updateW.Encode(hello); err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("Send hello error: %v", err),
		)
	}

	// Opens error channel errorC.
	ss.errorC, err = mux.Open()
	if err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("Error channel open error: %v", err),
		)
	}
	ss.errorR = gob.NewDecoder(ss.errorC)

	// Open data channel dataC.
	ss.dataC, err = mux.Open()
	if err != nil {
		ss.TearDown()
		return nil, errors.Trace(
			errors.Newf("Data channel open error: %v", err),
		)
	}

	// Setup warp state.
	ss.state = NewWarpState(hello)

	return ss, nil
}

// Command methods

// DataC returns the data channel. Using the dataC is not thread-safe and
// should happen from only one go routine for reading only. Writing should go
// through thread-safe WriteDataC.
func (ss *Session) DataC() net.Conn {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.dataC
}

// WriteData writes to dataC in a thread-safe way, checking that the session is
// not torn down.
func (ss *Session) WriteDataC(
	data []byte,
) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	if !ss.tornDown {
		ss.dataC.Write(data)
	}
}

// Warp returns the session warp token.
func (ss *Session) Warp() string {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.warp
}

// Session returns the protocol session representation.
func (ss *Session) Session() warp.Session {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.session
}

// State returns the session warp state.
func (ss *Session) ProtocolState() warp.State {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.state.ProtocolState()
}

// GetMode returns the mode of a user.
func (ss *Session) GetMode(
	user string,
) (*warp.Mode, error) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.state.GetMode(user)
}

// SetMode sets the mode for a user.
func (ss *Session) SetMode(
	user string,
	mode warp.Mode,
) error {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.state.SetMode(user, mode)
}

// UpdateState updates the session state with a received warp.State.
func (ss *Session) UpdateState(
	state warp.State,
	hosting bool,
) error {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.state.Update(state, hosting)
}

// HostCanReceiverWrite retruns whether the host can receive write from any
// shell client.
func (ss *Session) HostCanReceiveWrite() bool {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.state.HostCanReceiveWrite()
}

// WindowSizse returns the current window size.
func (ss *Session) WindowSize() warp.Size {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.state.WindowSize()
}

// Modes returns user modes.
func (ss *Session) Modes() map[string]warp.Mode {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.state.Modes()
}

// TornDown returns the session tornDown value.
func (ss *Session) TornDown() bool {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.tornDown
}

// TearDown tears down a session, closing and reclaiming channels.
func (ss *Session) TearDown() {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	if !ss.tornDown {
		ss.tornDown = true
		ss.cancel()
		// Closes stateC, updateC, errorC, dataC, mux and conn.
		ss.mux.Close()
	}
}

// SendHostUpdate is used to safely concurrently sending host updates.
func (ss *Session) SendHostUpdate(
	ctx context.Context,
	update warp.HostUpdate,
) error {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	if !ss.tornDown {
		if err := ss.updateW.Encode(update); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

//
// Non thread-safe methods.
//

// DecodeError attempts to decode an error from the errorC. This method is not
// thread-safe.
func (ss *Session) DecodeError(
	ctx context.Context,
) (*warp.Error, error) {
	var e warp.Error
	if err := ss.errorR.Decode(&e); err != nil {
		return nil, errors.Trace(err)
	}
	return &e, nil
}

// DecodeState attempts to decode state from the sateC. This method is not
// thread-safe.
func (ss *Session) DecodeState(
	ctx context.Context,
) (*warp.State, error) {
	var st warp.State
	if err := ss.stateR.Decode(&st); err != nil {
		return nil, errors.Trace(err)
	}
	return &st, nil
}
