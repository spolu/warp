package warp

import "regexp"

//
// Remote Warpd Protocol
//

// Version is the current warp version.
var Version = "0.0.2"

// DefaultAddress to connect to
var DefaultAddress = "warp.link:4242"

// WarpRegexp warp token regular expression.
var WarpRegexp = regexp.MustCompile("^[a-zA-Z0-9][a-zA-Z0-9-_.]{0,255}$")

// Mode is used to represent the mode of a client (read/write).
type Mode uint32

const (
	ModeShellRead  Mode = 1
	ModeShellWrite Mode = 1 << 1
	// Future usecases: ModeSpeakRead|ModeSpeakWrite|ModeSpeakMuted

	DefaultHostMode = ModeShellRead | ModeShellWrite
	DefaultUserMode = ModeShellRead
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

// Error is th struct sent over the network in case of errors.
type Error struct {
	Code    string
	Message string
}

// Size reprensents a window size.
type Size struct {
	Rows int
	Cols int
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
	Warp    string
	From    Session
	Version string

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

//
// Local Command Server Protocol
//

// EnvWarp the env variable where the warp token is stored.
var EnvWarp = "__WARP"

// EnvWarpUnixSocket the env variable where warp unix socket path is stored.
var EnvWarpUnixSocket = "__WARP_UNIX_SOCKET"

// CommandType encodes the type of the session:
type CommandType string

const (
	// CmdTpState retrieve the state of the warp.
	CmdTpState CommandType = "state"
	// CmdTpAuthorize authorizes a user for writing.
	CmdTpAuthorize CommandType = "authorize"
	// CmdTpRevoke a (or all) user(s) authorization to write.
	CmdTpRevoke CommandType = "revoke"
)

// Command is used to send command to the local host.
type Command struct {
	Type CommandType
	Args []string
}

// CommandResult is used to send command result to the local client.
type CommandResult struct {
	Type         CommandType
	Disconnected bool
	SessionState State
	Error        Error
}
