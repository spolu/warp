package command

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/user"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/spolu/warp"
	"github.com/spolu/warp/cli"
	"github.com/spolu/warp/lib/errors"
	"github.com/spolu/warp/lib/out"
	"github.com/spolu/warp/lib/plex"
	"github.com/spolu/warp/lib/token"
)

const (
	// CmdNmConnect is the command name.
	CmdNmConnect cli.CmdName = "connect"
)

func init() {
	cli.Registrar[CmdNmConnect] = NewConnect
}

// Connect connects to a shared terminal.
type Connect struct {
	address  string
	warp     string
	session  warp.Session
	username string

	ss *cli.Session
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
	out.Boldf("warp connect <id>\n")
	out.Normf("\n")
	out.Normf("  Connects to the shared terminal denoted by `id`. If possible warp will\n")
	out.Normf("  attempt to resize the window it is running in to the size of the shared\n")
	out.Normf("  terminal.\n")
	out.Normf("\n")
	out.Normf("Arguments:\n")
	out.Boldf("  id\n")
	out.Normf("    The id of the shared terminal to connect to.\n")
	out.Valuf("    ae7fd234abe2\n")
	out.Normf("\n")
	out.Normf("Examples:\n")
	out.Valuf("  warp connect ae7fd234abe2\n")
	out.Normf("\n")
}

// Parse parses the arguments passed to the command.
func (c *Connect) Parse(
	ctx context.Context,
	args []string,
) error {
	if len(args) == 0 {
		return errors.Trace(
			errors.Newf("Warp id required."),
		)
	} else {
		c.warp = args[0]
	}

	if !cli.WarpRegexp.MatchString(c.warp) {
		return errors.Trace(
			errors.Newf("Malformed warp ID: %s", c.warp),
		)
	}

	c.address = warp.DefaultAddress
	if os.Getenv("WARPD_ADDRESS") != "" {
		c.address = os.Getenv("WARPD_ADDRESS")
	}

	user, err := user.Current()
	if err != nil {
		return errors.Trace(
			errors.Newf("Failed to retrieve current user: %v.", err),
		)
	}
	c.username = user.Username

	c.session = warp.Session{
		Token:  token.New("session"),
		User:   token.New("guest"),
		Secret: token.RandStr(),
	}

	return nil
}

// Execute the command or return a human-friendly error.
func (c *Connect) Execute(
	ctx context.Context,
) error {
	ctx, cancel := context.WithCancel(ctx)

	conn, err := net.Dial("tcp", c.address)
	if err != nil {
		return errors.Trace(
			errors.Newf("Connection to warpd failed: %v.", err),
		)
	}

	c.ss, err = cli.NewSession(
		ctx,
		c.session,
		c.warp,
		warp.SsTpShellClient,
		c.username,
		cancel,
		conn,
	)
	if err != nil {
		return errors.Trace(err)
	}
	// Close and reclaims all session related state.
	defer c.ss.TearDown()

	out.Normf("Connected to warp: ")
	out.Boldf("%s\n", c.warp)

	// Setup local term.
	stdin := int(os.Stdin.Fd())
	if !terminal.IsTerminal(stdin) {
		return errors.Trace(
			errors.Newf("Not running in a terminal."),
		)
	}

	old, err := terminal.MakeRaw(stdin)
	if err != nil {
		return errors.Trace(
			errors.Newf("Unable to put terminal in raw mode: %v.", err),
		)
	}
	// Restors the terminal once we're done.
	defer terminal.Restore(stdin, old)

	// Main loops.

	// Listen for state updates.
	go func() {
		for {
			if st, err := c.ss.DecodeState(ctx); err != nil {
				break
			} else {
				if err := c.ss.State().Update(*st, false); err != nil {
					break
				}
				// Update the terminal size.
				fmt.Printf("\033[8;%d;%dt", st.WindowSize.Rows, st.WindowSize.Cols)
			}

			select {
			case <-ctx.Done():
				break
			default:
			}
		}
		c.ss.TearDown()
	}()

	// Listen for errors.
	go func() {
		if e, err := c.ss.DecodeError(ctx); err == nil {
			c.ss.ErrorOut(
				fmt.Sprintf("Received %s", e.Code),
				errors.Newf(e.Message),
			)
		}
		c.ss.TearDown()
	}()

	// Multiplex Stdin to dataC.
	go func() {
		plex.Run(ctx, func(data []byte) {
			c.ss.DataC().Write(data)
		}, os.Stdin)
		c.ss.TearDown()
	}()

	// Multiplex dataC to Stdout.
	go func() {
		plex.Run(ctx, func(data []byte) {
			os.Stdout.Write(data)
		}, c.ss.DataC())
		c.ss.TearDown()
	}()

	// Wait for cancellation to return and clean up everything.
	<-ctx.Done()

	return nil
}
