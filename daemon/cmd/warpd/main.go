package main

import (
	"context"
	"flag"
	"log"
	"os"
	"runtime/pprof"
	"time"

	"github.com/spolu/warp"
	"github.com/spolu/warp/daemon"
	"github.com/spolu/warp/lib/errors"
	"github.com/spolu/warp/lib/logging"
)

var lstFlag string
var prfFlag string

func init() {
	flag.StringVar(&lstFlag, "listen",
		":4242", "Address to listen on ([ip]:port), default: `:4242`")
	flag.StringVar(&prfFlag, "cpuprofile",
		"", "Enalbe CPU profiling and write to specified file")

	if fl := log.Flags(); fl&log.Ltime != 0 {
		log.SetFlags(fl | log.Lmicroseconds)
	}
}

func main() {
	if !flag.Parsed() {
		flag.Parse()
	}

	if prfFlag != "" {
		f, err := os.Create(prfFlag)
		if err != nil {
			log.Fatal(errors.Details(err))
		}

		go func() {
			pprof.StartCPUProfile(f)
			time.Sleep(10 * time.Second)
			pprof.StopCPUProfile()
			log.Fatal("OUT!")
		}()
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
