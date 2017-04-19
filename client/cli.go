package cli

import (
	"context"
	"regexp"
	"strings"

	"github.com/spolu/warp/lib/errors"
)

// CmdName represents a command name.
type CmdName string

// Command is the interface for a cli command.
type Command interface {
	// Name returns the command name.
	Name() CmdName

	// Help prints out the help message for the command.
	Help(context.Context)

	// Parse the arguments and flags passed to the command.
	Parse(context.Context, []string, map[string]string) error

	// Execute the command or return a human-friendly error.
	Execute(context.Context) error
}

// Registrar is used to register command generators within the module.
var Registrar = map[CmdName](func() Command){}

// Cli represents a cli instance.
type Cli struct {
	Ctx   context.Context
	Flags map[string]string
	Args  []string
}

// flagFilterRegexp filters out flags from arguments.
var flagFilterRegexp = regexp.MustCompile("^-+")

// New initializes a new Cli by parsing the passed arguments.
func New(
	argv []string,
) (*Cli, error) {
	ctx := context.Background()

	args := []string{}
	flags := map[string]string{}

	for _, a := range argv {
		if flagFilterRegexp.MatchString(a) {
			a = strings.Trim(a, "-")
			s := strings.Split(a, "=")
			if len(s) == 2 {
				flags[s[0]] = s[1]
			} else if len(s) == 1 {
				flags[s[0]] = "true"
			}
		} else {
			args = append(args, strings.TrimSpace(a))
		}
	}

	return &Cli{
		Ctx:   ctx,
		Args:  args,
		Flags: flags,
	}, nil
}

// Run the cli.
func (c *Cli) Run() error {
	if len(c.Args) == 0 {
		c.Args = append(c.Args, "help")
	}

	var command Command
	cmd, args := c.Args[0], c.Args[1:]
	if r, ok := Registrar[CmdName(cmd)]; !ok {
		command = Registrar[CmdName("help")]()
	} else {
		command = r()
	}

	err := command.Parse(c.Ctx, args, c.Flags)
	if err != nil {
		command.Help(c.Ctx)
		return errors.Trace(err)
	}

	err = command.Execute(c.Ctx)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}
