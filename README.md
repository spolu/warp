`warp` lets you share your terminal directly from your machine.

You can start sharing your terminal with:

```
warp open stan-dev
```

This will print the newly created warp ID and spawn a new shared temrinal.

From there anyone can connect to your newly spawned terminal using the warp ID
by running:

```
warp connect stan-dev
```

# Use-cases

Warp is designed to enable high-bandwidth intereactions between developers.

# Installation

```
go get -u github.com/spolu/warp/cli/cmd/warp
```

# Notes

Once connected, `warp` will resize your terminal window to the hosting tty size
(if possible). So, when connecting to a warp, it's recommended to run `warp` from
new terminal window.  

