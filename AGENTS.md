# AGENTS.md

Gangplank is a Go daemon and CLI that forwards router ports over UPnP for labelled Docker containers and static ports from a YAML file.

## Layout

- `cmd/`: cobra commands. Settings precedence is flag > `GANGPLANK_*` env var > config file > default (`cmd/root.go`).
- `internal/gangplank/`: `Manager` (port ownership, sync, prune, cleanup) and `Daemon` (refresh loop, gateway reconnect).
- `internal/providers/`: port sources. Config file, Docker labels (`gangplank.forward`, `gangplank.forward.container`) and Docker events.
- `internal/upnp/`: UPnP IGD client, dry-run client and `upnptest` fakes.
- `internal/portmap/`, `internal/config/`: mapping type and YAML config.

## Commands

- `make test`: tests with the race detector. Run before every commit.
- `make lint`: golangci-lint (config in `.golangci.yml`). `make fmt` formats.
- `make cover`: coverage across packages. Keep it high; new code needs tests.

## Conventions

- Log with `log/slog` to stderr. Command output (e.g. `list`) goes to stdout.
- Keep fakes in tests or `upnptest`; do not talk to real routers or Docker in tests.
- A failing source must not drop mappings from the other sources. Return joined errors instead of stopping early.
- Only touch router mappings Gangplank owns (description prefix `Gangplank UPnP`) when pruning.
- Update `doc/` and `README.md` when flags, labels or config keys change.
- Commit messages: one line, imperative, lowercase, no trailing period.
