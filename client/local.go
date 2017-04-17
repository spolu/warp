package cli

import (
	"context"
	"encoding/gob"
	"net"
	"os"

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
	defer conn.Close()

	commandR := gob.NewDecoder(conn)
	commandW := gob.NewEncoder(conn)

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
