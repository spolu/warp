package command

import (
	"context"
	"os"

	"github.com/spolu/warp"
	"github.com/spolu/warp/cli"
	"github.com/spolu/warp/lib/errors"
	"github.com/spolu/warp/lib/logging"
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
	out.Normf("  Displays the state of the current warp, including the list of connected users and their authorization. This command is only available\n")
	out.Normf("  from inside a warp.\n")
	out.Normf("\n")
	out.Normf("Examples:\n")
	out.Valuf("  warp state\n")
	out.Normf("\n")
}

// Parse parses the arguments passed to the command.
func (c *State) Parse(
	ctx context.Context,
	args []string,
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
		return errors.Trace(
			errors.Newf("Failed to execute warp command: %v.", err),
		)
	}

	logging.Logf(ctx, "RESULT: %+v", result)

	return nil
}
