package command

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/user"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/spolu/warp"
	"github.com/spolu/warp/client"
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
	noTLS       bool
	insecureTLS bool

	address  string
	warp     string
	session  warp.Session
	username string

	ss *cli.Session

	errC chan error
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
	out.Normf("  Connects to an existing warp (read-only).\n")
	out.Normf("\n")
	out.Normf("  If possible warp will attempt to resize the window it is running in to the\n")
	out.Normf("  size of the host terminal.\n")
	out.Normf("\n")
	out.Normf("Arguments:\n")
	out.Boldf("  id\n")
	out.Normf("    The ID of the warp to connect to.\n")
	out.Valuf("    DJc3hR0PoyFmQIIY goofy-dev\n")
	out.Normf("\n")
	out.Normf("Examples:\n")
	out.Valuf("    warp connect goofy-dev\n")
	out.Valuf("    warp connect DJc3hR0PoyFmQIIY\n")
	out.Normf("\n")
}

// Parse parses the arguments passed to the command.
func (c *Connect) Parse(
	ctx context.Context,
	args []string,
	flags map[string]string,
) error {
	if len(args) == 0 {
		return errors.Trace(
			errors.Newf("Warp ID required."),
		)
	} else {
		c.warp = args[0]
	}

	if !warp.WarpRegexp.MatchString(c.warp) {
		return errors.Trace(
			errors.Newf("Malformed warp ID: %s", c.warp),
		)
	}

	if _, ok := flags["insecure_tls"]; ok {
		c.insecureTLS = true
	}
	if _, ok := flags["no_tls"]; ok {
		c.noTLS = true
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

	config, err := cli.RetrieveOrGenerateConfig(ctx)
	if err != nil {
		return errors.Trace(
			errors.Newf("Error retrieving or generating config: %v", err),
		)
	}

	c.session = warp.Session{
		Token:  token.New("session"),
		User:   config.Credentials.User,
		Secret: config.Credentials.Secret,
	}

	return nil
}

// Execute the command or return a human-friendly error.
func (c *Connect) Execute(
	ctx context.Context,
) error {
	ctx, cancel := context.WithCancel(ctx)

	var conn net.Conn
	var err error

	if c.noTLS {
		conn, err = net.Dial("tcp", c.address)
		if err != nil {
			return errors.Trace(
				errors.Newf("Connection to warpd failed: %v.", err),
			)
		}
	} else {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: c.insecureTLS,
		}

		conn, err = tls.Dial("tcp", c.address, tlsConfig)
		if err != nil {
			return errors.Trace(
				errors.Newf("Connection to warpd failed: %v.", err),
			)
		}
	}
	defer conn.Close()

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
	out.Valuf("%s\n", c.warp)

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

	// c.errC is used to capture user facing errors generated from the
	// goroutines.
	c.errC = make(chan error)

	// Wait for an user facing error on the c.errC channel.
	var userErr error
	go func() {
		userErr = <-c.errC
		cancel()
	}()

	// Listen for state updates.
	go func() {
		for {
			if st, err := c.ss.DecodeState(ctx); err != nil {
				break
			} else {
				if err := c.ss.UpdateState(*st, false); err != nil {
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
		cancel()
	}()

	// Listen for errors.
	go func() {
		if e, err := c.ss.DecodeError(ctx); err == nil {
			c.errC <- errors.Newf(
				"Received %s: %s", e.Code, e.Message,
			)
		}
	}()

	// Multiplex Stdin to dataC.
	go func() {
		plex.Run(ctx, func(data []byte) {
			c.ss.DataC().Write(data)
		}, os.Stdin)
		cancel()
	}()

	// Multiplex dataC to Stdout.
	go func() {
		plex.Run(ctx, func(data []byte) {
			os.Stdout.Write(data)
		}, c.ss.DataC())
		c.errC <- errors.Newf(
			"Lost connection to warpd. You can attempt to reconnect once you " +
				"regain connetivity.",
		)
	}()

	// Wait for cancellation to return and clean up everything.
	<-ctx.Done()

	return userErr
}
