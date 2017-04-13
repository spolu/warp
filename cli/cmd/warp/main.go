package main

import (
	"os"
	"time"

	"github.com/spolu/warp/cli"
	_ "github.com/spolu/warp/cli/command"
	"github.com/spolu/warp/lib/out"
)

func main() {
	cli, err := cli.New(os.Args[1:])
	if err != nil {
		out.Errof("[Error] %s\n", err.Error())
	}

	err = cli.Run()
	if err != nil {
		out.Errof("[Error] %s\n", err.Error())
	}

	// Sleep for 100 give time to all goroutine to exist properly and to the
	// session ErrorOut to print out after the terminal is restored from raw
	// mode.
	time.Sleep(100 * time.Millisecond)
}
