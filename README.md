#### `warp` lets you instantly share your terminal directly from your machine.

Warp is designed to enable high-bandwidth interactions between developers.

## Usage

Instantly start sharing your terminal (read-only) under warp ID **goofy-dev**
with:

```
# While **goofy-dev** is a pretty cool name, you can name you warps however you
# want. In particular a cryptographically secure random ID will be generated
# for you if you don't specifiy one.

warp open goofy-dev
```

From there, anyone can connect (read-only) to your warp by running:

```
warp connect goofy-dev
```

## Installation

```
go get -u github.com/spolu/warp/cli/cmd/warp
```

## Security

#### TLS connections.

The connection between your host as well as you warp clients and the warpd
server are established over TLS, protecting you from man in the middle attacks.

#### Read-only by default.

#### Limited trust in warpd (trustless if no other writer than the host).

## Roadmap

- [ ] *v0.1.0 "metal"*
  - see [TODO](TODO)
- [ ] *v0.1.1 "chat"*
  - `warp chat :warp` lets you voice-over a warp
- [ ] *future releases*
  - terminal emulation
    - full redraw on connection
    - top status bar
  - warp signin and verified usernames

## Notes

Once connected, `warp` will resize your terminal window to the hosting tty size
(if possible). So, it's recommended to run `warp connect` from a new terminal
window.  

