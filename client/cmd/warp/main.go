package main

import (
	"os"

	"github.com/spolu/warp/client"
	_ "github.com/spolu/warp/client/command"
	"github.com/spolu/warp/lib/out"
)

func main() {
	cli, err := cli.New(os.Args[1:])
	if err != nil {
		out.Errof("[Error] %s\n", err.Error())
	}

	err = cli.Run()
	if err != nil {
		out.Errof("\n[Error] %s\n", err.Error())
	}
}
