package cli

import (
	"sync"

	"github.com/spolu/warp"
	"github.com/spolu/warp/lib/errors"
)

// UserState represents the state of a user as seen client-side.
type UserState struct {
	token    string
	username string
	mode     warp.Mode
	hosting  bool
}

// Warp repreents the state of a warp client side.
type Warp struct {
	token string

	windowSize warp.Size
	users      map[string]UserState

	mutex *sync.Mutex
}

// Returns a new empty wrap state.
func NewWarp(
	hello warp.SessionHello,
) *Warp {
	w := &Warp{
		token: hello.Warp,
		users: map[string]UserState{
			hello.From.User: UserState{
				token:    hello.From.User,
				username: hello.Username,
				mode:     warp.DefaultUserMode,
				hosting:  false,
			},
		},
		mutex: &sync.Mutex{},
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
func (w *Warp) Update(
	state warp.State,
	hosting bool,
) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

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

// SetMode updates the mode of a given user.
func (w *Warp) SetMode(
	user string,
	mode warp.Mode,
) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

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
func (w *Warp) HostCanReceiveWrite() bool {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	can := false
	for _, user := range w.users {
		if !user.hosting && user.mode&warp.ModeShellWrite != 0 {
			can = true
		}
	}
	return can
}
