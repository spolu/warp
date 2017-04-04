package command

import (
	"context"

	"github.com/spolu/wrp/cli"
	"github.com/spolu/wrp/lib/out"
)

const (
	// CmdNmHelp is the command name.
	CmdNmHelp cli.CmdName = "help"
)

func init() {
	cli.Registrar[CmdNmHelp] = NewHelp
}

// Help a user
type Help struct {
	Command cli.Command
}

// NewHelp constructs and initializes the command.
func NewHelp() cli.Command {
	return &Help{}
}

// Name returns the command name.
func (c *Help) Name() cli.CmdName {
	return CmdNmHelp
}

// Help prints out the help message for the command.
func (c *Help) Help(
	ctx context.Context,
) {
	out.Normf("\nUsage: ")
	out.Boldf("wrp <command> [<args> ...]\n")
	out.Normf("\n")
	out.Normf("  Terminal sharing directly from localhost.\n")
	out.Normf("\n")
	out.Normf("Commands:\n")

	out.Boldf("  help <command>\n")
	out.Normf("    Show help for a specific command.\n")
	out.Valuf("    wrp help open\n")
	out.Normf("\n")

	out.Boldf("  open\n")
	out.Normf("    Creates a new wrp.\n")
	out.Valuf("    wrp open\n")
	out.Normf("\n")
}

// Parse parses the arguments passed to the command.
func (c *Help) Parse(
	ctx context.Context,
	args []string,
) error {
	if len(args) == 0 {
		c.Command = NewHelp()
	} else {
		if r, ok := cli.Registrar[cli.CmdName(args[0])]; !ok {
			c.Command = NewHelp()
		} else {
			c.Command = r()
		}
	}
	return nil
}

// Execute the command or return a human-friendly error.
func (c *Help) Execute(
	ctx context.Context,
) error {
	c.Command.Help(ctx)
	return nil
}
