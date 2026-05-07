# Scopevisio Skill

This context describes the product language for the Scopevisio automation skill and its helper executable.

## Language

**Scopevisio Skill**:
The packaged unit that combines agent-facing instructions, Scopevisio references, and the helper executable.
_Avoid_: Skill when referring only to the executable

**Codex skill**:
The agent-facing instructions and bundled references that teach Codex how to work with Scopevisio.
_Avoid_: CLI, binary

**Scopevisio CLI**:
The executable helper that performs Scopevisio authentication, JSON API calls, Teamworkbridge downloads, and Teamworkbridge uploads.
_Avoid_: Skill

**Helper tool**:
A scriptable executable intended to be called by agents and other automation tools, while remaining usable by humans when needed.
_Avoid_: Interactive app, wizard

**Initial credentials**:
The Scopevisio customer number, username, password, and optional organisation ID used only to obtain a token. Two-factor authentication is not handled in the first implementation; automation should use a technical user without 2FA.
_Avoid_: Saved login, stored password

**Customer number**:
The Scopevisio customer identifier required by token creation and refresh-token exchange.
_Avoid_: Secret

**REST access token**:
The short-lived bearer credential sent on Scopevisio REST API requests.
_Avoid_: Password, login, refresh token

**Access token cache**:
A disposable local cache file containing a **REST access token** and its expiry metadata.
_Avoid_: Scopevisio config

**REST refresh token**:
The long-lived credential used by the **Scopevisio CLI** to obtain new **REST access tokens** without storing **Initial credentials**.
_Avoid_: Password, login, access token

**Token source**:
A location where the **Scopevisio CLI** may read a **REST refresh token** for normal automation.
_Avoid_: Login source

**Scopevisio config**:
A machine-facing env-file used by the **Scopevisio CLI** that may contain `REST_REFRESH_TOKEN`, `CUSTOMER`, and helper defaults such as base URL.
_Avoid_: Token cache, password store

**Config override**:
An explicit command-line option that points the **Scopevisio CLI** at a non-default **Scopevisio config** file.
_Avoid_: Project-local auto-discovery

**Environment override**:
A `SCOPESKILL_*` environment variable that overrides the corresponding **Scopevisio config** key for one process.
_Avoid_: Scopevisio environment variable

**Auth login**:
The explicit interactive setup action that collects **Initial credentials**, exchanges them for a **REST access token** and **REST refresh token**, and writes the **Scopevisio config**.
_Avoid_: Implicit login, automatic login

**Auth show**:
The action that displays a redacted view of the configured **REST refresh token**.
_Avoid_: Login

**Auth secret**:
The action that displays the full configured **REST refresh token**.
_Avoid_: Show

**Auth delete**:
The action that removes the configured **REST refresh token**.
_Avoid_: Logout when only the local token is removed

**Teamwork document**:
A remote CenterDevice document accessed through Scopevisio Teamworkbridge.
_Avoid_: File, doc

**Local file**:
The byte payload on disk that can be uploaded to or downloaded from a **Teamwork document**.
_Avoid_: Teamwork document

## Relationships

