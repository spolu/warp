package wrp

// Size reprensents a window size.
type Size struct {
	Rows int
	Cols int
}

// State is the struct sent over the network to update the client state.
type State struct {
	Warp       string
	Lurking    bool
	WindowSize Size
	ReadOnly   bool
}
