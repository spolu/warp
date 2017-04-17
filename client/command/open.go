package command

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"syscall"
	"unsafe"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/kr/pty"
	"github.com/spolu/warp"
	"github.com/spolu/warp/client"
	"github.com/spolu/warp/lib/errors"
	"github.com/spolu/warp/lib/out"
	"github.com/spolu/warp/lib/plex"
	"github.com/spolu/warp/lib/token"
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
	shell string

	address  string
	warp     string
	session  warp.Session
	username string

	cmd *exec.Cmd
	pty *os.File
	ss  *cli.Session
	srv *cli.Srv
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
	out.Boldf("warp open [<id>]\n")
	out.Normf("\n")
	out.Normf("  Creates a new warp with the specified ID and starts sharing your terminal\n")
	out.Normf("  (read-only). If no ID is provided a (cryptographically secure) random one is\n")
	out.Normf("  generated.\n")
	out.Normf("\n")
	out.Normf("  Anyone can then connect to you warp using the ")
	out.Boldf("connect")
	out.Normf(" command.\n")
	out.Normf("\n")
	out.Normf("Arguments:\n")
	out.Boldf("  id\n")
	out.Normf("    The ID to assign to the new warp.\n")
	out.Valuf("    goofy-dev\n")
	out.Normf("\n")
	out.Normf("Examples:\n")
	out.Valuf("  warp open\n")
	out.Valuf("  warp open goofy-dev\n")
	out.Normf("\n")
}