- A **Scopevisio Skill** contains exactly one **Codex skill**.
- A **Scopevisio Skill** bundles one **Scopevisio CLI** for deterministic API operations.
- A **Codex skill** instructs agents when to call the **Scopevisio CLI**.
- The **Scopevisio CLI** is a **Helper tool** with agents as the primary caller.
- **Initial credentials** may be used to obtain a **REST access token** and **REST refresh token**, but must not be stored by the **Scopevisio CLI**.
- Normal automation should read a **REST refresh token** from configured **Token sources** instead of asking for **Initial credentials**.
- A **Scopevisio config** stores the preferred `REST_REFRESH_TOKEN` and helper defaults for the **Scopevisio CLI**.
- A **Scopevisio config** stores `CUSTOMER` with `REST_REFRESH_TOKEN` because refresh-token exchange requires the **Customer number**.
- **Initial credentials** are collected by **Auth login** and are not read from **Scopevisio config** in the first implementation.
- `ORGANISATION_ID` may be collected for **Auth login** but is optional and is not stored in **Scopevisio config**.
- A **Scopevisio config** uses simple env-file syntax for CLI parsing, not as a human-friendly preferences format.
- The grammar is deliberately tiny: each line is blank, a `#`-prefixed comment (whitespace-then-`#` only, never inline), or `KEY=VALUE`. `KEY` matches `[A-Z_][A-Z0-9_]*` with no whitespace around `=`. `VALUE` is the literal text from after `=` to end-of-line with surrounding whitespace trimmed; no quoting, no escapes, no `$VAR` interpolation.
- Unknown keys are silently ignored on read and preserved on rewrite. Duplicate keys: last wins on read; `auth login` deduplicates on write.
- `auth login` writes keys in a stable order under a single header comment line, preserves unknown keys and prior comments, and keeps the file at mode `0600`.
- `SCOPESKILL_*` environment variables override matching **Scopevisio config** values, including `REST_REFRESH_TOKEN`.
- `--config` selects the **Scopevisio config** path; `SCOPESKILL_CONFIG` is the environment fallback for that path.
- `SCOPESKILL_REST_REFRESH_TOKEN` and `SCOPESKILL_BASE_URL` are **Environment overrides** for `REST_REFRESH_TOKEN` and `BASE_URL`.
- The default `BASE_URL` is `https://appload.scopevisio.com`, the documented Scopevisio REST host. No regional or staging detection is built in; alternative endpoints are reached by overriding the value.
- `CUSTOMER` is not exposed as a standalone env override: the **REST refresh token** is minted for a specific customer, so varying `CUSTOMER` independently is a footgun. To switch identity wholesale, use `--config` or `SCOPESKILL_CONFIG` to point at a different **Scopevisio config** file.
- `SCOPESKILL_REST_REFRESH_TOKEN` only works when the active **Scopevisio config** also contains the matching `CUSTOMER`, because the refresh exchange requires both.
- The bearer auth header is hardcoded to `Authorization` because Scopevisio's Swagger and first-steps docs both specify `Authorization: Bearer <token>`; no override is exposed.
- The default **Scopevisio config** belongs in the user's config directory; project-local secret files are not auto-discovered.
- A **Config override** may point the **Scopevisio CLI** at a different config file for a specific run.
- **Auth login** is the only interactive command and is responsible for writing `REST_REFRESH_TOKEN` to **Scopevisio config**.
- **Auth login** is TTY-prompt only in the first implementation; non-interactive flags or `SCOPESKILL_LOGIN_*` env input are deferred until there is a concrete unattended-setup need.
- **Auth login** prompts for Kundennummer, Benutzername, Passwort, and an optional Organisations-ID; password input is masked with `*`.
- **Auth login** refuses to overwrite an existing `REST_REFRESH_TOKEN` in **Scopevisio config** unless `--force` is passed.
- **Auth login** warns when `SCOPESKILL_REST_REFRESH_TOKEN` is set in the environment, because that **Environment override** would shadow the freshly written token.
- `auth` without a subcommand outputs one-line help for the auth command group.
- **Auth show** displays a redacted **REST refresh token**, labelled with its source (`config` or `env:SCOPESKILL_REST_REFRESH_TOKEN`), and reflects the effective token that normal API calls would use.
- **Auth secret** displays the full effective **REST refresh token**, labelled with its source.
- **Auth delete** removes `REST_REFRESH_TOKEN` from **Scopevisio config** and warns when `SCOPESKILL_REST_REFRESH_TOKEN` is set in the environment, because the deletion does not take effect for the next call in that case.
- Normal API calls use short-lived **REST access tokens** internally.
- Normal API calls may reuse a valid **REST access token** from the **Access token cache**.
- The **Access token cache** is separate from **Scopevisio config** and can be deleted without losing setup.
- The default **Access token cache** lives in the user's cache directory and can be overridden with `SCOPESKILL_ACCESS_TOKEN_CACHE`.
- The **Access token cache** filename is derived from the **REST refresh token** fingerprint (first 16 hex of SHA-256), so different refresh tokens (different customers / configs) cannot cross-pollute caches.
- The cache directory is created with mode `0700` and cache files with mode `0600`, mirroring `.ssh` conventions; the **Scopevisio config** file uses the same restrictive permissions because it stores the **REST refresh token**.
- Agents should advise the user to run **Auth login** when no **REST refresh token** exists or the configured **REST refresh token** is invalid.
- Invalid-token errors should recommend **Auth login** without deleting the existing **REST refresh token** automatically.
- On a 401/403 from the refresh-token exchange (or on a 401 from an API call made with a freshly minted **REST access token**), the **Scopevisio CLI** deletes the **Access token cache** file, leaves the **REST refresh token** in **Scopevisio config**, and exits non-zero with a message recommending **Auth login**.
- On a 5xx or network failure during the refresh-token exchange, the **Scopevisio CLI** leaves both the **Access token cache** and **Scopevisio config** untouched and exits non-zero with a transient-error message; it does not conflate Scopevisio outages with revoked tokens.
- A **Teamwork document** is the remote object; a **Local file** is the on-disk content uploaded or downloaded.
- Teamwork folders are accessed through generic JSON calls in the first implementation.
- `download <path> --out` is a generic binary GET and is not Teamwork-specific.
- Teamwork-specific operations that need bespoke flags or formatting (currently only multipart upload) live under the `teamwork` subcommand group, e.g. `sv-cli teamwork upload`.

## Example dialogue

> **Dev:** "Should the **Codex skill** handle multipart upload details in prose?"
> **Domain expert:** "No. The **Codex skill** should tell the agent to use the **Scopevisio CLI**, because the executable owns that fragile API formatting."
>
> **Dev:** "Should the **Scopevisio CLI** ask questions interactively when configuration is missing?"
> **Domain expert:** "No. It is a **Helper tool**. It should fail clearly so the calling agent or automation can decide what to do next."
>
> **Dev:** "Can we save the password after `auth` succeeds?"
> **Domain expert:** "No. Those are **Initial credentials**. Save or configure the resulting **REST refresh token**, never the login password."

## Flagged ambiguities

- "skill" was used to mean both the whole package and the executable helper. Resolved: use **Scopevisio Skill** for the package and **Scopevisio CLI** for the executable.
- "CLI for non-technical users" could imply an interactive app. Resolved: the **Scopevisio CLI** is a **Helper tool** for agents and automation first, with direct human use as a fallback.
- "credentials" was used for both password login and API access. Resolved: **Initial credentials** are transient login inputs; **REST refresh token** is the reusable durable API credential.
- "REST token" was initially treated as singular. Resolved: Scopevisio returns a short-lived **REST access token** and a long-lived **REST refresh token**; the helper stores the refresh token and uses access tokens internally.
- Environment override names were initially described with a `SCOPEVISIO_` prefix. Resolved: use `SCOPESKILL_*` because the variables belong to this helper, not the Scopevisio product.
- "doc" and "file" were used casually for Teamworkbridge content. Resolved: use **Teamwork document** for the remote CenterDevice object and **Local file** for bytes on disk.
- Teamwork upload/download command grouping. Resolved: hybrid. Generic `download <path>` stays top-level (it is just a binary GET); Teamwork-specific operations with bespoke flags (currently only multipart upload) live under `teamwork ...`.
