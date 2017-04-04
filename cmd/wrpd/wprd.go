package main

import (
	"context"
	"encoding/gob"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/spolu/wrp"

	"github.com/hashicorp/yamux"
)

// wrpd listens for connection from wrp client to allow multiplexing of ttys
// over the network and across client machines.

// client represents a client whether its a lurker or the origin.
type client struct {
	conn     net.Conn
	session  *yamux.Session
	readOnly bool

	stateChannel net.Conn
	stateDecoder *gob.Decoder
	stateEncoder *gob.Encoder

	dataChannel net.Conn

	ctx    context.Context
	cancel func()
}

// warp represents one pty served from a remote client attached to a name.
type warp struct {
	name    string
	origin  *client
	lurkers []*client
	mutex   *sync.Mutex

	data chan []byte

	size wrp.Size
}

// Internal state
var address = flag.String(
	"listen", ":4242", "Address to listen on ([ip]:port).",
)
var warps = map[string]*warp{}
var mutex = &sync.Mutex{}

func main() {
	ctx := context.Background()
	ln, err := net.Listen("tcp", *address)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("listening on %s", *address)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf(
				"ERROR: [%s] accept error: %v",
				conn.RemoteAddr().String(), err,
			)
			continue
		}
		go func() {
			err := handle(ctx, conn)
			if err != nil {
				log.Printf(
					"ERROR: [%s] %v",
					conn.RemoteAddr().String(), err,
				)
			} else {
				log.Printf("[%s] done", conn.RemoteAddr().String())
			}
		}()
	}
}

func handle(
	ctx context.Context,
	conn net.Conn,
) error {
	session, err := yamux.Server(conn, nil)
	if err != nil {
		return fmt.Errorf("session error: %v", err)
	}
	defer session.Close()

	stateChannel, err := session.Accept()
	if err != nil {
		return fmt.Errorf("state channel accept error: %v", err)
	}
	stateDecoder := gob.NewDecoder(stateChannel)
	stateEncoder := gob.NewEncoder(stateChannel)

	var hello wrp.State
	if err := stateDecoder.Decode(&hello); err != nil {
		return fmt.Errorf("handshake error: %v", err)
	}
	fmt.Printf(
		"[%s] handshake received: warp=%s lurking=%t rows=%d cols=%d\n",
		conn.RemoteAddr().String(), hello.Warp, hello.Lurking,
		hello.WindowSize.Rows, hello.WindowSize.Cols,
	)

	dataChannel, err := session.Accept()
	if err != nil {
		return fmt.Errorf("data channel accept error: %v", err)
	}

	mutex.Lock()
	w, ok := warps[hello.Warp]

	ctx, cancel := context.WithCancel(ctx)

	c := &client{
		conn:         conn,
		session:      session,
		readOnly:     hello.ReadOnly,
		stateChannel: stateChannel,
		stateDecoder: stateDecoder,
		stateEncoder: stateEncoder,
		dataChannel:  dataChannel,
		ctx:          ctx,
		cancel:       cancel,
	}

	switch hello.Lurking {
	case true:
		{
			if !ok {
				mutex.Unlock()
				return fmt.Errorf(
					"handshake error: lurking unknown warp %s",
					hello.Warp,
				)
			}

			w.lurkers = append(w.lurkers, c)

			mutex.Unlock()
			return handleLurker(ctx, session, c, w)
		}
	case false:
		{
			if ok {
				mutex.Unlock()
				return fmt.Errorf(
					"handshake error: opening existing warp %s",
					hello.Warp,
				)
			}

			w := &warp{
				name:    hello.Warp,
				origin:  c,
				lurkers: []*client{},
				mutex:   &sync.Mutex{},
				data:    make(chan []byte),
			}
			warps[hello.Warp] = w

			mutex.Unlock()
			return handleOrigin(ctx, session, c, w)
		}
	}

	return nil
}

