package main

import (
	"context"
	"flag"
	"log"

	"github.com/spolu/wrp/daemon"
	"github.com/spolu/wrp/lib/errors"
)

var lstFlag string

func init() {
	flag.StringVar(&lstFlag, "listen",
		":4242", "Address to listen on ([ip]:port), default: `:4242`")

	if fl := log.Flags(); fl&log.Ltime != 0 {
		log.SetFlags(fl | log.Lmicroseconds)
	}
}

func main() {
	if !flag.Parsed() {
		flag.Parse()
	}

	ctx := context.Background()

	srv := daemon.NewSrv(
		ctx,
		lstFlag,
	)

	err := srv.Run(ctx)
	if err != nil {
		log.Fatal(errors.Details(err))
	}
}
