package cli

import (
	"github.com/spolu/warp"
	"github.com/spolu/warp/lib/errors"
)

// WarpState repreents the state of a warp client side. The warp state method
// are not thread-safe and access to it should be protected by the associated
// session lock.
type WarpState struct {
	token string

	windowSize warp.Size
	users      map[string]UserState
}

// UserState represents the state of a user as seen client-side.
type UserState struct {
	token    string
	username string
	mode     warp.Mode
	hosting  bool
}

// User returns a warp.User from the current UserState.
func (u *UserState) ProtocolUser() warp.User {
	return warp.User{
		Token:    u.token,
		Username: u.username,
		Mode:     u.mode,
		Hosting:  u.hosting,
	}
}

// Returns a new warp state initialized by a hello message.
func NewWarpState(
	hello warp.SessionHello,
) *Warp {
	w := &WarpState{
		token: hello.Warp,
		users: map[string]UserState{
			hello.From.User: UserState{
				token:    hello.From.User,
				username: hello.Username,
				mode:     warp.DefaultUserMode,
				hosting:  false,
			},
		},
	}
	if hello.Type == warp.SsTpHost {
		userState := w.users[hello.From.User]
		userState.hosting = true
		userState.mode = warp.DefaultHostMode
		w.users[hello.From.User] = userState
	}
	return w
}

// Update the warp state given a warp.State received over the wire.
//
// If preserveModes is true the modes are preserved (used from the host session
// as the server is not trusted with modes). If the state includes an unknown
// user, the default secure modes are used (~read-only).
func (w *WarpState) Update(
	state warp.State,
	hosting bool,
) error {
	if state.Warp != w.token {
		return errors.Trace(
			errors.Newf("Warp token mismatch: %s", state.Warp),
		)
	}

	w.windowSize = state.WindowSize

	for token, user := range state.Users {
		if token != user.Token {
			return errors.Trace(
				errors.Newf(
					"User token mismatch: %s <> %s",
					token, user.Token,
				),
			)
		}
		if _, ok := w.users[token]; !ok {
			// User connected.

			if hosting && user.Hosting {
				return errors.Trace(
					errors.Newf("Unexptected hosting user update: %s", token),
				)
			}
			if hosting && user.Mode != warp.DefaultUserMode {
				return errors.Trace(
					errors.Newf(
						"Unexptected user update mode: %s %d",
						token, user.Mode,
					),
				)
			}

			// We have a new user that connected let's add it.
			w.users[token] = UserState{
				token:    token,
				username: user.Username,
				mode:     warp.DefaultUserMode,
				hosting:  user.Hosting,
			}
		} else {
			// Update the user state.
			userState := w.users[token]
			userState.username = user.Username
			if !hosting {
				userState.mode = user.Mode
			}
			w.users[token] = userState
		}
	}

	for token, _ := range w.users {
		if _, ok := state.Users[token]; !ok {
			// User disconnected.
			delete(w.users, token)
		}
	}

	return nil
}

// GetMode returns the mode of a given user.
func (w *WarpState) GetMode(
	user string,
) (*warp.Mode, error) {
	userState, ok := w.users[user]
	if !ok {
		return nil, errors.Trace(
			errors.Newf("Unknown user: %s", user),
		)
	}

	return &userState.mode, nil
}

// SetMode updates the mode of a given user.
func (w *WarpState) SetMode(
	user string,
	mode warp.Mode,
) error {
	userState, ok := w.users[user]
	if !ok {
		return errors.Trace(
			errors.Newf("Unknown user: %s", user),
		)
	}

	userState.mode = mode
	w.users[user] = userState

	return nil
}

// HostCanReceiveWrite computes whether the host can receive write from the
// shell clients. This is used as defense in depth to prevent any write if
// that's not the case.
func (w *WarpState) HostCanReceiveWrite() bool {
	can := false
	for _, user := range w.users {
		if !user.hosting && user.mode&warp.ModeShellWrite != 0 {
			can = true
		}
	}
	return can
}

// ProtocolState computes a warp.State from the current warp. It acquires the
// warp lock.
func (w *WarpState) ProtocolState() warp.State {
	state := warp.State{
		Warp:       w.token,
		WindowSize: w.windowSize,
		Users:      map[string]warp.User{},
	}

	for token, user := range w.users {
		state.Users[token] = user.ProtocolUser()
	}

	return state
}

// WindowSizse returns the current window size.
func (w *WarpState) WindowSize() warp.Size {
	return w.windowSize
}

// Modes returns user modes.
func (w *WarpState) Modes() map[string]warp.Mode {
	modes := map[string]warp.Mode{}
	for token, u := range w.users {
		if !u.hosting {
			modes[token] = u.mode
		}
	}
	return modes
}
