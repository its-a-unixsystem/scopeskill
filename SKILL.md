---
name: scopevisio
description: Automate Scopevisio bookkeeping and Teamwork/CenterDevice document workflows. Use when Codex needs to authenticate against the Scopevisio REST API, obtain or refresh tokens, inspect Scopevisio business objects, call bookkeeping endpoints, or access, upload, and download Teamworkbridge documents through the Scopevisio API.
---

# Scopevisio

Use Scopevisio's REST API through the repo helper first; fall back to raw `curl` only when the helper lacks a needed endpoint.

## Quick Start

1. Read `references/auth.md` before creating or refreshing tokens.
2. Read `references/bookkeeping.md` before changing contacts, products, projects, offers, orders, invoices, payments, journal data, tasks, or custom fields.
3. Read `references/teamworkbridge.md` before accessing, uploading, or downloading Teamwork/CenterDevice documents.
4. Prefer the compiled `scopevisio` binary. During development, use `go run ./cmd/scopevisio ...` from this repo.

## Authentication

Never ask the user for a password in chat if credentials can be loaded from environment variables or an existing token cache. Use a technical Scopevisio user for automation. Prefer refresh-token auth after the first login.

Minimal environment:

```bash
SCOPEVISIO_CUSTOMER=1234567
SCOPEVISIO_ORGANISATION="Example GmbH"
SCOPEVISIO_USERNAME=automation@example.com
SCOPEVISIO_PASSWORD=...
```

Create or refresh a token:

```bash
scopevisio auth
```

Check the active account:

```bash
scopevisio get /myaccount
```

## Bookkeeping Calls

Use the generic commands for JSON API calls:

```bash
scopevisio get /myaccount
scopevisio post /contacts --data '{"page":0,"pageSize":25}'
```

For list/search endpoints, assume Scopevisio usually expects `POST` plus a JSON search body. Keep changes narrow, verify required profiles in the live Swagger, and do not invent field names for custom fields.

## Teamworkbridge Test Workflow

Teamworkbridge maps CenterDevice API resources under Scopevisio's `/teamworkbridge/...` path and uses the Scopevisio token.

Retrieve document metadata or download a document:

```bash
scopevisio get /teamworkbridge/document/<document-id>
scopevisio download /teamworkbridge/document/<document-id> --out ./document.bin
```

List top-level folders for a collection:

```bash
scopevisio get /teamworkbridge/folders \
  --query parent=none \
  --query collection=<collection-id>
```

Upload a document:

```bash
scopevisio teamwork-upload ./invoice.pdf \
  --collection <collection-id> \
  --tag scopevisio-test
```

If a command returns `404`, treat it as either not found or not visible to the authenticated user. Check user rights in Scopevisio under System administration -> DMS Teamwork -> Manage users.

## Live Docs

Use these sources when endpoint details matter:

- Scopevisio first steps: `https://help.scopevisio.com/en/articles/467358-rest-api-first-steps`
- Scopevisio general docs: `https://help.scopevisio.com/en/articles/467359-general-documentation`
- Swagger JSON: `https://appload.scopevisio.com/rest/swagger.json`
- Swagger UI: `https://appload.scopevisio.com/static/swagger/index.html#/`
