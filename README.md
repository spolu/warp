```
      ___           ___           ___           ___   
     /__/\         /  /\         /  /\         /  /\  
    _\_ \:\       /  /::\       /  /::\       /  /::\ 
   /__/\ \:\     /  /:/\:\     /  /:/\:\     /  /:/\:\
  _\_ \:\ \:\   /  /:/~/::\   /  /:/~/:/    /  /:/~/:/
 /__/\ \:\ \:\ /__/:/ /:/\:\ /__/:/ /:/___ /__/:/ /:/ 
 \  \:\ \:\/:/ \  \:\/:/__\/ \  \:\/:::::/ \  \:\/:/  
  \  \:\ \::/   \  \::/       \  \::/~~~~   \  \::/   
   \  \:\/:/     \  \:\        \  \:\        \  \:\   
    \  \::/       \  \:\        \  \:\        \  \:\  
     \__\/         \__\/         \__\/         \__\/  
```

#### `warp` lets you instantly share your terminal directly from your machine

`warp` is designed and optimized for high-bandwidth interactions between
developers.

## Usage

Instantly start sharing your terminal (read-only) under warp ID **goofy-dev**
with:

```shell
# While **goofy-dev** is a pretty cool warp name, you can name your warps
# however you want. In particular a cryptographically secure random ID will be
# generated for you if you don't specifiy one.

$ warp open goofy-dev
```

From there, anyone can connect (read-only) to your warp with:

```shell
$ warp connect goofy-dev
```

## Installation

#### From source code

```shell
# Requires Go to be installed on your machine. You can easily install Go from
# https://golang.org/doc/install

go get -u github.com/spolu/warp/client/cmd/warp
```

## Security

`warp` is a powerful, and therefore, dangerous tool. Its misuse can potentially
enable an attacker to easily gain arbitrary remote code execution priviledges.

#### TLS connections

The connection between your host as well as your warp clients and the `warpd`
server are established over TLS, protecting you from man in the middle attacks.

#### Read-only by default

By default, warps are created read-only. Being protected by TLS does not
protect you from phishing. Be extra careful when running `warp authorize`.

#### IDs are secure and secret

Generated warp IDs are cryptographically secure and not publicized. If you want
to authorize someone to write to your warp, we recommend you use a generated
warp ID (to protect yourself against phishing attacks).

#### Trustless read-only

In particular, when your warp does not authorize anyone to write, it does not
trust the `warpd` daemon to enforce that noone other than you can write to it.
When at least one client is authorized to write, `warp` does trust the `warpd`
daemon it is connected to to enforce the read/write policy of clients.

## Roadmap

- [ ] *v0.0.2 "bare"*
  - see [TODO](TODO)
- [ ] *v0.1.0 "safe"*
  - TLS
  - PROMPT support
  - Persisted user token/secret
  - Graceful host reconnect
- [ ] *v0.1.1 "chat"*
  - `warp chat :warp` lets you voice-over a warp
- [ ] *future releases*
  - terminal emulation
    - full redraw on connection
    - top status bar
    - terminal truncation
  - warp signin and verified usernames

## Notes

#### `warp` is not a fork of tmux

`warp` is not a fork of tmux and is not a terminal emulator (though we might
eventually get to terminal emulation to enhance the user experience). It really
simply multiplexes stdin/stdout to raw ptys between host and clients. For that
reason, if you connect to a warp already running a GUI-like application (tmux,
vim, htop, ...) it might take time or host interactions for the GUI-like
application to visually reconstruct properly client-side.

In particular, since `warp` does not emulate the terminal it cannot reformat or
truncate the output of the host terminal to fit client terminal windows which
may lead to distorted outputs client side if the terminal sizes mismatch. To
mitigate that, `warp` relies on automatic client terminal resizing.

#### Automatic client terminal resize

Once connected as a client and whenever the host terminal window size changes,
`warp` will attempt to resize your terminal window to the hosting tty size. For
that reason it is recommended to run `warp connect` from a new terminal window.

#### Development of warp

Development of `warp` is generally broadcasted in **warp-dev**. Feel free to
connect at any time.
