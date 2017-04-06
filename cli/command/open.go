package command

import (
	"context"
	"encoding/gob"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"syscall"
	"unsafe"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/hashicorp/yamux"
	"github.com/kr/pty"
	"github.com/spolu/wrp"
	"github.com/spolu/wrp/cli"
	"github.com/spolu/wrp/lib/errors"
	"github.com/spolu/wrp/lib/out"
	"github.com/spolu/wrp/lib/token"
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

	address string
	session string
	user    wrp.User

	username string

	cmd *exec.Cmd
	pty *os.File

	dataC   net.Conn
	stateC  net.Conn
	stateR  *gob.Decoder
	updateC net.Conn
	updateW *gob.Encoder
	hostC   net.Conn
	hostW   *gob.Encoder
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
	out.Boldf("wrp open [<id>]\n")
	out.Normf("\n")
	out.Normf("  Spawns a shared terminal with the provided id. Others can use the id to connect.\n")
	out.Normf("  If no id is provided a random one is generated.\n")
	out.Normf("\n")
	out.Normf("Arguments:\n")
	out.Boldf("  id\n")
	out.Normf("    The id to assign to the newly shared terminal.\n")
	out.Valuf("    spolu-dev\n")
	out.Normf("\n")
	out.Normf("Examples:\n")
	out.Valuf("  wrp open\n")
	out.Valuf("  wrp open spolu-dev\n")
	out.Normf("\n")
}

// Parse parses the arguments passed to the command.
func (c *Open) Parse(
	ctx context.Context,
	args []string,
) error {
	if len(args) == 0 {
		c.session = token.RandStr()
	} else {
		c.session = args[0]
	}

	c.address = wrp.DefaultAddress

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

	c.user = wrp.User{
		Token:   token.New("guest"),
		Secret:  "",
		Session: token.New("session"),
	}

	return nil
}

// Execute the command or return a human-friendly error.
func (c *Open) Execute(
	ctx context.Context,
) error {
	ctx, cancel := context.WithCancel(ctx)

	out.Normf("\n")
	out.Normf("wrp id: ")
	out.Boldf("%s\n", c.session)
	out.Normf("\n")

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
	// Closes stateC, updateC, hostC, dataC, mux and conn.
	defer mux.Close()

	// Setup pty
	c.cmd = exec.Command(c.shell)
	c.pty, err = pty.Start(c.cmd)
	if err != nil {
		return errors.Trace(
			errors.Newf("PTY error: %v", err),
		)
	}
	go func() {
		if err := c.cmd.Wait(); err != nil {
			out.Errof("[Error] Cmd wait error: %v\n", err)
		}
		cancel()
	}()
	// Closes the newly created pty.
	defer c.pty.Close()

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
	// Restores the terminal once we're done.
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

	// Send initial client update.
	if err := c.updateW.Encode(wrp.ClientUpdate{
		Session:  c.session,
		From:     c.user,
		Hosting:  true,
		Username: c.username,
	}); err != nil {
		return errors.Trace(
			errors.Newf("Send client update error: %v", err),
		)
	}

	// Open data channel dataC.
	c.dataC, err = mux.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("Data channel open error: %v", err),
		)
	}

	// Open host channel hostC.
	c.hostC, err = mux.Open()
	if err != nil {
		return errors.Trace(
			errors.Newf("Host channel open error: %v", err),
		)
	}
	c.hostW = gob.NewEncoder(c.hostC)

	// Send initial host update.
	cols, rows, err := terminal.GetSize(stdin)
	if err != nil {
		return errors.Trace(
			errors.Newf("Getsize error: %v", err),
		)
	}

	if err := c.updateW.Encode(wrp.HostUpdate{
		Session:    c.session,
		From:       c.user,
		WindowSize: wrp.Size{Rows: rows, Cols: cols},
	}); err != nil {
		return errors.Trace(
			errors.Newf("Send host update error: %v", err),
		)
	}

	// Main loops.

	// Forward window resizes to pty and stateChannel
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGWINCH)
		for {
			cols, rows, err := terminal.GetSize(stdin)
			if err != nil {
				out.Errof("[Error] Getsize error: %v\n", err)
				break
			}
			if err := Setsize(c.pty, rows, cols); err != nil {
				out.Errof("[Error] Setsize error: %v\n", err)
				break
			}
			if err := syscall.Kill(c.cmd.Process.Pid, syscall.SIGWINCH); err != nil {
				out.Errof("[Error] Sigwinch error: %v\n", err)
				break
			}

			if err := c.hostW.Encode(wrp.HostUpdate{
				Session:    c.session,
				From:       c.user,
				WindowSize: wrp.Size{Rows: rows, Cols: cols},
			}); err != nil {
				out.Errof("[Error] Send host update error: %v\n", err)
				break
			}
			<-ch
		}
		cancel()
	}()

	// Multiplex shell to dataC, Stdout
	go func() {
		cli.Multiplex(ctx, []io.Writer{c.dataC, os.Stdout}, c.pty)
		cancel()
	}()

	// Multiplex dataC to pty
	go func() {
		cli.Multiplex(ctx, []io.Writer{c.pty}, c.dataC)
		cancel()
	}()

	// Multiplex Stdin to pty
	go func() {
		cli.Multiplex(ctx, []io.Writer{c.pty}, os.Stdin)
		cancel()
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
