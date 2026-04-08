# Contributing

## Getting Started

- Use Go `1.25+`.
- Run `go test ./...` before opening a pull request.
- If your change affects Python workers, keep compatibility with `python/requirements.txt`.

## Configuration and Secrets

- Do not commit real trading credentials, private package indexes, or operator-specific `Host` / `Port` values.
- `.con` files are local runtime cache artifacts and should not be committed.

## Pull Requests

- Keep changes small and single-purpose.
- Describe purpose, key files changed, config/runtime impact, and test evidence.
- Update documentation when behavior, defaults, or setup flow changes.

## Licensing

- The repository is released under `BSD-3-Clause`; see `LICENSE`.
- By submitting a contribution, you agree that your changes may be distributed under the same license.

## Development Notes

- The repository defaults leave `live-md.host` unset and `live-md.port` at `0`.
- Configure real values locally before attempting `Live market data`.
