package wrp

import "fmt"

// DefaultAddress to connect to
var DefaultAddress = "warp.link:4242"

// Mode is used to represent the mode of a client (read/write).
type Mode uint32

const (
	ModeRead  Mode = 1
	ModeWrite Mode = 1 << 1
)

// Size reprensents a window size.
type Size struct {
	Rows int
	Cols int
}

// Client represents a client connected to the wrp.
type Client struct {
	Username string
	Mode     Mode
}

// Session identifies a user's session.
type Session struct {
	Token  string
	User   string
	Secret string
}

func (u Session) String() string {
	return fmt.Sprintf(
		"%s:%s",
		u.User, u.Token,
	)
}

// State is the struct sent over the network to update the client state.
type State struct {
	Warp string

	Host    Client
	Clients map[string]Client

	WindowSize Size
}

// HostUpdate represents an update to the wrp general state from its host.
type HostUpdate struct {
	Warp string
	From Session

	WindowSize Size
	Modes      map[string]Mode
}

// ClientUpdate represents an update to the wrp state for a particular client,
// sent from the client or the host. A initial update is sent both when opening
// or connecting to a session.
type ClientUpdate struct {
	Warp string
	From Session

	Hosting  bool
	Username string
}
