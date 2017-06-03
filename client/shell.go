package cli

import (
	"context"
	"os"
        "bufio"
        "fmt"
        "strconv"
        "strings"

	"github.com/spolu/warp/lib/errors"
)

type Shell struct {
	Command string
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
        file, err := os.Open("/etc/passwd")
        if err != nil {
            return nil, errors.Trace(err)
        }
        defer file.Close()
        scanner := bufio.NewScanner(file)

        for scanner.Scan() {
            s := strings.Split(scanner.Text(), ":")
            if (len(s) > 3) {
                value, _ := strconv.Atoi(s[2])
                if (value >= 1000) && !(strings.Contains(s[6], "nologin")) {
                    return s[len(s)-1]
                }
            }
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
