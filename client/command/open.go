package command

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"sync"
	"syscall"
	"time"
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
	noTLS       bool
	insecureTLS bool
	shell       *cli.Shell

	address  string
	warp     string
	session  warp.Session
	username string

	cmd *exec.Cmd
	pty *os.File
	srv *cli.Srv

	mutex *sync.Mutex
	size  warp.Size
	ss    *cli.Session

	errC   chan error
	initC  chan struct{}
	inited bool
}

// NewOpen constructs and initializes the command.
func NewOpen() cli.Command {
	return &Open{
		mutex: &sync.Mutex{},
	}
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
	flags map[string]string,
) error {
	if len(args) == 0 {
		c.warp = token.RandStr()
	} else {
		c.warp = args[0]
	}

	if !warp.WarpRegexp.MatchString(c.warp) {
		return errors.Trace(
			errors.Newf("Malformed warp ID: %s", c.warp),
		)
	}

	if _, ok := flags["insecure_tls"]; ok ||
		os.Getenv("WARPD_INSECURE_TLS") != "" {
		c.insecureTLS = true
	}
	if _, ok := flags["no_tls"]; ok ||
		os.Getenv("WARPD_NO_TLS") != "" {
		c.noTLS = true
	}

	c.address = warp.DefaultAddress
	if os.Getenv("WARPD_ADDRESS") != "" {
		c.address = os.Getenv("WARPD_ADDRESS")
	}
	if os.Getenv("WARPD_NO_TLS") != "" {
		c.noTLS = true
	}

	s, err := cli.DetectShell(ctx)
	if err != nil {
		return errors.Trace(
			errors.Newf("Error detecting shell: %v", err),
		)
	}
	c.shell = s

	user, err := user.Current()
	if err != nil {
		return errors.Trace(
			errors.Newf("Error retrieving current user: %v", err),
		)
	}
	c.username = user.Username

	// Sets the BASH prompt
	// prompt := fmt.Sprintf(
	// 	"\\[\033[01;31m\\][warp:%s]\\[\033[00m\\] "+
	// 		"\\[\033[01;34m\\]\\W\\[\033[00m\\]\\$ ",
	// 	c.warp,
	// )
	// os.Setenv("PS1", prompt)
	// os.Setenv("PROMPT", prompt)

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

// HostSession accessor is used by the local server to retrieve the current
// host session. The host session can be nil if the warp is currently
// disconnected from warpd. It is protected by a lock as the host session is
// set or unset by the ConnLoop.
func (c *Open) HostSession() *cli.Session {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.ss
}

// WindowSize returns the current window size for the host terminal.
func (c *Open) WindowSize() warp.Size {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.size
}

// Warp returns the warp name
func (c *Open) Warp() string {
	return c.warp
}

// Execute the command or return a human-friendly error.
func (c *Open) Execute(
	ctx context.Context,
) error {
	ctx, cancel := context.WithCancel(ctx)

	// Build the local command server.
	c.srv = cli.NewSrv(ctx, c.warp)

	// Setup local term.
	stdin := int(os.Stdin.Fd())
	if !terminal.IsTerminal(stdin) {
		return errors.Trace(
			errors.Newf("Not running in a terminal."),
		)
	}

	// Store initial size of the terminal.
	cols, rows, err := terminal.GetSize(stdin)
	if err != nil {
		return errors.Trace(
			errors.Newf("Failed to retrieve the terminal size: %v.", err),
		)
	}
	c.mutex.Lock()
	c.size = warp.Size{Rows: rows, Cols: cols}
	c.mutex.Unlock()

	// Display open message
	out.Normf("Opened warp: ")
	out.Valuf("%s\n", c.warp)

	// Make the terminal raw.
	old, err := terminal.MakeRaw(stdin)
	if err != nil {
		return errors.Trace(
			errors.Newf("Unable to put terminal in raw mode: %v.", err),
		)
	}
	// Restores the terminal once we're done.
	defer func() {
		terminal.Restore(stdin, old)
		// Let's attempt to clean things up with a newline.
		fmt.Printf("\n")
	}()

	// Start shell.
	c.cmd = exec.Command(c.shell.Command, "-l")

	// Set the warp env variable for the shell.
	env := os.Environ()
	env = append(
		env, fmt.Sprintf("%s=%s", warp.EnvWarp, c.warp),
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
		cancel()
	}()

	// Main loops.

	// c.errC is used to capture user facing errors generated from the
	// goroutines.
	c.errC = make(chan error)

	// c.initC is used to signal that the warp was able to properly init
	// without error. It is used to start the local server after we got a
	// chance to receive any error from warpd.
	c.initC = make(chan struct{})

	// Wait for an user facing error on the c.errC channel.
	var userErr error
	go func() {
		userErr = <-c.errC
		cancel()
	}()

	// Launch the connection loop.
	go func() {
		c.ConnLoop(ctx)
		// Errors are sent to the errC, no need to cancel.
	}()

	// Launch the local command server.
	go func() {
		<-c.initC
		c.inited = true
		c.srv.Run(ctx)
		cancel()
	}()

	// Forward window resizes to pty and updateC.
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGWINCH)
		for {
			ss := c.HostSession()
			if ss != nil && ss.TornDown() {
				break
			}
			cols, rows, err := terminal.GetSize(stdin)
			if err != nil {
				c.errC <- errors.Newf(
					"Failed to retrieve the terminal size: %v", err,
				)
				break
			}
			if err := Setsize(c.pty, rows, cols); err != nil {
				c.errC <- errors.Newf(
					"Failed to set the pty size", err,
				)
				break
			}
			if err := syscall.Kill(
				c.cmd.Process.Pid, syscall.SIGWINCH,
			); err != nil {
				c.errC <- errors.Newf(
					"Failed to signal SIGWINCH", err,
				)
				break
			}

			c.mutex.Lock()
			c.size = warp.Size{Rows: rows, Cols: cols}
			c.mutex.Unlock()

			ss = c.HostSession()
			if ss != nil {
				// Send an update and ignore errors.
				ss.SendHostUpdate(ctx, warp.HostUpdate{
					Warp:       c.warp,
					From:       c.session,
					WindowSize: c.size,
				})
			}

			<-ch
		}
		cancel()
	}()

	// Multiplex shell to dataC, Stdout.
	go func() {
		plex.Run(ctx, func(data []byte) {
			os.Stdout.Write(data)
			ss := c.HostSession()
			if ss != nil {
				ss.WriteDataC(data)
			}
		}, c.pty)
		cancel()
	}()

	// Multiplex Stdin to pty.
	go func() {
		plex.Run(ctx, func(data []byte) {
			c.pty.Write(data)
		}, os.Stdin)
		cancel()
	}()

	<-ctx.Done()

	return errors.Trace(userErr)
}

