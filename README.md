# Scopevisio Skill

Codex skill plus helper client for Scopevisio automation.

Scopevisio documents the REST API at:

- https://help.scopevisio.com/en/articles/467358-rest-api-first-steps
- https://appload.scopevisio.com/static/swagger/index.html#/

The Swagger UI is backed by:

- https://appload.scopevisio.com/rest/swagger.json

## Skill Layout

- `SKILL.md`: the trigger and operating guide for agents.
- `references/auth.md`: token and login workflow.
- `references/bookkeeping.md`: Scopevisio bookkeeping object map and API guardrails.
- `references/teamworkbridge.md`: Teamwork/CenterDevice access, upload, and download workflow.
- `cmd/scopevisio/`: small dependency-free Go helper CLI. Build it as `sv-cli`.

## Setup

Create a technical user in Scopevisio and give it the required licences and rights.

Build locally:

```bash
go build -o ./bin/sv-cli ./cmd/scopevisio
```

Run one-time interactive login:

```bash
./bin/sv-cli auth login
```

`auth login` asks for Kundennummer, Benutzername, Passwort, and an optional Organisations-ID. It writes only `CUSTOMER` and `REST_REFRESH_TOKEN` to the active Scopevisio config. It never stores the initial username, password, or organisation ID.

Check authentication:

```bash
./bin/sv-cli auth show
./bin/sv-cli get /myaccount
```

Search contacts:

```bash
./bin/sv-cli post /contacts --data '{
  "page": 0,
  "pageSize": 25,
  "fields": ["id", "firstname", "lastname", "email"],
  "search": [
    {"field": "email", "operator": "is not null"}
  ]
}'
```

## Configuration

The Scopevisio config is an env-file. By default, `sv-cli` uses the user config directory; pass `--config <path>` or set `SCOPESKILL_CONFIG` to use a different file.

Durable config keys:

- `CUSTOMER`: the Scopevisio customer number paired with the refresh token.
- `REST_REFRESH_TOKEN`: durable credential used to obtain REST access tokens.
- `BASE_URL`: optional Scopevisio REST base URL override.

Supported one-process environment overrides:

- `SCOPESKILL_CONFIG`
- `SCOPESKILL_REST_REFRESH_TOKEN`
- `SCOPESKILL_BASE_URL`
- `SCOPESKILL_ACCESS_TOKEN_CACHE`

`SCOPESKILL_CUSTOMER` is intentionally not supported. Switch identity with `--config` or `SCOPESKILL_CONFIG` so `CUSTOMER` and `REST_REFRESH_TOKEN` stay paired. The bearer header is always `Authorization`; there is no `AUTH_HEADER` config key or auth-header environment override.

REST access tokens are short-lived request credentials. `sv-cli` stores them in a separate disposable access-token cache, keyed by refresh-token fingerprint. REST refresh tokens are durable config credentials. Deleting the access-token cache does not remove setup; deleting `REST_REFRESH_TOKEN` from config does.

## Teamworkbridge Smoke Test

List top-level folders for a collection:

```bash
./bin/sv-cli get /teamworkbridge/folders \
  --query parent=none \
  --query collection=<collection-id>
```

Download a document:

```bash
./bin/sv-cli download /teamworkbridge/document/<document-id> --out ./document.bin
```

Upload a document:

```bash
./bin/sv-cli teamwork upload ./invoice.pdf \
  --collection <collection-id> \
  --tag scopevisio-test
```

The current implementation keeps generic `get`, `post`, and `download` commands, plus grouped `teamwork upload` for multipart document upload. Folder and collection reads remain generic JSON calls until live Teamworkbridge tests prove a specialized command is useful.

## Non-Technical Users

For out-of-the-box use, publish GitHub Releases with prebuilt binaries. The release workflow builds:

- `sv-cli-darwin-arm64` for Apple Silicon Macs
- `sv-cli-darwin-amd64` for Intel Macs
- `sv-cli-linux-amd64`
- `sv-cli-windows-amd64.exe`

A Mac user should download the matching `darwin` binary, rename it to `sv-cli`, allow it in macOS if Gatekeeper asks, and run it without installing Python, Go, or package dependencies.

## Useful API Patterns

Most list endpoints are `POST` endpoints with a JSON search body. Common fields:

- `page`: starts at `0`
- `pageSize`: defaults to `100`, maximum `1000`
- `fields`: result fields to include
- `search`: array of `{field, value, operator}` filters
- `order`: array like `["lastname = asc"]`
- `count`: return only the matching count

Fetch the live OpenAPI document when in doubt:

```bash
curl -L https://appload.scopevisio.com/rest/swagger.json > /tmp/scopevisio-swagger.json
jq '.paths["/contacts"]' /tmp/scopevisio-swagger.json
```
