package command

import (
	"context"

	"github.com/spolu/wrp/cli"
	"github.com/spolu/wrp/lib/out"
)

const (
	// CmdNmConnect is the command name.
	CmdNmConnect cli.CmdName = "connect"
)

func init() {
	cli.Registrar[CmdNmConnect] = NewConnect
}

// Connect spawns a new shared terminal.
type Connect struct {
}

// NewConnect constructs and initializes the command.
func NewConnect() cli.Command {
	return &Connect{}
}

// Name returns the command name.
func (c *Connect) Name() cli.CmdName {
	return CmdNmConnect
}

// Help prints out the help message for the command.
func (c *Connect) Help(
	ctx context.Context,
) {
	out.Normf("\nUsage: ")
	out.Boldf("wrp connect <id>\n")
	out.Normf("\n")
	out.Normf("  Connects to the shared terminal denoted by `id`. If possible wrp will\n")
	out.Normf("  attempt to resize the window it is running in to the size of the shared\n")
	out.Normf("  terminal.\n")
	out.Normf("\n")
	out.Normf("Arguments:\n")
	out.Boldf("  id\n")
	out.Normf("    The id of the shared terminal to connect to.\n")
	out.Valuf("    ae7fd234abe2\n")
	out.Normf("\n")
	out.Normf("Examples:\n")
	out.Valuf("  wrp connect ae7fd234abe2\n")
	out.Normf("\n")
}

// Parse parses the arguments passed to the command.
func (c *Connect) Parse(
	ctx context.Context,
	args []string,
) error {
	return nil
}

// Execute the command or return a human-friendly error.
func (c *Connect) Execute(
	ctx context.Context,
) error {

	return nil
}
