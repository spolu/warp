package command

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/hashicorp/yamux"
	"github.com/spolu/wrp"
	"github.com/spolu/wrp/cli"
	"github.com/spolu/wrp/lib/errors"
	"github.com/spolu/wrp/lib/out"
	"github.com/spolu/wrp/lib/token"
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
	address string
	warp    string
	session wrp.Session

	username string

	dataC   net.Conn
	stateC  net.Conn
	stateR  *gob.Decoder
	updateC net.Conn
	updateW *gob.Encoder
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
	if len(args) == 0 {
		return errors.Trace(
			errors.Newf("Warp id required."),
		)
	} else {
		c.warp = args[0]
	}

	c.address = wrp.DefaultAddress
	if os.Getenv("WRPD_ADDRESS") != "" {
		c.address = os.Getenv("WRPD_ADDRESS")
	}

	user, err := user.Current()
	if err != nil {
		return errors.Trace(
			errors.Newf("Error retrieving current user: %v", err),
		)
	}
	c.username = user.Username

	c.session = wrp.Session{
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
			errors.Newf("Connection error: %v", err),
		)
	}

	mux, err := yamux.Client(conn, nil)
	if err != nil {
		return errors.Trace(
			errors.Newf("Session error: %v", err),
		)
	}
	// Closes stateC, updateC, dataC, mux and conn.
	defer mux.Close()

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
			errors.Newf("Unable to make terminal raw: %v", err),
		)
	}
	// Restors the terminal once we're done.
	defer terminal.Restore(stdin, old)

	// Opens state channel stateC.
	c.stateC, err = mux.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("State channel open error: %v", err),
		)
	}
	c.stateR = gob.NewDecoder(c.stateC)

	// Open update channel updateC.
	c.updateC, err = mux.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("Update channel open error: %v", err),
		)
	}
	c.updateW = gob.NewEncoder(c.updateC)

	// Send initial SessionHello.
	if err := c.updateW.Encode(wrp.SessionHello{
		Warp:     c.warp,
		From:     c.session,
		Type:     wrp.SsTpShellClient,
		Username: c.username,
	}); err != nil {
		return errors.Trace(
			errors.Newf("Send hello error: %v", err),
		)
	}

	// Open data channel dataC.
	c.dataC, err = mux.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("Data channel open error: %v", err),
		)
	}

	// Main loops.

	// Listen for state updates.
	go func() {
		for {
			var st wrp.State
			if err := c.stateR.Decode(&st); err != nil {
				out.Errof("[Error] State channel decode error: %v\n", err)
				break
			}
			// Update the terminal size.
			fmt.Printf("\033[8;%d;%dt", st.WindowSize.Rows, st.WindowSize.Cols)

			select {
			case <-ctx.Done():
				break
			default:
			}
		}
		cancel()
	}()

	// Multiplex Stdin to dataC.
	go func() {
		cli.Multiplex(ctx, []io.Writer{c.dataC}, os.Stdin)
		cancel()
	}()

	// Multiplex dataC to Stdout.
	go func() {
		cli.Multiplex(ctx, []io.Writer{os.Stdout}, c.dataC)
		cancel()
	}()

	// Wait for cancellation to return and clean up everything.
	<-ctx.Done()

	return nil
}
