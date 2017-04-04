package main

import (
	"os"

	"github.com/spolu/wrp/cli"
	_ "github.com/spolu/wrp/cli/command"
	"github.com/spolu/wrp/lib/out"
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
}
