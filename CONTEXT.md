# scopeskill

This context describes the product language for scopeskill and its helper executable.

## Language

**scopeskill**:
The packaged unit that combines agent-facing instructions, Scopevisio references, and the helper executable.
_Avoid_: Skill when referring only to the executable

**Codex skill**:
The agent-facing instructions and bundled references that teach Codex how to work with Scopevisio.
_Avoid_: CLI, binary

**`sv-cli`**:
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
_Avoid_: scopeskill config

**REST refresh token**:
The long-lived credential used by the **`sv-cli`** to obtain new **REST access tokens** without storing **Initial credentials**.
_Avoid_: Password, login, access token

**Token source**:
A location where the **`sv-cli`** may read a **REST refresh token** for normal automation.
_Avoid_: Login source

**scopeskill config**:
A machine-facing env-file used by the **`sv-cli`** that may contain `REST_REFRESH_TOKEN`, `CUSTOMER`, and helper defaults such as base URL.
_Avoid_: Token cache, password store

**Config override**:
An explicit command-line option that points the **`sv-cli`** at a non-default **scopeskill config** file.
_Avoid_: Project-local auto-discovery

**Environment override**:
A `SCOPESKILL_*` environment variable that overrides the corresponding **scopeskill config** key for one process.
_Avoid_: Scopevisio environment variable

**Auth login**:
The explicit interactive setup action that collects **Initial credentials**, exchanges them for a **REST access token** and **REST refresh token**, and writes the **scopeskill config**.
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

**Unternehmen**:
The Scopevisio organisation/tenant scope of an authenticated session, paired with the `CUSTOMER` and `REST_REFRESH_TOKEN` in **scopeskill config**.
_Avoid_: Mandant, tenant, organisation when the Scopevisio billing scope is meant

**Unternehmen probe**:
A deterministic API call run during **Auth login** to derive a fixed **Unternehmen** attribute (e.g. **SKR**) from the live Scopevisio API and persist it to **scopeskill config**.
_Avoid_: Login probe, account probe

**SKR**:
The chart-of-accounts standard (`SKR03` or `SKR04`) of the **Unternehmen**, persisted in **scopeskill config** and used by accounting commands to interpret Kontonummern.
_Avoid_: Kontenrahmen when the standard rather than the live chart is meant

**Sachkonto**:
A Scopevisio impersonal G/L account under the **Unternehmen**'s chart, identified by Kontonummer, with no link to a **Kontakt**.
_Avoid_: G/L account, ledger account when speaking with users

**Kontakt**:
The Scopevisio master-directory entity (`/contacts`) that owns identifying details (name, address, USt-ID) for both **Debitor** and **Kreditor** accounts.
_Avoid_: Customer, supplier when only the linked party is meant

**Personenkonto**:
A subledger account linked to exactly one **Kontakt**, either as a **Debitor** or a **Kreditor**.
_Avoid_: Subledger account

**Debitor**:
An accounts-receivable Konto attached to exactly one **Kontakt**, receiving postings for customer-side transactions.
_Avoid_: Customer account when the **Kontakt** is meant

**Kreditor**:
An accounts-payable Konto attached to exactly one **Kontakt**, receiving postings for supplier-side transactions.
_Avoid_: Vendor account when the **Kontakt** is meant

**Sammelkonto**:
The impersonal G/L account (**Sachkonto**) linked to a **Debitor** or **Kreditor** where aggregate subledger balances are posted in the General Ledger.
_Avoid_: Collective account, summary account

**SuSa**:
The Scopevisio Summen- und Saldenliste report providing aggregate opening balances, debit/credit totals, and closing balances for accounts over a date range.
_Avoid_: Trial balance, balance sheet

**Saldo**:
The balance figures of an account over a specified date range, sourced from the **SuSa**.
_Avoid_: Account balance

**Nummernkreis**:
A configured sequential number range in Scopevisio from which **Debitor**, **Kreditor**, or **Beleg** numbers are automatically generated.
_Avoid_: Number range, account range

**Fiskaljahr**:
A designated financial accounting year comprising sequential **Buchungsperioden**, having an open or closed status that governs whether **Buchungen** or settlement operations may be posted.
_Avoid_: Fiscal year when speaking in domain context, calendar year

