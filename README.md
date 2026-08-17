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
- A served web dashboard for composing routes, managing keys and saved model
  presets, and bench-testing
- Daily release checks, with an opt-in automatic download-and-stage setting
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
handoff snippets for Shell, agent prompts, and Python. The masthead gear opens
a Settings modal where you can manage updates and add or remove the
provider/model presets shown above the signal path. Model settings persist in
`~/.config/gatekey-proxy/models.json`; any model ID can still be entered
directly without saving it first.

### Updates

Gatekey checks the latest GitHub release when it starts and once a day while it
is running. Automatic checks are on by default; automatic installation is opt-in
in the gear modal. A downloaded release is verified against `checksums.txt`,
staged under `~/.config/gatekey-proxy/update/`, and applied the next time Gatekey
starts. The running proxy is never replaced mid-request.

Only stable `vMAJOR.MINOR.PATCH` release builds on macOS and Linux can replace
themselves. Development builds can still report available versions. Update
preferences persist in `~/.config/gatekey-proxy/updates.json`.

To check and stage an update from the terminal:

```sh
gatekey-proxy update
```

To print the installed version:

```sh
gatekey-proxy version
```

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
| `openai/X`             | api.openai.com                    | `X`                    |
| `provider/X`           | api.provider.com                  | `X`                    |
| no prefix              | api.openai.com                    | unchanged              |

Prefixed models route to a fixed upstream even when that provider is slashed
(e.g. `openrouter/anthropic/claude-3.5-sonnet`).

## CLI

```
start                        Start the Gatekey Proxy server on port 8181
version                      Print the installed Gatekey Proxy version
update                       Check for and stage a verified update
keys add <provider> <key>    Securely store an API key
keys list                    List all configured providers
```

## Security

- Listens on `127.0.0.1:8181` — loopback only
- Config directory created with mode 0700; file written with mode 0600
- The key is plaintext JSON on disk — anyone with your user account can read it
- Update downloads are restricted to GitHub HTTPS asset hosts, size-bounded,
  checksum-verified, and staged before executable replacement
- Dashboard update mutations require same-origin browser requests

## Layout

| Path              | Role                                                        |
|-------------------|-------------------------------------------------------------|
| `main.go`          | applies a staged release, then calls `cli.Run()`            |
| `cli/cli.go`       | `start`, `version`, `update`, and key commands              |
| `buildinfo/`       | build-time/current version metadata                         |
| `config/config.go` | key store at `~/.config/gatekey-proxy/config.json`          |
| `config/models.go` | saved model presets at `~/.config/gatekey-proxy/models.json`|
| `config/updates.go`| automatic update preferences                                |
| `updater/`         | release checks, verification, staging, and replacement     |
| `server/proxy.go`  | JSON APIs, OpenAI-compatible proxy, and UI                  |
| `ui/`              | dashboard — `index.html`, `styles.css`, `app.js`            |

Pushing a stable version tag runs `.github/workflows/release.yml`, which tests
the code, builds macOS and Linux archives for Intel and Apple/ARM systems,
publishes `checksums.txt`, and creates the GitHub release consumed by Gatekey.

The dashboard is served from `./ui` when that folder exists in the process
working directory (UI edits are live without a rebuild); otherwise the copy
embedded in the binary at build time is used, so `gatekey-proxy start` works
from any directory.
