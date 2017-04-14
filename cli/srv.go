package cli

import (
	"context"
	"sync"
)

type Srv struct {
	warp string

	mutex *sync.Mutex
}

// NewSrv constructs a Srv ready to start serving local requests.
func NewSrv(
	ctx context.Context,
	warp string,
) *Srv {
	return &Srv{
		address: address,
		warps:   map[string]*Warp{},
		mutex:   &sync.Mutex{},
	}
}
