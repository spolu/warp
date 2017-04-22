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

// CredentialsPath returns the crendentials path for the current environment.
func CredentialsPath(
	ctx context.Context,
) (*string, error) {
	path, err := homedir.Expand(
		"~/.warp/credentials.json",
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

// CurrentCredentials retrieves the current user by reading CredentialsPath.
func CurrentCredentials(
	ctx context.Context,
) (*Credentials, error) {
	path, err := CredentialsPath(ctx)
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

	var c Credentials
	err = json.Unmarshal(raw, &c)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &c, nil
}

// GenerateCredentials generates new credentials and store them.
func GenerateCredentials(
	ctx context.Context,
) (*Credentials, error) {
	creds := &Credentials{
		User:   token.New("guest"),
		Secret: token.RandStr(),
	}

	path, err := CredentialsPath(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	formatted, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = ioutil.WriteFile(*path, formatted, 0644)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return creds, nil
}

// GetOrGenerateCredentials retrieves the current credentials or generate them.
func GetOrGenerateCredentials(
	ctx context.Context,
) (*Credentials, error) {

	creds, err := CurrentCredentials(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if creds == nil {
		creds, err = GenerateCredentials(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return creds, nil
}