// Parse parses the arguments passed to the command.
func (c *Open) Parse(
	ctx context.Context,
	args []string,
) error {
	if len(args) == 0 {
		c.warp = token.RandStr()
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

	c.shell = "/bin/bash"
	if os.Getenv("SHELL") != "" {
		c.shell = os.Getenv("SHELL")
	}

	user, err := user.Current()
	if err != nil {
		return errors.Trace(
			errors.Newf("Error retrieving current user: %v", err),
		)
	}
	c.username = user.Username

	// Sets the BASH prompt
	prompt := fmt.Sprintf(
		"\\[\033[01;31m\\][warp:%s]\\[\033[00m\\] "+
			"\\[\033[01;34m\\]\\W\\[\033[00m\\]\\$ ",
		c.warp,
	)
	os.Setenv("PS1", prompt)
	os.Setenv("PROMPT", prompt)

	c.session = warp.Session{
		Token:  token.New("session"),
		User:   token.New("guest"),
		Secret: token.RandStr(),
	}

	return nil
}

// Execute the command or return a human-friendly error.
func (c *Open) Execute(
	ctx context.Context,
) error {
	ctx, cancel := context.WithCancel(ctx)

	conn, err := net.Dial("tcp", c.address)
	if err != nil {
		return errors.Trace(
			errors.Newf("Connection error: %v", err),
		)
	}

	c.ss, err = cli.NewSession(
		ctx, c.session, c.warp, warp.SsTpHost, c.username, cancel, conn,
	)
	if err != nil {
		return errors.Trace(err)
	}
	// Close and reclaims all session related state.
	defer c.ss.TearDown()

	// Build the local command server.
	c.srv = cli.NewSrv(ctx, c.ss)

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

	// Setup local term.
	stdin := int(os.Stdin.Fd())
	if !terminal.IsTerminal(stdin) {
		return errors.Trace(
			errors.Newf("Not running in a terminal."),
		)
	}

	// Send initial host update.
	cols, rows, err := terminal.GetSize(stdin)
	if err != nil {
		return errors.Trace(
			errors.Newf("Failed to retrieve the terminal size: %v.", err),
		)
	}

	if err := c.ss.SendHostUpdate(ctx, warp.HostUpdate{
		Warp:       c.warp,
		From:       c.session,
		WindowSize: warp.Size{Rows: rows, Cols: cols},
	}); err != nil {
		return errors.Trace(
			errors.Newf("Failed to send initial host update: %v.", err),
		)
	}

	// Wait for a first state update from warpd.
	if st, err := c.ss.DecodeState(ctx); err != nil {
		// Let's not print any error here as we should have received an error
		// from the server.
		return nil
	} else {
		if err := c.ss.State().Update(*st, true); err != nil {
			return errors.Trace(
				errors.Newf("Failed to apply initial state update: %v.", err),
			)
		}
	}

	out.Normf("Opened warp: ")
	out.Valuf("%s\n", c.warp)

	old, err := terminal.MakeRaw(stdin)
	if err != nil {
		return errors.Trace(
			errors.Newf("Unable to put terminal in raw mode: %v.", err),
		)
	}
	// Restores the terminal once we're done.
	defer terminal.Restore(stdin, old)

	// Start shell.
	c.cmd = exec.Command(c.shell)

	// Set the warp env variables for the shell.
	env := os.Environ()
	env = append(
		env, fmt.Sprintf("%s=%s", warp.EnvWarp, c.warp),
	)
	env = append(
		env, fmt.Sprintf("%s=%s", warp.EnvWarpUnixSocket, c.srv.Path()),
	)
	c.cmd.Env = env

	// Setup pty.
	c.pty, err = pty.Start(c.cmd)
	if err != nil {
		return errors.Trace(
			errors.Newf("Failed to create pty: %v.", err),
		)
	}
	go func() {
		c.cmd.Wait()
		c.ss.TearDown()
	}()

	// Closes the newly created pty.
	defer c.pty.Close()

	// Main loops.

	// Listen for state updates.
	go func() {
		for {
			if st, err := c.ss.DecodeState(ctx); err != nil {
				// Do not print that error as it will be triggered when
				// disconnecting.
				break
			} else {
				if err := c.ss.State().Update(*st, true); err != nil {
					c.ss.ErrorOut("Failed to apply state update", err)
					break
				}
			}

			select {
			case <-ctx.Done():
				break
			default:
			}
		}
		c.ss.TearDown()
	}()

	// Forward window resizes to pty and updateC.
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGWINCH)
		for {
			if c.ss.TornDown() {
				break
			}
			cols, rows, err := terminal.GetSize(stdin)
			if err != nil {
				c.ss.ErrorOut("Failed to retrieve the terminal size", err)
				break
			}
			if err := Setsize(c.pty, rows, cols); err != nil {
				c.ss.ErrorOut("Failed to set pty size", err)
				break
			}
			if err := syscall.Kill(
				c.cmd.Process.Pid, syscall.SIGWINCH,
			); err != nil {
				c.ss.ErrorOut("Failed to signal SIGWINCH", err)
				break
			}

			if err := c.ss.SendHostUpdate(ctx, warp.HostUpdate{
				Warp:       c.warp,
				From:       c.session,
				WindowSize: warp.Size{Rows: rows, Cols: cols},
			}); err != nil {
				c.ss.ErrorOut("Failed to send host update", err)
				break
			}

			<-ch
		}
		c.ss.TearDown()
	}()

	// Launch the local command server.
	go func() {
		c.srv.Run(ctx)
	}()

	// Multiplex shell to dataC, Stdout.
	go func() {
		plex.Run(ctx, func(data []byte) {
			os.Stdout.Write(data)
			c.ss.DataC().Write(data)
		}, c.pty)
		c.ss.TearDown()
	}()

	// Multiplex dataC to pty.
	go func() {
		plex.Run(ctx, func(data []byte) {
			if c.ss.State().HostCanReceiveWrite() {
				c.pty.Write(data)
			}
		}, c.ss.DataC())
		c.ss.TearDown()
	}()

	// Multiplex Stdin to pty.
	go func() {
		plex.Run(ctx, func(data []byte) {
			c.pty.Write(data)
		}, os.Stdin)
		c.ss.TearDown()
	}()

	<-ctx.Done()

	return nil
}

type winsize struct {
	ws_row    uint16
	ws_col    uint16
	ws_xpixel uint16
	ws_ypixel uint16
}

func Setsize(f *os.File, rows, cols int) error {
	ws := winsize{ws_row: uint16(rows), ws_col: uint16(cols)}
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}
