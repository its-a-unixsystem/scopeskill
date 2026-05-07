# Authentication

Scopevisio uses OAuth2-style bearer tokens for REST calls.

## Credential Vocabulary

- Initial credentials: Kundennummer, Benutzername, Passwort, and an optional Organisations-ID; password input is masked with `*`. `sv-cli auth login` collects these from a TTY and never stores them.
- REST refresh token: durable config credential stored as `REST_REFRESH_TOKEN` with the paired `CUSTOMER` customer number.
- REST access token: short-lived request credential obtained from the refresh token and stored only in the disposable access-token cache.

## Recommended Flow

1. Use a technical Scopevisio user for automation.
2. Run `sv-cli auth login` once in a terminal.
3. Use refresh-token based auth for ongoing automation.
4. Use `sv-cli auth show` to inspect setup without printing the full token.
5. Use `sv-cli auth secret` only when the user explicitly asks for the full refresh token.

## Scopevisio Config

The Scopevisio config is an env-file read by `sv-cli`. The default path comes from the user config directory. Pass `--config <path>` or set `SCOPESKILL_CONFIG` to use another config file.

Durable config keys:

```dotenv
CUSTOMER=1234567
REST_REFRESH_TOKEN=...
BASE_URL=https://appload.scopevisio.com/rest
```

`BASE_URL` is optional. `auth login` writes `CUSTOMER` and `REST_REFRESH_TOKEN`; it preserves unrelated config keys and comments where possible.

## Environment Overrides

Supported one-process overrides:

```bash
SCOPESKILL_CONFIG=/path/to/config
SCOPESKILL_REST_REFRESH_TOKEN=...
SCOPESKILL_BASE_URL=https://appload.scopevisio.com/rest
SCOPESKILL_ACCESS_TOKEN_CACHE=/path/to/access-token-cache.json
```

`SCOPESKILL_REST_REFRESH_TOKEN` shadows the config refresh token for that process. `SCOPESKILL_CUSTOMER` is intentionally unsupported so customer number and refresh token stay paired in one config file. The bearer header is always `Authorization`; there is no auth-header override.

## Access-Token Cache

The access-token cache is separate from the Scopevisio config. It stores short-lived REST access tokens with expiry metadata and uses a refresh-token fingerprint in the default cache filename. Deleting the cache only forces the next command to refresh a new REST access token.

## Commands

```bash
sv-cli auth login
sv-cli auth show
sv-cli auth secret
sv-cli auth delete
sv-cli get /myaccount
```

Do not print tokens unless the user explicitly asks for it. When reporting auth results, redact `REST_REFRESH_TOKEN`, `access_token`, and `refresh_token`.
