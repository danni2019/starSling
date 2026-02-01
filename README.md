# starSling

starSling is a real-time trading-session watcher + strategy runner for macOS and Linux.

## Quickstart
- Go 1.25.6
- Build and run:

```bash
go build ./cmd/starsling
./starsling
```

- Or run directly:

```bash
go run ./cmd/starsling
```

The CLI launches an interactive terminal UI (arrow keys + prompts). The default
idle mode runs without any config file. Choose Live Market Data to enter or
override the `live-md` settings at startup.

To make Live Market Data fully standalone, run the Python bootstrap once to
download a local Python runtime and install dependencies:

```bash
./scripts/bootstrap_python.sh
```

You can also select "Setup Python runtime" from the interactive menu.
If you have a local OpenCTP wheel, set `OPENCTP_WHEEL=/path/to/openctp.whl`
before running the bootstrap script.

## Live market data (optional)
Live data requires a bundled Python 3.11 runtime and the OpenCTP Python
bindings. Use the bootstrap script above, or provide your own Python executable
in the prompt. When you select Live Market Data, starSling will launch the
embedded Python script and stream JSON ticks to stdout.

## Configuration (optional)
Configuration is JSON-only. Inputs are read from config files; no endpoints or
credentials are hard-coded. See `config/starsling.example.json` for the template.

When prompted, choose "Load config file" and provide the path to override the
embedded defaults.

## Metadata cache
On startup, starSling fetches OpenCTP metadata and caches it locally with a
`last_updated` timestamp. Cache files are stored under
`~/.starsling/metadata` when available, with a fallback to `runtime/metadata`.
Metadata sources are configured in `config/metadata.sources.json`.

## Project layout
- `cmd/starsling`: CLI entrypoint
- `internal/config`: config loader + embedded defaults
- `internal/session`: session detection boundary (stub)
- `internal/live`: live MD runner (embedded Python script)
- `internal/registry`: strategy registry placeholder
- `internal/marketdata`, `internal/strategy`: reserved for future runtime work
- `python`: placeholder for future Python strategy/data modules
