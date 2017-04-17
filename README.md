`warp` lets you instantly share your terminal directly from your machine.

Instantly start sharing your terminal (read-only) under warp **goofy-dev**
with:

```
warp open goofy-dev
```

From there, anyone can connect (read-only) to your newly spawned terminal using
your warp **goofy-dev** by running:

```
warp connect goofy-dev
```

# Use-cases

Warp is designed to enable high-bandwidth intereactions between developers.

# Installation

```
go get -u github.com/spolu/warp/cli/cmd/warp
```

# Security

#### TLS connections.

The connection between your host as well as you warp clients and the warpd
server are established over TLS, protecting you from man in the middle attacks.

#### Read-only by default.

#### Limited trust in warpd (trustless if no other writer than the host).

# Roadmap

- [ ] *v0.1.0 "metal"*
  - see [TODO](TODO)
- [ ] *v0.1.1 "chat"*
  - `warp chat :warp` lets you voice-over a warp
- [ ] *future releases*
  - terminal emulation
    - full redraw on connection
    - top status bar
  - warp signin and verified usernames

# Notes

Once connected, `warp` will resize your terminal window to the hosting tty size
(if possible). So, it's recommended to run `warp connect` from a new terminal
window.  

