# Scopeskill

A claude/codex AI skill (plus helper client) for accessing and automating the bookkeeping system [Scopevisio](https://www.scopevisio.com/).

Scopevisio's REST API is documents at:

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
- `references/workflows/`: step-by-step SOPs (incoming invoices, account movements, reconciliations, etc.).
- `references/workflows/cleanup-checklist.md`: preflight and postcondition checklist for cleanup and reconciliation.
- `cmd/sv-cli/`: small Go helper CLI. Build it as `sv-cli`.
- `internal/scopeskill/`: helper client and config package used by `sv-cli`.

## Quickstart

### Create a technical user in Scopevisio

Give the user the required licences and rights.


### Get the binary

You find it under the [latest release](https://github.com/its-a-unixsystem/scopeskill/releases/tag/v0.9).

> [!NOTE]
> You can build the binary locally:
>
> ```bash
> go build -o ./bin/sv-cli ./cmd/sv-cli
> ```

### Authenticate

Choose one of these authentication methods.

The interactive login is easier but requires you to enter your password. If you want to generate the token yourself you can do this Scopevisio's webpage.

### Scopevisio-generated token
Follow [Scopevisio's REST API instructions](https://help.scopevisio.com/de/articles/467358-rest-api-erste-schritte) and generate tokens through its Swagger UI or documented `curl` request. The response contains a short-lived `access_token` and a long-lived `refresh_token`.

You then write the config:

```bash
sv-cli auth import
```

It will ask you for your Kundennummer and the refresh token and writes the config

### Interactive login with login and password

Run the one-time login:

```bash
sv-cli auth login
```

The command asks for:

1. Kundennummer — required
2. Benutzername — required
3. Passwort — required and masked while typing
4. Organisations-ID — optional

It stores `CUSTOMER` and `REST_REFRESH_TOKEN` in the active scopeskill config and detects `SKR` automatically.

> [!WARNING]
> It never stores the username, password, or organisation ID.

### Verify authentication

Check authentication:

```bash
sv-cli auth show
```

Search contacts using the `sv-cli` helper:

```bash
sv-cli kontakt search --email="@example.com"
```

For a comprehensive list of all accounting, teamwork, and REST API commands available via `sv-cli`, please refer to the **[CLI Reference](docs/cli-reference.md)**.

> [!NOTE]
> sv-cli is focussed on agentic bookkeeping, so the output is usually always JSON:
>
> ```bash
> $ bin/sv-cli sachkonto search --number-prefix=1800
> [
>   {
>     "accountTypeName": "Aktiv/Passiv",
>     "active": true,
>     "name": "Bank",
>     "number": "1800"
>   }
> ]
> ```

## SKILL

Place the repository files in the `skills` directory either of the local project (`.agents/skills` or `.claude/skills`).

Easie is to use vercels skills tool:

```bash
npx skills add https://github.com/its-a-unixsystem/scopeskill --skill scopeskill
```

## First steps

Make sure your agent loads the skill correctly, then simply ask questions:

```bash
What are the last 10 transactions on 1800 ?
```

```bash
Please list all booked invoices from Google and verify that they are correct.
```


## Configuration details

The configuration is short:

```ini
# scopeskill config — managed by 'sv-cli auth login'
  {
    "accountTypeName": "Aktiv/Passiv",
    "active": true,
    "name": "Bank",
    "number": "1800"
  }
]
```


## SKILL

Place the repository files in the `skills` directory either of the local project (`.agents/skills` or `.claude/skills`).

Easie is to use vercels skills tool:

```bash
npx skills add https://github.com/its-a-unixsystem/scopeskill --skill scopeskill
```

## Configuration details

The configuration is short:

```ini
# scopeskill config — managed by 'sv-cli auth login'
CUSTOMER=12345678
REST_REFRESH_TOKEN=aaaaaaaa-bbbbbbb-cccc-dddd-eeee-ffffffffffff
SKR=skr04
```

By default, `sv-cli` uses the user config directory; pass `--config <path>` or set `SCOPESKILL_CONFIG` to use a different file.

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

### config location

| OS      | Default path                                                                             |
|---------|------------------------------------------------------------------------------------------|
| Linux   | `$XDG_CONFIG_HOME/scopeskill/config`, falling back to `~/.config/scopeskill/config`          |
| macOS   | `~/Library/Application Support/scopeskill/config`                                          |
| Windows | `%AppData%\scopeskill\config` (typically `C:\Users\<you>\AppData\Roaming\scopeskill\config`) |


## License

[AGPL-3.0](LICENSE)