**Buchungsperiode**:
A discrete monthly or year-end closing time window within a **Fiskaljahr** against which **Buchungen** are recorded; postings cannot mutate closed periods.
_Avoid_: Accounting period, booking month

**Buchung**:
A single posting in the **Journal**: at minimum a Soll/Haben pair on Konten with an amount and a Buchungsdatum, plus optional Steuerschlüssel and Dimensionen.
_Avoid_: Posting line, journal entry

**Kasse**:
A Scopevisio cash ledger identified by its Ref-Nummer (`cashbookId`).
_Avoid_: Cashbook

**Kassenbuchung**:
An Einnahme or Ausgabe recorded in a **Kasse** with a Belegdatum and Gegenkonto.
_Avoid_: Cashbook entry

**Journal**:
The chronological sequence of general, impersonal ledger postings for a Fiskaljahr, queried through `sv-cli journal search` (`/journal`); postings touching customer or supplier accounts appear here only via aggregate **Sammelkonto** rows.
_Avoid_: Ledger, Sachjournal, general ledger

**Personenjournal**:
The chronological sequence of subledger postings on **Personenkonten** for a Fiskaljahr, queried through `sv-cli personenkonto journal` (`/personaljournal`), containing the customer and supplier contra rows.
_Avoid_: Personal journal, subledger journal

**Storno**:
A cancellation **Buchung** that reverses an original **Buchung**: the provider links it to the original, and its rows are an exact sign reversal of the original's rows (same accounts, posting date, and document numbers, negated amounts). An original **Buchung** has at most one **Storno**.
_Avoid_: Reversal entry, negative posting

**Korrekturbuchung**:
A reviewed replacement **Buchung** that, together with the **Storno**, supersedes an erroneous original: the original remains linked to its **Storno**, and the **Korrekturbuchung** carries the corrected content as a new **Buchung**.
_Avoid_: Correction posting, amendment

**Beleg**:
The document underlying one or more **Buchungen**, identified by Belegnummer. Examples include **Eingangsrechnung** and **Gutschrift**.
_Avoid_: Voucher, document when the Scopevisio document type is meant

**Beleg-Status**:
A **Beleg** (like an **Eingangsrechnung**) progresses through three independent state machines: `contentState` (sachliche Prüfung), `paymentState` (Zahlungsfreigabe), and `postingState` (Verbuchung). The CLI filters on the raw IDs (e.g. `postingStateId=3`) rather than mapping the string names.
_Avoid_: Workflow state, approval status when a specific state machine is meant

**Eingangsrechnung**:
A specific kind of **Beleg** representing an incoming invoice from a supplier (**Kreditor**).
_Avoid_: Incoming invoice

**Gutschrift**:
A specific kind of **Beleg** representing a credit note.
_Avoid_: Credit, credit note

**Offene Posten**:
The list of unsettled **Belege** on either the debitor side (Forderungen) or the kreditor side (Verbindlichkeiten); the side is always specified explicitly when querying.
_Avoid_: Open invoices, OPs

**DATEV EXTF**:
The standard CSV-based exchange format used to transfer accounting data between Scopevisio and a Steuerberater.
_Avoid_: DATEV format (when the specific EXTF variant is meant)

**BWA**:
Betriebswirtschaftliche Auswertung. The standard short-term financial report used by SMEs to assess current profitability, distinct from the statutory **Bilanz** and **GuV**.
_Avoid_: Business evaluation, P&L

**Bilanz**:
The balance sheet report showing assets and liabilities at a specific date.
_Avoid_: Balance sheet

**GuV**:
Gewinn- und Verlustrechnung. The profit and loss statement over a period.
_Avoid_: P&L, income statement

**Reisekosten**:
Travel expenses comprising different position types like **Nebenkosten** (incidentals), **Übernachtung** (accommodation), and **Fahrtkosten** (mileage/transport).
_Avoid_: Travel entries

## Relationships

