package main

import (
	"context"
	"flag"
	"log"

	"github.com/spolu/warp"
	"github.com/spolu/warp/daemon"
	"github.com/spolu/warp/lib/errors"
	"github.com/spolu/warp/lib/logging"
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

	logging.Logf(ctx, "Started warpd: version=%s", warp.Version)

	err := srv.Run(ctx)
	if err != nil {
		log.Fatal(errors.Details(err))
	}
}
