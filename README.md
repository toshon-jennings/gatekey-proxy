# Gatekey Proxy

A local, loopback-only OpenAI-compatible proxy. Point any agent or tool at
`http://127.0.0.1:8181/v1` with a dummy key; the proxy swaps in your real
provider key and forwards the request upstream. Your keys never leave this
machine.

```
your tool ─── sk-dummy ───> 127.0.0.1:8181 ─── real key ───> your provider
```

## Why

OpenAI SDKs and agent frameworks typically expect one base URL and one API key.
If you work with several providers, you either juggle environments or stash
keys in agent config. Gatekey Proxy gives you a single OpenAI-compatible
endpoint, and picks the upstream based on a provider prefix in the model id.

## Features

- Single endpoint at `http://127.0.0.1:8181/v1` for every provider
- Prefix-based routing (`groq/...`, `openrouter/...`, `opencode/...`, …)
- Keys stored locally in `~/.config/gatekey-proxy/config.json`
  (dir mode 0700, file mode 0600, readable only by you — plaintext, not encrypted)
- A served web dashboard for composing routes, managing keys, and bench-testing
- Bound to `127.0.0.1` only — nothing on your network can reach it
- Go binary + CLI, no runtime dependencies

## Install

Build from source (requires Go 1.26+):

```sh
go install github.com/toshon-jennings/gatekey-proxy@latest
```

Or build in the repo:

```sh
go build -o gatekey-proxy .
```

## Usage

### 1. Store a key

```sh
gatekey-proxy keys add groq gsk_your_real_key_here
```

### 2. Start the proxy

```sh
gatekey-proxy start
```

Log line: `Starting Gatekey Proxy server securely on http://127.0.0.1:8181`

### 3. Point a tool at it

```sh
export OPENAI_BASE_URL="http://127.0.0.1:8181/v1"
export OPENAI_API_KEY="sk-dummy"
export OPENAI_MODEL="groq/llama-3.3-70b-versatile"
```

The dashboard (`http://127.0.0.1:8181/`) has a bench test and ready-to-copy
handoff snippets for Shell, agent prompts, and Python.

## Routing

The proxy splits the model id on the first `/`. The part before the slash
selects the provider; the rest is forwarded as the model name.

| Model sent             | Upstream                          | Model forwarded        |
|------------------------|-----------------------------------|------------------------|
| `groq/X`               | api.groq.com                      | `X`                    |
| `openrouter/X`         | openrouter.ai                     | `X`                    |
| `opencode/X`           | opencode.ai (Zen)                 | `X`                    |
| `deepinfra/X`          | api.deepinfra.com                 | `X`                    |
| `together/X`           | api.together.xyz                  | `X`                    |
| anything else          | api.openai.com                    | **unchanged, prefix not stripped** |

Prefixed models route to a fixed upstream even when that provider is slashed
(e.g. `openrouter/anthropic/claude-3.5-sonnet`).

## CLI

```
start                        Start the Gatekey Proxy server on port 8181
keys add <provider> <key>    Securely store an API key
keys list                    List all configured providers
```

## Security

- Listens on `127.0.0.1:8181` — loopback only
- Config directory created with mode 0700; file written with mode 0600
- The key is plaintext JSON on disk — anyone with your user account can read it

## Layout

| Path              | Role                                                        |
|-------------------|-------------------------------------------------------------|
| `main.go`         | entry point, calls `cli.Run()`                              |
| `cli/cli.go`      | `start`, `keys add`, `keys list`                            |
| `config/config.go`| key store at `~/.config/gatekey-proxy/config.json`           |
| `server/proxy.go` | `/api/keys`, `/v1/chat/completions`, static file server     |
| `ui/`             | dashboard — `index.html`, `styles.css`, `app.js`            |

The dashboard is served from `./ui` when that folder exists in the process
working directory (UI edits are live without a rebuild); otherwise the copy
embedded in the binary at build time is used, so `gatekey-proxy start` works
from any directory.