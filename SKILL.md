---
name: scopeskill
description: Automate Scopevisio bookkeeping and Teamwork/CenterDevice document workflows. Use when Codex needs to authenticate against the Scopevisio REST API, obtain or refresh tokens, inspect Scopevisio business objects, call bookkeeping endpoints, or access, upload, and download Teamworkbridge documents through the Scopevisio API.
---

# scopeskill

Use Scopevisio's REST API through the repo helper first; fall back to raw `curl` only when the helper lacks a needed endpoint.

## Quick Start

1. Read `references/auth.md` before creating or refreshing tokens.
2. Read `references/bookkeeping.md` before changing contacts, products, projects, offers, orders, invoices, payments, journal data, tasks, or custom fields.
3. Read `references/teamworkbridge.md` before accessing, uploading, or downloading Teamwork/CenterDevice documents.
4. Before interpreting Kontonummern, run `sv-cli auth show` and read `SKR` from the scopeskill config. `auth login` probes and stores it; if it is missing, read `references/skr-overview.md` and only then use the documented 4400/8400 fallback heuristic.
5. Prefer the compiled `sv-cli` binary. During development, use `go run ./cmd/sv-cli ...` from this repo.

## Authentication

Never ask the user for Initial credentials in chat if the scopeskill config or an environment override already provides a REST refresh token. Use a technical Scopevisio user for automation.

One-time interactive setup:

```bash
sv-cli auth login
```

`auth login` prompts on a TTY for Kundennummer, Benutzername, Passwort, and an optional Organisations-ID. Password input is masked with `*`. It exchanges those inputs for tokens and writes `CUSTOMER` plus `REST_REFRESH_TOKEN` to the active scopeskill config. It does not store the username, password, or organisation ID.

Inspect or manage the configured REST refresh token:

```bash
sv-cli auth show    # redacted token plus source=config or source=env:SCOPESKILL_REST_REFRESH_TOKEN
sv-cli auth secret  # full token plus the same source label
sv-cli auth delete  # remove REST_REFRESH_TOKEN from the scopeskill config
```

`auth login` also runs Unternehmen probes. For bookkeeping, it stores `SKR=skr03` or `SKR=skr04` when the Mandant's standard Erlöskonto probe is conclusive.

Check the active account:

```bash
sv-cli get /myaccount
```

## Configuration

The scopeskill config is an env-file read by `sv-cli`. v1 reads these keys:

- `REST_REFRESH_TOKEN`
- `CUSTOMER`
- `SKR`
- `BASE_URL`

Environment overrides:

- `SCOPESKILL_REST_REFRESH_TOKEN`
- `SCOPESKILL_BASE_URL`
- `SCOPESKILL_CONFIG`
- `SCOPESKILL_ACCESS_TOKEN_CACHE`

`SCOPESKILL_CUSTOMER` deliberately does not exist; switch identity with `--config` or `SCOPESKILL_CONFIG` so `CUSTOMER` and `REST_REFRESH_TOKEN` stay paired (see `docs/adr/0004-customer-not-an-env-override.md`). The bearer header is always `Authorization`; there is no `AUTH_HEADER` config key or auth-header environment override (see `docs/adr/0003-drop-auth-header-configurability.md`).

The Access token cache is a separate, disposable per-fingerprint file for short-lived REST access tokens. Deleting it does not remove setup because the REST refresh token remains in the scopeskill config (see `docs/adr/0002-access-token-cache-per-refresh-token.md`).

## Bookkeeping Calls

Use the generic commands for JSON API calls:

```bash
sv-cli get /myaccount
sv-cli post /contacts --data '{"page":0,"pageSize":25}'
```

For list/search endpoints, assume Scopevisio usually expects `POST` plus a JSON search body. Keep changes narrow, verify required profiles in the live Swagger, and do not invent field names for custom fields.

For SKR-specific account interpretation, use `SKR` from config and then read `references/skr03.csv` or `references/skr04.csv`. Do not guess SKR03 vs SKR04 from a single Kontonummer if the config is missing and the 4400/8400 probe is inconclusive.

## Teamworkbridge Document Workflow

Teamworkbridge maps CenterDevice API resources under Scopevisio's `/teamworkbridge/...` path and uses the Scopevisio token.

From an already configured `sv-cli`, confirm the active refresh token source:

```bash
sv-cli auth show
```

List collections, then copy the desired `id`:

```bash
sv-cli get /teamworkbridge/collections --query all=true
```

List documents in a collection:

```bash
sv-cli get /teamworkbridge/documents \
  --query all=true \
  --query collection=<collection-id>
```

Retrieve document metadata or download the document bytes:

```bash
sv-cli get /teamworkbridge/document/<document-id>
sv-cli download /teamworkbridge/document/<document-id> --out ./document.pdf
```

If a command returns `404`, treat it as either not found or not visible to the authenticated user. Check user rights in Scopevisio under System administration -> DMS Teamwork -> Manage users.

## Live Docs

Use these sources when endpoint details matter:

- Scopevisio first steps: `https://help.scopevisio.com/en/articles/467358-rest-api-first-steps`
- Scopevisio general docs: `https://help.scopevisio.com/en/articles/467359-general-documentation`
- Swagger JSON: `https://appload.scopevisio.com/rest/swagger.json`
- Swagger UI: `https://appload.scopevisio.com/static/swagger/index.html#/`