- A **scopeskill** contains exactly one **Codex skill**.
- A **scopeskill** bundles one **`sv-cli`** for deterministic API operations.
- A **Codex skill** instructs agents when to call the **`sv-cli`**.
- The **`sv-cli`** is a **Helper tool** with agents as the primary caller.
- **Initial credentials** may be used to obtain a **REST access token** and **REST refresh token**, but must not be stored by the **`sv-cli`**.
- Normal automation should read a **REST refresh token** from configured **Token sources** instead of asking for **Initial credentials**.
- A **scopeskill config** stores the preferred `REST_REFRESH_TOKEN` and helper defaults for the **`sv-cli`**.
- A **scopeskill config** stores `CUSTOMER` with `REST_REFRESH_TOKEN` because refresh-token exchange requires the **Customer number**.
- **Initial credentials** are collected by **Auth login** and are not read from **scopeskill config** in the first implementation.
- `ORGANISATION_ID` may be collected for **Auth login** but is optional and is not stored in **scopeskill config**.
- A **scopeskill config** uses simple env-file syntax for CLI parsing, not as a human-friendly preferences format.
- The grammar is deliberately tiny: each line is blank, a `#`-prefixed comment (whitespace-then-`#` only, never inline), or `KEY=VALUE`. `KEY` matches `[A-Z_][A-Z0-9_]*` with no whitespace around `=`. `VALUE` is the literal text from after `=` to end-of-line with surrounding whitespace trimmed; no quoting, no escapes, no `$VAR` interpolation.
- Unknown keys are silently ignored on read and preserved on rewrite. Duplicate keys: last wins on read; `auth login` deduplicates on write.
- `auth login` writes keys in a stable order under a single header comment line, preserves unknown keys and prior comments, and keeps the file at mode `0600`.
- `SCOPESKILL_*` environment variables override matching **scopeskill config** values, including `REST_REFRESH_TOKEN`.
- `--config` selects the **scopeskill config** path; `SCOPESKILL_CONFIG` is the environment fallback for that path.
- `SCOPESKILL_REST_REFRESH_TOKEN` and `SCOPESKILL_BASE_URL` are **Environment overrides** for `REST_REFRESH_TOKEN` and `BASE_URL`.
- The default `BASE_URL` is `https://appload.scopevisio.com`, the documented Scopevisio REST host. No regional or staging detection is built in; alternative endpoints are reached by overriding the value.
- `CUSTOMER` is not exposed as a standalone env override: the **REST refresh token** is minted for a specific customer, so varying `CUSTOMER` independently is a footgun. To switch identity wholesale, use `--config` or `SCOPESKILL_CONFIG` to point at a different **scopeskill config** file.
- `SCOPESKILL_REST_REFRESH_TOKEN` only works when the active **scopeskill config** also contains the matching `CUSTOMER`, because the refresh exchange requires both.
- The bearer auth header is hardcoded to `Authorization` because Scopevisio's Swagger and first-steps docs both specify `Authorization: Bearer <token>`; no override is exposed.
- The default **scopeskill config** belongs in the user's config directory; project-local secret files are not auto-discovered.
- A **Config override** may point the **`sv-cli`** at a different config file for a specific run.
- **Auth login** is the only interactive command and is responsible for writing `REST_REFRESH_TOKEN` to **scopeskill config**.
- **Auth login** is TTY-prompt only in the first implementation; non-interactive flags or `SCOPESKILL_LOGIN_*` env input are deferred until there is a concrete unattended-setup need.
- **Auth login** prompts for Kundennummer, Benutzername, Passwort, and an optional Organisations-ID; password input is masked with `*`.
- **Auth login** refuses to overwrite an existing `REST_REFRESH_TOKEN` in **scopeskill config** unless `--force` is passed.
- **Auth login** warns when `SCOPESKILL_REST_REFRESH_TOKEN` is set in the environment, because that **Environment override** would shadow the freshly written token.
- `auth` without a subcommand outputs one-line help for the auth command group.
- **Auth show** displays a redacted **REST refresh token**, labelled with its source (`config` or `env:SCOPESKILL_REST_REFRESH_TOKEN`), and reflects the effective token that normal API calls would use.
- **Auth secret** displays the full effective **REST refresh token**, labelled with its source.
- **Auth delete** removes `REST_REFRESH_TOKEN` from **scopeskill config** and warns when `SCOPESKILL_REST_REFRESH_TOKEN` is set in the environment, because the deletion does not take effect for the next call in that case.
- Normal API calls use short-lived **REST access tokens** internally.
- Normal API calls may reuse a valid **REST access token** from the **Access token cache**.
- The **Access token cache** is separate from **scopeskill config** and can be deleted without losing setup.
- The default **Access token cache** lives in the user's cache directory and can be overridden with `SCOPESKILL_ACCESS_TOKEN_CACHE`.
- The **Access token cache** filename is derived from the **REST refresh token** fingerprint (first 16 hex of SHA-256), so different refresh tokens (different customers / configs) cannot cross-pollute caches.
- The cache directory is created with mode `0700` and cache files with mode `0600`, mirroring `.ssh` conventions; the **scopeskill config** file uses the same restrictive permissions because it stores the **REST refresh token**.
- Agents should advise the user to run **Auth login** when no **REST refresh token** exists or the configured **REST refresh token** is invalid.
- Invalid-token errors should recommend **Auth login** without deleting the existing **REST refresh token** automatically.
- On a 401/403 from the refresh-token exchange (or on a 401 from an API call made with a freshly minted **REST access token**), the **`sv-cli`** deletes the **Access token cache** file, leaves the **REST refresh token** in **scopeskill config**, and exits non-zero with a message recommending **Auth login**.
- On a 5xx or network failure during the refresh-token exchange, the **`sv-cli`** leaves both the **Access token cache** and **scopeskill config** untouched and exits non-zero with a transient-error message; it does not conflate Scopevisio outages with revoked tokens.
- A **Teamwork document** is the remote object; a **Local file** is the on-disk content uploaded or downloaded.
- `sv-cli datev export` writes the `/datevexport` response bytes unchanged, and
  `sv-cli datev import` sends Local file bytes in the API-required Base64 JSON
  envelope without parsing or validating DATEV EXTF; Scopevisio validation
  error response bodies are preserved in the returned API error.
