`keep` is a rudimentary cache for expensive commands.

- [x] Simple to use: Just prefix any command with `keep`.
- [x] Request debouncing: A command called from multiple processes simultaneously is executed only once.
- [x] Time-to-live: Specify a maximum age for recalling command output.
- [x] Private: Cached outputs are owned by, and may only be read by, the originating user[^1].

[^1]: Restrictive file permissions are smart, but not inherently secure; this utility would be
unsuitable for keeping highly-sensitive data or for use outside local development.

```bash
keep --ttl 5m curl https://example.com/slow-resource
```

### Installation

Only Linux (or Windows Subsystem for Linux), and macOS are supported.

1. Go 1.20+ (https://go.dev/dl/)
2. Add `~/go/bin` to `PATH`
3. `go install github.com/credding/keep@latest`

### Usage

```
Usage: keep [OPTION]... [--] COMMAND [ARG]...
Save the output of a command for recall later.
      --ttl duration   time to remember command output (default 1h0m0s)
  -h, --help           display this help message
```

### Internals

Cached outputs are saved in `$XDG_STATE_HOME/keep`, or `~/.local/state/keep`.
