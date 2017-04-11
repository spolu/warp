package wrp

import "fmt"

// DefaultAddress to connect to
var DefaultAddress = "warp.link:4242"

// Mode is used to represent the mode of a client (read/write).
type Mode uint32

const (
	ModeShellRead  Mode = 1
	ModeShellWrite Mode = 1 << 1
	// Future usecases: ModeSpeakRead|ModeSpeakWrite|ModeSpeakMuted
)

// SessionType encodes the type of the session:
type SessionType string

const (
	// SsTpHost the host session that created the warp (`warp open`)
	SsTpHost SessionType = "host"
	// SsTpShellClient shell client session (`warp connect`)
	SsTpShellClient SessionType = "shell"
	// SsTpChatClient chat client session (`warp chat`)
	SsTpChatClient SessionType = "chat"
)

// Size reprensents a window size.
type Size struct {
	Rows int
	Cols int
}

// User represents a user of a warp.
type User struct {
	Token    string
	Username string

	Mode    Mode
	Hosting bool
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

// State is the struct sent over the network to update sessions state.
type State struct {
	Warp       string
	WindowSize Size
	Users      map[string]User
}

// SessionHello is the initial message sent over a session update channel to
// identify itself to the server.
type SessionHello struct {
	Warp string
	From Session

	Type     SessionType
	Username string
}

// HostUpdate represents an update to the warp state from its host.
type HostUpdate struct {
	Warp string
	From Session

	WindowSize Size
	// Modes is a map from user token to mode.
	Modes map[string]Mode
}