- `sv-cli bericht show` writes the `/proreport` JSON response unchanged, while
  `sv-cli bericht export` writes `/reports/{type}` CSV or PDF bytes unchanged;
  neither command derives, flattens, or interprets BWA, Bilanz, or GuV content.
- Teamwork folders are accessed through generic JSON calls in the first implementation.
- `download <path> --out` is a generic binary GET and is not Teamwork-specific.
- Teamwork-specific operations that need bespoke flags or formatting (currently only multipart upload) live under the `teamwork` subcommand group, e.g. `sv-cli teamwork upload`.
- An **Unternehmen probe** runs during **Auth login** after token exchange and writes its result (e.g. `SKR`) to **scopeskill config**; re-running **Auth login** re-probes and overwrites the stored value.
- The first **Unternehmen probe** is **SKR** detection, which queries `/impersonalaccounts` for `4400` (→ `SKR04`) and `8400` (→ `SKR03`), falling back to a TTY prompt when the chart is custom.
- No `SCOPESKILL_*` environment override is exposed for **Unternehmen** attributes such as `SKR`, because they pair with `CUSTOMER` and `REST_REFRESH_TOKEN`; switch identity wholesale via `--config` (consistent with ADR-0004).
- A **Personenkonto** is either a **Debitor** or a **Kreditor** linked to a **Kontakt**.
- A **Debitor** and a **Kreditor** each link to exactly one **Kontakt**; a **Sachkonto** does not.
- A **Debitor** or **Kreditor** is assigned a **Sammelkonto** (**Sachkonto**) where aggregate subledger balances accumulate in the general ledger.
- A **Debitor** or **Kreditor** account number is assigned either explicitly or generated automatically from a configured **Nummernkreis**.
- A **Buchung** belongs to exactly one **Journal** (per Fiskaljahr) and references one or more Konten (Sachkonto, Debitor, or Kreditor).
- A **Buchung** touching a **Personenkonto** is exposed through three distinct Scopevisio views: impersonal rows in the **Journal** (booked against the **Sammelkonto**), subledger contra rows in the **Personenjournal**, and settlement tracking in **Offene Posten**.
- `sv-cli journal search` queries the **Journal** and returns only impersonal rows; it does not return **Personenkonto** rows when **Sammelkonten** are used. Subledger contra rows must be queried via `sv-cli personenkonto journal`, and open balances via `sv-cli offene-posten list`.
- A **Buchung** can have one attached **Beleg** file. `buchung file add` attaches a **Local file**; `buchung file get` retrieves it with optional invoice stamps.
- An **Offene Posten** entry references the **Beleg** that originated it and the **Kontakt** owning the **Debitor** or **Kreditor** side.
- The **`sv-cli`** stitches data on `show`-style commands when the second piece is reliably co-requested, but never on list-style commands (N+1 risk) and never derives business answers (see ADR-0006).
- Scopevisio's **SuSa** omits accounts that have no postings or carryover in the queried date range; **Saldo** queries synthesize a zeroed record for an existing account rather than treating omission as an error.
- A **Fiskaljahr** partitions the ledger into chronological **Buchungsperioden**; postings or settlements targeting closed periods or closed **Fiskaljahre** are rejected by the provider. `sv-cli buchhaltung fiscalyears` delivers the raw periods structure unchanged without deriving the active period.
- An **Eingangsrechnung** enters Scopevisio via PDF ingestion (`sv-cli eingangsrechnung import`) into the incoming invoice inbox (*Rechnungseingangsbuch*) without business metadata (`contentStateId=0`, `postingStateId=0`); metadata enrichment (`sv-cli eingangsrechnung update`) is performed as a separate, subsequent step.
- An existing **Buchung** is immutable regarding accounts, amounts, tax keys, and posting dates under GoBD; only `internalDocumentNumber` and `externalDocumentNumber` may be modified in place via `sv-cli buchung update`.
- `sv-cli kasse list` queries available **Kassen** through `/cashbooksheets`; `sv-cli kasse create` creates one **Kassenbuchung** through `/cashbookentry/new` and never sends the provider-managed internal Belegnummer.

