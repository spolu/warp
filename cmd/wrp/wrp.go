package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/hashicorp/yamux"
	"github.com/kr/pty"
	"github.com/spolu/wrp"
)

// flagFilterRegexp filters out flags from arguments.
var flagFilterRegexp = regexp.MustCompile("^-+")

func usage(
	ctx context.Context,
) {
	fmt.Printf("Usage: wrp {action} {name}\n")
	fmt.Printf("\n")
	fmt.Printf("Actions: \n")
	fmt.Printf("  open\n")
	fmt.Printf("    wrp open stan_dev\n")
	fmt.Printf("\n")
	fmt.Printf("  connect\n")
	fmt.Printf("    wrp connect stan-dev\n")
	fmt.Printf("    wrp stan-dev\n")
	fmt.Printf("    wrp stan-dev --readwrite\n")
	fmt.Printf("\n")
}

// Default address
var address = ":4242"

func main() {
	ctx := context.Background()

	if os.Getenv("WRPD_ADDRESS") != "" {
		address = os.Getenv("WRPD_ADDRESS")
	}

	args := []string{}
	flags := map[string]string{}

	for _, a := range os.Args[1:] {
		if flagFilterRegexp.MatchString(a) {
			a = strings.Trim(a, "-")
			s := strings.Split(a, "=")
			if len(s) == 2 {
				flags[s[0]] = s[1]
			} else {
				flags[s[0]] = ""
			}
		} else {
			args = append(args, strings.TrimSpace(a))
		}
	}

	// Default action to `connect`.
	if len(args) == 1 {
		args = append([]string{"connect"}, args...)
	}

	if len(args) != 2 {
		usage(ctx)
		return
	}

	readOnly := true
	if _, ok := flags["readwrite"]; ok {
		readOnly = false
	}

	switch args[0] {
	case "open":
		err := open(ctx, args[1])
		if err != nil {
			log.Fatalf("error: %v", err)
		}
	case "connect":
		err := connect(ctx, args[1], readOnly)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
	}
}

func open(
	ctx context.Context,
	warp string,
) error {
	ctx, cancel := context.WithCancel(ctx)

	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("connection error: %v", err)
	}

	session, err := yamux.Client(conn, nil)
	if err != nil {
		return fmt.Errorf("session error: %v", err)
	}
	defer session.Close() // closes stateChannel, dataChannel, session and conn

	// Setup pty
	cmd := exec.Command("/bin/bash")
	shell, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("pty error: %v", err)
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("wait error: %v", err)
		}
		cancel()
	}()
	defer shell.Close()

	// Setup local term
	stdin := int(os.Stdin.Fd())
	if !terminal.IsTerminal(stdin) {
		return fmt.Errorf("not a terminal")
	}
	oldState, err := terminal.MakeRaw(stdin)
	if err != nil {
		return fmt.Errorf("unable to make terminal raw: %v", err)
	}
	defer terminal.Restore(stdin, oldState)

	// Open state channel
	stateChannel, err := session.Open()
	if err != nil {
		return fmt.Errorf("state channel open error: %v", err)
	}
	w := gob.NewEncoder(stateChannel)

	// Send handshake.
	cols, rows, err := terminal.GetSize(stdin)
	if err != nil {
		return fmt.Errorf("getsize error: %v", err)
	}

	if err := w.Encode(wrp.State{
		Warp:       warp,
		Lurking:    false,
		WindowSize: wrp.Size{Rows: rows, Cols: cols},
		ReadOnly:   false,
	}); err != nil {
		return fmt.Errorf("handshake error: %v", err)
	}

	// Forward window resizes to pty and stateChannel
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGWINCH)
		for {
			cols, rows, err := terminal.GetSize(stdin)
			if err != nil {
				log.Printf("getsize error: %v", err)
				break
			}
			if err := Setsize(shell, rows, cols); err != nil {
				log.Printf("setsize error: %v", err)
				break
			}
			if err := syscall.Kill(cmd.Process.Pid, syscall.SIGWINCH); err != nil {
				log.Printf("sigwinch error: %v", err)
				break
			}

			if err := w.Encode(wrp.State{
				Warp:       warp,
				Lurking:    false,
				WindowSize: wrp.Size{Rows: rows, Cols: cols},
				ReadOnly:   false,
			}); err != nil {
				break
			}
			<-c
		}
		cancel()
	}()

	dataChannel, err := session.Open()
	if err != nil {
		return fmt.Errorf("data channel open error: %v", err)
	}

	multiplex := func(dst []io.Writer, src io.Reader) {
		buf := make([]byte, 1024)
		for {
			nr, err := src.Read(buf)
			if nr > 0 {
				cpy := make([]byte, nr)
				copy(cpy, buf)
				for _, d := range dst {
					d.Write(cpy)
				}
			}
			if err != nil {
				break
			}
			select {
			case <-ctx.Done():
				break
			default:
			}
		}
		cancel()
	}
	cp := func(dst io.Writer, src io.Reader) {
		io.Copy(dst, src)
		cancel()
	}

	go multiplex([]io.Writer{dataChannel, os.Stdout}, shell)
	go cp(shell, dataChannel)
	go cp(shell, os.Stdin)

	<-ctx.Done()

	return nil
}

func connect(
	ctx context.Context,
	warp string,
	readOnly bool,
) error {
	ctx, cancel := context.WithCancel(ctx)

	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("connection error: %v", err)
	}

	session, err := yamux.Client(conn, nil)
	if err != nil {
		return fmt.Errorf("session error: %v", err)
	}
	defer session.Close() // closes stateChannel, dataChannel, session and conn

	// Setup local term
	stdin := int(os.Stdin.Fd())
	if !terminal.IsTerminal(stdin) {
		return fmt.Errorf("not a terminal")
	}
	oldState, err := terminal.MakeRaw(stdin)
	if err != nil {
		return fmt.Errorf("unable to make terminal raw: %v", err)
	}
	defer terminal.Restore(stdin, oldState)

	// Open state channel
	stateChannel, err := session.Open()
	if err != nil {
		return fmt.Errorf("state channel open error: %v", err)
	}
	w := gob.NewEncoder(stateChannel)

	// Send handshake.
	cols, rows, err := terminal.GetSize(stdin)
	if err != nil {
		return fmt.Errorf("getsize error: %v", err)
	}

	if err := w.Encode(wrp.State{
		Warp:       warp,
		Lurking:    true,
		WindowSize: wrp.Size{Rows: rows, Cols: cols},
		ReadOnly:   readOnly,
	}); err != nil {
		return fmt.Errorf("handshake error: %v", err)
	}

	go func() {
		r := gob.NewDecoder(stateChannel)
		for {
			var st wrp.State
			if err := r.Decode(&st); err != nil {
				log.Printf("state channel decode error: %v", err)
				break
			}
			// Update the terminal size.
			fmt.Printf("\033[8;%d;%dt", st.WindowSize.Rows, st.WindowSize.Cols)
		}
		cancel()
	}()

	dataChannel, err := session.Open()
	if err != nil {
		return fmt.Errorf("data channel open error: %v", err)
	}

	cp := func(dst io.Writer, src io.Reader) {
		io.Copy(dst, src)
		cancel()
	}
	go cp(dataChannel, os.Stdin)
	go cp(os.Stdout, dataChannel)

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
