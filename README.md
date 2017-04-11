`warp` lets you share your terminal directly from your machine.

You can start sharing your terminal with:

```
warp open
```

This will print the newly created warp ID and spawn a new shared temrinal.

From there anyone can connect to your newly spawned terminal using the warp ID
by running:

```
warp ae7fb6a24
```

# Use-cases

- quickly share a terminal with someone for bug squashing or pairing
- broadcast your daily musing with code to the ether

# Installation

```
go get -u github.com/spolu/warp
```

# Notes

Once connected, `warp` will resize your terminal window to the hosting tty size
(if possible). So, when connecting to a warp, it's recommended to run `warp` from
new terminal window.  

Unfortunately, `warp` does not support redrawing the whole tty for new
clients... yet /o\ 

*If the warp contains tmux session, changing the current tab will trigger
a full redraw, resizing the hosting window will do the trick as well.*
