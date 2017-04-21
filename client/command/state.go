package command

import (
	"context"
	"os"

	"github.com/spolu/warp"
	"github.com/spolu/warp/client"
	"github.com/spolu/warp/lib/errors"
	"github.com/spolu/warp/lib/out"
)

const (
	// CmdNmState is the command name.
	CmdNmState cli.CmdName = "state"
)

func init() {
	cli.Registrar[CmdNmState] = NewState
}

// State retrieve the state of the current warp (in-warp only).
type State struct {
}

// NewState constructs and initializes the command.
func NewState() cli.Command {
	return &State{}
}

// Name returns the command name.
func (c *State) Name() cli.CmdName {
	return CmdNmState
}

// Help prints out the help message for the command.
func (c *State) Help(
	ctx context.Context,
) {
	out.Normf("\nUsage: ")
	out.Boldf("warp state\n")
	out.Normf("\n")
	out.Normf("  Displays the state of the current warp, including the list of connected users\n")
	out.Normf("  and their authorization state. This command is only available from inside a\n")
	out.Normf("  warp.\n")
	out.Normf("\n")
	out.Normf("Examples:\n")
	out.Valuf("  warp state\n")
	out.Normf("\n")
}

// Parse parses the arguments passed to the command.
func (c *State) Parse(
	ctx context.Context,
	args []string,
	flags map[string]string,
) error {
	if os.Getenv(warp.EnvWarpUnixSocket) == "" {
		return errors.Trace(
			errors.Newf("This command is only available from inside a warp."),
		)
	}

	return nil
}

// Execute the command or return a human-friendly error.
func (c *State) Execute(
	ctx context.Context,
) error {

	result, err := cli.RunLocalCommand(ctx, warp.Command{
		Type: warp.CmdTpState,
		Args: []string{},
	})
	if err != nil {
		return errors.Trace(err)
	}

	PrintSessionState(ctx, result.Disconnected, result.SessionState)

	return nil
}

func PrintSessionState(
	ctx context.Context,
	disconnected bool,
	state warp.State,
) {
	out.Boldf("Warp:\n")
	out.Normf("  ID: ")
	out.Valuf("%s\n", state.Warp)
	if !disconnected {
		out.Normf("  Size: ")
		out.Valuf(
			"%dx%d\n", state.WindowSize.Cols, state.WindowSize.Rows,
		)
	}
	out.Normf("  Status: ")
	if disconnected {
		out.Errof("disconnected\n")
	} else {
		out.Statf("connected\n")
	}
	out.Normf("\n")

	out.Boldf("Host:\n")
	for _, u := range state.Users {
		if u.Hosting {
			out.Normf("  ID: ")
			out.Valuf("%s", u.Token)
			out.Normf(" Username: ")
			out.Valuf("%s", u.Username)
			out.Normf("\n")
		}
	}
	out.Normf("\n")

	if !disconnected {
		out.Boldf("Clients:\n")
		found := false
		for _, u := range state.Users {
			if !u.Hosting {
				found = true
				out.Normf("  ID: ")
				out.Valuf("%s", u.Token)
				out.Normf(" Username: ")
				out.Valuf("%s", u.Username)
				out.Normf(" Authorized: ")
				if u.Mode&warp.ModeShellWrite != 0 {
					out.Errof("true")
				} else {
					out.Valuf("false")
				}
				out.Normf("\n")
			}
		}
		if !found {
			out.Normf("  No client.\n")
		}
	}

}
