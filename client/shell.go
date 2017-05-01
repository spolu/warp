package cli

import (
	"context"
	"os"

	"github.com/spolu/warp/lib/errors"
)

type Shell struct {
	Command string
	PS1     string
	PROMPT  string
}

// retrieveShell retrieves the current shell for the user using the following
// fallbacks:
// - read env variable SHELL
// - TODO: read /etc/password
// - default to `/bin/bash`
func retrieveShell(
	ctx context.Context,
) (string, error) {
	if os.Getenv("SHELL") != "" {
		return os.Getenv("SHELL"), nil
	}
	return "/bin/bash", nil
}

func DetectShell(
	ctx context.Context,
) (*Shell, error) {
	command, err := retrieveShell(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	shell := Shell{
		Command: command,
	}

	return &shell, nil
}
