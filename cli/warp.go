package cli

import (
	"sync"

	"github.com/spolu/warp"
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
				mode:     warp.ModeShellRead,
				hosting:  false,
			},
		},
		mutex: &sync.Mutex{},
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
	return nil
}

// SetMode updates the mode of a given user.
func (w *Warp) SetMode(
	user string,
	mode warp.Mode,
) {
}
