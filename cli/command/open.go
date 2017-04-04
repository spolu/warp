package command

import (
	"context"

	"github.com/spolu/wrp/cli"
	"github.com/spolu/wrp/lib/out"
)

const (
	// CmdNmOpen is the command name.
	CmdNmOpen cli.CmdName = "open"
)

func init() {
	cli.Registrar[CmdNmOpen] = NewOpen
}

// Open spawns a new shared terminal.
type Open struct {
}

// NewOpen constructs and initializes the command.
func NewOpen() cli.Command {
	return &Open{}
}

// Name returns the command name.
func (c *Open) Name() cli.CmdName {
	return CmdNmOpen
}

// Help prints out the help message for the command.
func (c *Open) Help(
	ctx context.Context,
) {
	out.Normf("\nUsage: ")
	out.Boldf("wrp open\n")
	out.Normf("\n")
	out.Normf("  Spawns a shared terminal and assigns a generated ID others can use to\n")
	out.Normf("  connect. THe ID is echoed before the new terminal is spawned.\n")
	out.Normf("\n")
	out.Normf("Examples:\n")
	out.Valuf("  wrp open\n")
	out.Normf("\n")
}

// Parse parses the arguments passed to the command.
func (c *Open) Parse(
	ctx context.Context,
	args []string,
) error {
	return nil
}

// Execute the command or return a human-friendly error.
func (c *Open) Execute(
	ctx context.Context,
) error {

	return nil
}
