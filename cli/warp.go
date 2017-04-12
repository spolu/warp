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
		w.hosting = true
		w.users[hello.From.User].mode = warp.DefaultHostMode
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
	preserveModes bool,
) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if state.Warp != w.token {
		return errors.Trace(
			errors.Newf("Warp token mismatch: %s", state.Warp),
		)
	}

	w.windowSize = state.WindowState

	for token, user := range state.Users {
		if _, ok := w.users[token]; !ok {
			if user.Hosting {
				return errors.Trace(
					errors.Newf("Unexptected hosting user update: %s", token),
				)
			}
			if preserveModes && user.Mode != warp.DefaultUserMode {
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
				mode:     user.Mode,
				hosting:  user.Hosting,
			}
		}
	}
	return nil
}

// SetMode updates the mode of a given user.
func (w *Warp) SetMode(
	user string,
	mode warp.Mode,
) {
}