// ReconnectLoop handles reconnecting the host to warpd. Each time the
// connection drops, the associated Session is destroyed and another one is
// created as a reconnection is attempted.
// Errors are returned during the first iteration of the reconnect loop. On
// subsequent reconnect, only warpd generated errors are returned.
func (c *Open) ConnLoop(
	ctx context.Context,
) {
	first := true
CONNLOOP:
	for {
		var conn net.Conn
		var err error

		if c.noTLS {
			conn, err = net.Dial("tcp", c.address)
			if err != nil {
				if first {
					c.errC <- errors.Trace(
						errors.Newf("Connection error: %v", err),
					)
					break
				}
				// Silentluy ignore and attempt a reconnect 500ms after.
				time.Sleep(500 * time.Millisecond)
				continue
			}
		} else {
			tlsConfig := &tls.Config{
				InsecureSkipVerify: c.insecureTLS,
			}

			conn, err = tls.Dial("tcp", c.address, tlsConfig)
			if err != nil {
				if first {
					c.errC <- errors.Trace(
						errors.Newf("Connection error: %v", err),
					)
					break
				}
				// Silentluy ignore and attempt a reconnect.
				time.Sleep(500 * time.Millisecond)
				continue
			}
		}
		defer conn.Close()

		c.ManageSession(ctx, conn, !first)
		first = false

		select {
		case <-ctx.Done():
			break CONNLOOP
		default:
		}
	}
}

// ManageSession creates an manage a session. It
func (c *Open) ManageSession(
	ctx context.Context,
	conn net.Conn,
	warpdErrOnly bool,
) {
	// This ctx can be canceled by the session or its parent context.
	ctx, cancel := context.WithCancel(ctx)

	ss, err := cli.NewSession(
		ctx, c.session, c.warp, warp.SsTpHost, c.username, cancel, conn,
	)
	if err != nil {
		if !warpdErrOnly {
			c.errC <- errors.Newf(
				"Failed to open session to warpd: %s", err,
			)
		}
		return
	}
	// Close and reclaims all session related state.
	defer ss.TearDown()

	// Listen for errors.
	go func() {
		if e, err := ss.DecodeError(ctx); err == nil {
			c.errC <- errors.Newf(
				"Received %s: %s", e.Code, e.Message,
			)
		}
		cancel()
	}()

	if err := ss.SendHostUpdate(ctx, warp.HostUpdate{
		Warp:       c.warp,
		From:       c.session,
		WindowSize: c.WindowSize(),
	}); err != nil {
		if !warpdErrOnly {
			c.errC <- errors.Trace(
				errors.Newf("Failed to send initial host update: %v.", err),
			)
		}
		return
	}

	// Wait for a first state update from warpd.
	if st, err := ss.DecodeState(ctx); err != nil {
		// Let's not print any error here as we should have received an error
		// from the server.
		return
	} else {
		if err := ss.UpdateState(*st, true); err != nil {
			if !warpdErrOnly {
				c.errC <- errors.Trace(
					errors.Newf(
						"Failed to apply initial state update: %v.", err,
					),
				)
			}
			return
		} else {
			c.mutex.Lock()
			inited := c.inited
			c.mutex.Unlock()
			if !inited {
				c.initC <- struct{}{}
			}
		}
	}

	// The host session is ready
	c.mutex.Lock()
	c.ss = ss
	c.srv.SetSession(ctx, ss)
	c.mutex.Unlock()

	// Main loops

	// Listen for state updates.
	go func() {
	STATELOOP:
		for {
			if st, err := ss.DecodeState(ctx); err != nil {
				break
			} else {
				if err := ss.UpdateState(*st, true); err != nil {
					break
				}
			}
			select {
			case <-ctx.Done():
				break STATELOOP
			default:
			}
		}
		cancel()
	}()

	// Multiplex dataC to pty.
	go func() {
		plex.Run(ctx, func(data []byte) {
			if ss.HostCanReceiveWrite() {
				c.pty.Write(data)
			}
		}, ss.DataC())
		ss.TearDown()
	}()

	<-ctx.Done()
	ss.TearDown()

	c.mutex.Lock()
	c.ss = nil
	c.srv.SetSession(ctx, nil)
	c.mutex.Unlock()
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
