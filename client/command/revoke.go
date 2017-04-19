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
	// CmdNmRevoke is the command name.
	CmdNmRevoke cli.CmdName = "revoke"
)

func init() {
	cli.Registrar[CmdNmRevoke] = NewRevoke
}

// Revoke authorizes write access to a warp client.
type Revoke struct {
	usernameOrToken string
}

// NewRevoke constructs and initializes the command.
func NewRevoke() cli.Command {
	return &Revoke{}
}

// Name returns the command name.
func (c *Revoke) Name() cli.CmdName {
	return CmdNmRevoke
}

// Help prints out the help message for the command.
func (c *Revoke) Help(
	ctx context.Context,
) {
	out.Normf("\nUsage: ")
	out.Boldf("warp revoke [<username_or_token>]\n")
	out.Normf("\n")
	out.Normf("  Revokes write access to a client of the current warp. If no argument is\n")
	out.Normf("  provided, it revokes write access to all connected clients.\n")
	out.Normf("\n")
	out.Normf("Arguments:\n")
	out.Boldf("  username_or_token\n")
	out.Normf("    The username or token of a connected user.\n")
	out.Valuf("    guest_JpJP50EIas9cOfwo goofy\n")
	out.Normf("\n")
	out.Normf("Examples:\n")
	out.Valuf("  warp revoke\n")
	out.Valuf("  warp revoke goofy\n")
	out.Valuf("  warp revoke guest_JpJP50EIas9cOfwo\n")
	out.Normf("\n")
}

// Parse parses the arguments passed to the command.
func (c *Revoke) Parse(
	ctx context.Context,
	args []string,
	flags map[string]string,
) error {
	if len(args) > 0 {
		c.usernameOrToken = args[0]
	}

	if os.Getenv(warp.EnvWarpUnixSocket) == "" {
		return errors.Trace(
			errors.Newf("This command is only available from inside a warp."),
		)
	}

	return nil
}

// Execute the command or return a human-friendly error.
func (c *Revoke) Execute(
	ctx context.Context,
) error {
	args := []string{}

	result, err := cli.RunLocalCommand(ctx, warp.Command{
		Type: warp.CmdTpState,
		Args: []string{},
	})
	if err != nil {
		return errors.Trace(err)
	}

	match := false
	for _, user := range result.State.Users {
		if !user.Hosting {
			if user.Username == c.usernameOrToken ||
				user.Token == c.usernameOrToken {
				match = true
				args = append(args, user.Token)
			}
			if c.usernameOrToken == "" && user.Mode&warp.ModeShellWrite != 0 {
				match = true
				args = append(args, user.Token)
			}
		}
	}

	if c.usernameOrToken != "" && !match {
		return errors.Trace(
			errors.Newf(
				"Username or token not found: %s. Use `warp state` to "+
					"retrieve a list of currently connected warp clients.",
				c.usernameOrToken,
			),
		)
	}

	result, err = cli.RunLocalCommand(ctx, warp.Command{
		Type: warp.CmdTpRevoke,
		Args: args,
	})
	if err != nil {
		return errors.Trace(err)
	}

	OutState(ctx, result.State)

	return nil
}
