package command

import (
	"context"

	"github.com/spolu/warp"
	"github.com/spolu/warp/cli"
	"github.com/spolu/warp/lib/out"
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
	out.Normf("\n")
	out.Normf(" __      ____ _ _ __ _ __  \n")
	out.Normf(" \\ \\ /\\ / / _` | '__| '_ \\ \n")
	out.Normf("  \\ V  V / (_| | |  | |_) |\n")
	out.Normf("   \\_/\\_/ \\__,_|_|  | .__/ \n")
	out.Normf("                    |_|    ")
	out.Boldf("  v%s\n", warp.Version)
	out.Normf("\n")
	out.Normf("Usage: ")
	out.Boldf("warp <command> [<args> ...]\n")
	out.Normf("\n")
	out.Normf("  Instant terminal sharing directly from your machine.\n")
	out.Normf("\n")
	out.Normf("Commands:\n")
	out.Boldf("  help <command>\n")
	out.Normf("    Show help for a specific command.\n")
	out.Valuf("    warp help open\n")
	out.Normf("\n")

	out.Boldf("  open\n")
	out.Normf("    Creates a new warp.\n")
	out.Valuf("    warp open\n")
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
