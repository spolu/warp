`warp` lets you instantly share your terminal directly from your machine.

Instantly start sharing your terminal (read-only) with:

```
warp open stan-dev
```
From there, anyone can connect (read-only) to your newly spawned terminal using
your warp ID by running:

```
warp connect stan-dev
```

# Use-cases

Warp is designed to enable high-bandwidth intereactions between developers.

# Installation

```
go get -u github.com/spolu/warp/cli/cmd/warp
```

# Security

 - TLS connections.
 - Read-only by default.
 - Limited trust in warpd (trustless if no other writer than the host).

# Roadmap

  - *v0.1.0 "metal"*
    - see [TODO](TODO)
  - *v0.1.1 "chat"*
    - `warp chat :warp` lets you voice-over a warp
  - *future*
    - terminal emulation
      - full redraw on connection
      - top status bar

# Notes

Once connected, `warp` will resize your terminal window to the hosting tty size
(if possible). So, it's recommended to run `warp connect` from a new terminal
window.  

