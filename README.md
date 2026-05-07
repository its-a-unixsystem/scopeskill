# scopeskill

Codex skill plus helper client for Scopevisio automation.

Scopevisio documents the REST API at:

- https://help.scopevisio.com/en/articles/467358-rest-api-first-steps
- https://appload.scopevisio.com/static/swagger/index.html#/

The Swagger UI is backed by:

- https://appload.scopevisio.com/rest/swagger.json

## Skill Layout

- `SKILL.md`: the trigger and operating guide for agents.
- `docs/cli-reference.md`: `sv-cli` command reference and usage examples.
- `references/auth.md`: token and login workflow.
- `references/bookkeeping.md`: Scopevisio bookkeeping object map and API guardrails.
- `references/teamworkbridge.md`: Teamwork/CenterDevice access, upload, and download workflow.
- `cmd/sv-cli/`: small Go helper CLI. Build it as `sv-cli`.
- `internal/scopeskill/`: helper client and config package used by `sv-cli`.

## Quickstart

Create a technical user in Scopevisio and give it the required licences and rights.

Build locally:

```bash
go build -o ./bin/sv-cli ./cmd/sv-cli
```

Run one-time interactive login:

```bash
./bin/sv-cli auth login
```

`auth login` asks for Kundennummer, Benutzername, Passwort, and an optional Organisations-ID; password input is masked with `*`. It writes only `CUSTOMER` and `REST_REFRESH_TOKEN` to the active scopeskill config. It probes and stores the `SKR` automatically. It never stores the initial username, password, or organisation ID.

Check authentication:

```bash
./bin/sv-cli auth show
```

Search contacts using the `sv-cli` helper:

```bash
./bin/sv-cli kontakt search --email="@example.com"
```

For a comprehensive list of all accounting, teamwork, and REST API commands available via `sv-cli`, please refer to the **[CLI Reference](docs/cli-reference.md)**.

## Configuration

The scopeskill config is an env-file. By default, `sv-cli` uses the user config directory; pass `--config <path>` or set `SCOPESKILL_CONFIG` to use a different file.

Durable config keys:

- `CUSTOMER`: the Scopevisio customer number paired with the refresh token.
- `REST_REFRESH_TOKEN`: durable credential used to obtain REST access tokens.
- `SKR`: the active chart-of-accounts standard (`skr03` or `skr04`).
- `BASE_URL`: optional Scopevisio REST base URL override.

Supported one-process environment overrides:

- `SCOPESKILL_CONFIG`
- `SCOPESKILL_REST_REFRESH_TOKEN`
- `SCOPESKILL_BASE_URL`
- `SCOPESKILL_ACCESS_TOKEN_CACHE`

`SCOPESKILL_CUSTOMER` is intentionally not supported. Switch identity with `--config` or `SCOPESKILL_CONFIG` so `CUSTOMER` and `REST_REFRESH_TOKEN` stay paired. The bearer header is always `Authorization`; there is no `AUTH_HEADER` config key or auth-header environment override.

REST access tokens are short-lived request credentials. `sv-cli` stores them in a separate disposable access-token cache, keyed by refresh-token fingerprint. REST refresh tokens are durable config credentials. Deleting the access-token cache does not remove setup; deleting `REST_REFRESH_TOKEN` from config does.

## Non-Technical Users

For out-of-the-box use, publish GitHub Releases with prebuilt binaries. The release workflow builds:

- `sv-cli-darwin-arm64` for Apple Silicon Macs
- `sv-cli-darwin-amd64` for Intel Macs
- `sv-cli-linux-amd64`
- `sv-cli-windows-amd64.exe`

A Mac user should download the matching `darwin` binary, rename it to `sv-cli`, allow it in macOS if Gatekeeper asks, and run it without installing Python, Go, or package dependencies.
