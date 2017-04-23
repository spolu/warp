package cli

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spolu/warp/lib/errors"
	"github.com/spolu/warp/lib/token"
)

// Credentials repesents the credentials of the currently logged in user.
type Credentials struct {
	User   string `json:"user"`
	Secret string `json:"secret"`
}

// Config represents the local configuration for warp.
type Config struct {
	Credentials Credentials `json:"credentials"`
}

// ConfigPath returns the crendentials path for the current environment.
func ConfigPath(
	ctx context.Context,
) (*string, error) {
	path, err := homedir.Expand(
		"~/.warp/config.json",
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = os.MkdirAll(filepath.Dir(path), 0777)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &path, nil
}

// RetrieveConfig retrieves the current user config by reading ConfigPath.
func RetrieveConfig(
	ctx context.Context,
) (*Config, error) {
	path, err := ConfigPath(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if _, err := os.Stat(*path); os.IsNotExist(err) {
		return nil, nil
	}

	raw, err := ioutil.ReadFile(*path)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var c Config
	err = json.Unmarshal(raw, &c)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &c, nil
}

// GenerateConfig generates a new config and store it. As part of it, it
// generates a new set of credentials.
func GenerateConfig(
	ctx context.Context,
) (*Config, error) {
	config := &Config{
		Credentials: Credentials{
			User:   token.New("guest"),
			Secret: token.RandStr(),
		},
	}

	path, err := ConfigPath(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	formatted, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = ioutil.WriteFile(*path, formatted, 0644)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return config, nil
}

// RetrieveOrGenerateConfig retrieves the current config or generates it.
func RetrieveOrGenerateConfig(
	ctx context.Context,
) (*Config, error) {

	config, err := RetrieveConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if config == nil {
		config, err = GenerateConfig(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return config, nil
}
