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
- `cmd/scopevisio/`: small dependency-free Go helper CLI.

## Setup

Create a technical user in Scopevisio, give it the required licences and rights,
then configure credentials through environment variables:

```bash
cp .env.example .env
set -a
. ./.env
set +a
```

Build locally:

```bash
go build -o ./bin/scopevisio ./cmd/scopevisio
```

Check authentication:

```bash
./bin/scopevisio auth
./bin/scopevisio get /myaccount
```

Search contacts:

```bash
./bin/scopevisio post /contacts --data '{
  "page": 0,
  "pageSize": 25,
  "fields": ["id", "firstname", "lastname", "email"],
  "search": [
    {"field": "email", "operator": "is not null"}
  ]
}'
```

## Teamworkbridge Smoke Test

List top-level folders for a collection:

```bash
./bin/scopevisio get /teamworkbridge/folders \
  --query parent=none \
  --query collection=<collection-id>
```

Download a document:

```bash
./bin/scopevisio download /teamworkbridge/document/<document-id> --out ./document.bin
```

Upload a document:

```bash
./bin/scopevisio teamwork-upload ./invoice.pdf \
  --collection <collection-id> \
  --tag scopevisio-test
```

## Non-Technical Users

For out-of-the-box use, publish GitHub Releases with prebuilt binaries. The
release workflow builds:

- `scopevisio-darwin-arm64` for Apple Silicon Macs
- `scopevisio-darwin-amd64` for Intel Macs
- `scopevisio-linux-amd64`
- `scopevisio-windows-amd64.exe`

A Mac user should download the matching `darwin` binary, rename it to
`scopevisio`, allow it in macOS if Gatekeeper asks, and run it without installing
Python, Go, or package dependencies.

## Authentication Notes

Scopevisio supports `password`, `refresh_token`, and `authorization_code` grant
types on `POST /token`. For long-running automation, use a technical user and
move to refresh-token based authentication after the initial token creation.

By default, tokens are cached at:

```text
~/.config/scopeskill/token.json
```

Override that with:

```bash
SCOPEVISIO_TOKEN_CACHE=/path/to/token.json
```

The client sends bearer tokens in the standard `Authorization` header. If a
specific Scopevisio environment expects the spelling used in some help examples,
set:

```bash
SCOPEVISIO_AUTH_HEADER=Authorisation
```

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