func handleLurker(
	ctx context.Context,
	session *yamux.Session,
	c *client,
	w *warp,
) error {
	// Read lurker data into data if not readOnly
	go func() {
		buf := make([]byte, 1024)
		for {
			nr, err := c.dataChannel.Read(buf)
			if nr > 0 && !c.readOnly {
				cpy := make([]byte, nr)
				copy(cpy, buf)

				fmt.Printf(
					"[%s] Received from lurker: warp=%s size=%d\n",
					c.conn.RemoteAddr().String(), w.name, nr,
				)

				w.data <- cpy
			}
			if err != nil {
				break
			}
			select {
			case <-c.ctx.Done():
				break
			default:
			}
		}
		c.cancel()
	}()

	// Send initial state
	w.mutex.Lock()
	size := w.size
	w.mutex.Unlock()

	fmt.Printf(
		"[%s] sending win size: warp=%s cols=%d rows=%d\n",
		c.conn.RemoteAddr().String(), w.name,
		size.Rows, size.Cols,
	)
	c.stateEncoder.Encode(wrp.State{
		Warp:       w.name,
		Lurking:    true,
		WindowSize: size,
		ReadOnly:   c.readOnly,
	})

	fmt.Printf(
		"[%s] lurker running: warp=%s\n",
		c.conn.RemoteAddr().String(), w.name,
	)

	<-c.ctx.Done()

	return nil
}

func handleOrigin(
	ctx context.Context,
	session *yamux.Session,
	c *client,
	w *warp,
) error {
	// run state updates
	go func() {
		for {
			var st wrp.State
			if err := c.stateDecoder.Decode(&st); err != nil {
				break
			}

			w.mutex.Lock()
			w.size = st.WindowSize
			w.mutex.Unlock()

			fmt.Printf(
				"[%s] received win size: warp=%s cols=%d rows=%d\n",
				c.conn.RemoteAddr().String(), w.name,
				w.size.Rows, w.size.Cols,
			)

			for _, l := range w.lurkers {
				fmt.Printf(
					"[%s] sending win size: warp=%s cols=%d rows=%d\n",
					l.conn.RemoteAddr().String(), w.name,
					st.WindowSize.Rows, st.WindowSize.Cols,
				)
				l.stateEncoder.Encode(wrp.State{
					Warp:       w.name,
					Lurking:    true,
					WindowSize: st.WindowSize,
					ReadOnly:   l.readOnly,
				})
			}
		}
	}()

	// Read origin data and relay to lurkers
	go func() {
		buf := make([]byte, 1024)
		for {
			nr, err := c.dataChannel.Read(buf)
			if nr > 0 {
				cpy := make([]byte, nr)
				copy(cpy, buf)

				for _, l := range w.lurkers {
					fmt.Printf(
						"[%s] Sending to lurker: warp=%s size=%d\n",
						l.conn.RemoteAddr().String(), w.name, len(cpy),
					)
					_, err := l.dataChannel.Write(cpy)
					if err != nil {
						fmt.Printf("[%s] error sending: %v\n",
							l.conn.RemoteAddr().String(), err,
						)
					}
				}
			}
			if err != nil {
				break
			}
			select {
			case <-c.ctx.Done():
				break
			default:
			}
		}
		fmt.Printf("CANCEL origin data -> dataOut\n")
		c.cancel()
	}()

	// Read data into origin
	go func() {
		for {
			select {
			case buf := <-w.data:

				fmt.Printf(
					"[%s] Sending to origin: warp=%s size=%d\n",
					c.conn.RemoteAddr().String(), w.name, len(buf),
				)

				_, err := c.dataChannel.Write(buf)
				if err != nil {
					break
				}
			case <-c.ctx.Done():
				break
			default:
			}
		}
		fmt.Printf("CANCEL origin data -> data\n")
		c.cancel()
	}()

	fmt.Printf(
		"[%s] origin running: warp=%s\n",
		c.conn.RemoteAddr().String(), w.name,
	)

	<-c.ctx.Done()

	for _, l := range w.lurkers {
		l.cancel()
	}
	mutex.Lock()
	delete(warps, w.name)
	mutex.Unlock()

	return nil
}
