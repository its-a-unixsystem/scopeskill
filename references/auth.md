# Authentication

Scopevisio uses OAuth2-style bearer tokens for REST calls.

## Recommended Flow

1. Use a technical Scopevisio user for automation.
2. Create an initial token with `POST /token`.
3. Store the returned refresh token outside the chat transcript.
4. Use `grant_type=refresh_token` for ongoing automation.

The helper caches tokens at `~/.config/scopeskill/token.json` unless `SCOPEVISIO_TOKEN_CACHE` is set.

## Environment

```bash
SCOPEVISIO_CUSTOMER=1234567
SCOPEVISIO_ORGANISATION="Example GmbH"
SCOPEVISIO_USERNAME=automation@example.com
SCOPEVISIO_PASSWORD=...
SCOPEVISIO_REFRESH_TOKEN=...
SCOPEVISIO_CLIENT_ID=...
SCOPEVISIO_CLIENT_SECRET=...
SCOPEVISIO_TOKEN_CACHE=~/.config/scopeskill/token.json
```

`SCOPEVISIO_CUSTOMER`, `SCOPEVISIO_USERNAME`, and `SCOPEVISIO_PASSWORD` are enough for the password grant. `SCOPEVISIO_REFRESH_TOKEN` plus `SCOPEVISIO_CUSTOMER` is enough for refresh-token auth.

Scopevisio's help text shows the header spelling `Authorisation` in one example, while the OpenAPI security scheme uses a normal bearer token. The helper defaults to `Authorization`. Set `SCOPEVISIO_AUTH_HEADER=Authorisation` only if a specific environment requires that spelling.

## Commands

```bash
scopevisio auth
scopevisio auth --show-token
scopevisio auth --totp 123456
scopevisio get /myaccount
```

Do not print tokens unless the user explicitly asks for it. When reporting auth results, redact `access_token` and `refresh_token`.
