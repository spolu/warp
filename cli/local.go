package cli

import (
	"context"
	"encoding/gob"
	"net"
	"os"

	"github.com/hashicorp/yamux"
	"github.com/spolu/warp"
	"github.com/spolu/warp/lib/errors"
)

// Runs a local in-warp command and returns the result.
func RunLocalCommand(
	ctx context.Context,
	cmd warp.Command,
) (*warp.CommandResult, error) {
	path := os.Getenv(warp.EnvWarpUnixSocket)

	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, errors.Trace(err)
	}

	mux, err := yamux.Client(conn, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer mux.Close()

	// Opens command channel commandC.
	commandC, err := mux.Open()
	if err != nil {
		return nil, errors.Trace(err)
	}
	commandR := gob.NewDecoder(commandC)
	commandW := gob.NewEncoder(commandC)

	if err := commandW.Encode(cmd); err != nil {
		return nil, errors.Trace(
			errors.Newf("Failed to send command: %v", err),
		)
	}

	// Waiting for command result.
	var result warp.CommandResult
	if err := commandR.Decode(&result); err != nil {
		return nil, errors.Trace(err)
	}

	return &result, nil
}
