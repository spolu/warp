package cli

import (
	"context"
	"encoding/gob"
	"fmt"
	"net"
	"os"
	"path"

	"github.com/spolu/warp"
	"github.com/spolu/warp/lib/errors"
	"github.com/spolu/warp/lib/out"
)

// RunLocalCommand runs a local in-warp command and returns the result. If an
// error is returned as part of the result, it formats a human readable error
// that can be safely returned top the user.
func RunLocalCommand(
	ctx context.Context,
	cmd warp.Command,
) (*warp.CommandResult, error) {
	p := path.Join(
		os.TempDir(),
		fmt.Sprintf("_warp_%s.sock", os.Getenv(warp.EnvWarp)),
	)

	conn, err := net.Dial("unix", p)
	if err != nil {
		return nil, errors.Trace(
			errors.Newf("Failed to connect to warpd: %v", err),
		)
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

	if result.Error.Code != "" {
		return nil, errors.Newf(
			"Received %s: %s",
			result.Error.Code,
			result.Error.Message,
		)
	}

	return &result, nil
}

// CheckWarpEnv checks that the warp.EnvWarp env variable is set. If not it
// returns an error after displaying an helpful message.
func CheckEnvWarp(
	ctx context.Context,
) error {
	if os.Getenv(warp.EnvWarp) != "" {
		return nil
	}
	out.Normf("\n")
	out.Normf("`warp` uses the environment variable `%s` to detect that it is running from\n", warp.EnvWarp)
	out.Normf("inside a warp (for in-warp commands). `%s` not being currently set, it\n", warp.EnvWarp)
	out.Normf("indicates that you are not executing this from inside a warp.\n")
	out.Normf("\n")
	out.Normf("Expert mode: if you connected to a pre-existing tmux or screen session from\n")
	out.Normf("your current warp, `%s` will not be propagated automatically. You can fix\n", warp.EnvWarp)
	out.Normf("this by setting `%s` to the ID of your warp in your current environment.\n", warp.EnvWarp)
	out.Normf("\n")

	return errors.Trace(
		errors.Newf("This command is only available from inside a warp."),
	)
}