## Example dialogue

> **Dev:** "Should the **Codex skill** handle multipart upload details in prose?"
> **Domain expert:** "No. The **Codex skill** should tell the agent to use the **`sv-cli`**, because the executable owns that fragile API formatting."
>
> **Dev:** "Should the **`sv-cli`** ask questions interactively when configuration is missing?"
> **Domain expert:** "No. It is a **Helper tool**. It should fail clearly so the calling agent or automation can decide what to do next."
>
> **Dev:** "Can we save the password after `auth` succeeds?"
> **Domain expert:** "No. Those are **Initial credentials**. Save or configure the resulting **REST refresh token**, never the login password."

## Flagged ambiguities

- "skill" was used to mean both the whole package and the executable helper. Resolved: use **scopeskill** for the package and **`sv-cli`** for the executable.
- "CLI for non-technical users" could imply an interactive app. Resolved: the **`sv-cli`** is a **Helper tool** for agents and automation first, with direct human use as a fallback.
- "credentials" was used for both password login and API access. Resolved: **Initial credentials** are transient login inputs; **REST refresh token** is the reusable durable API credential.
- "REST token" was initially treated as singular. Resolved: Scopevisio returns a short-lived **REST access token** and a long-lived **REST refresh token**; the helper stores the refresh token and uses access tokens internally.
- Environment override names were initially described with a `SCOPEVISIO_` prefix. Resolved: use `SCOPESKILL_*` because the variables belong to this helper, not the Scopevisio product.
- "doc" and "file" were used casually for Teamworkbridge content. Resolved: use **Teamwork document** for the remote CenterDevice object and **Local file** for bytes on disk.
- Teamwork upload/download command grouping. Resolved: hybrid. Generic `download <path>` stays top-level (it is just a binary GET); Teamwork-specific operations with bespoke flags (currently only multipart upload) live under `teamwork ...`.
- `journal search` was assumed to return all sides of a posting. Resolved: Scopevisio separates general impersonal rows (`/journal`) from personal subledger contra rows (`/personaljournal`) and settlement state (`/openitems/...`). Postings against a **Personenkonto** appear in the **Journal** only as aggregate rows on the **Sammelkonto**.
