package wrp

// Size reprensents a window size.
type Size struct {
	Rows int
	Cols int
}

// Client represents a client connected to the wrp.
type Client struct {
	Username string
	CanWrite bool
	CanSpeak bool
}

// State is the struct sent over the network to update the client state.
type State struct {
	ID              string
	WindowSize      Size
	Owner           string
	OtherCanWrite   bool
	OtherCanSpeak   bool
	DefaultCanWrite bool
	DefaultCanSpeak bool
	Clients         map[string]Client
}

// OwnerUpdate represents an update to the wrp state from its owner.
type OwnerUpdate struct {
	Username        string
	OtherCanWrite   bool
	OtherCanSpeak   bool
	DefaultCanWrite bool
	DefaultCanSpeak bool
}
