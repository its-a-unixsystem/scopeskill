# sv-cli Reference

`sv-cli` is the command-line helper for Scopevisio automation. It manages authentication, makes raw REST API calls, and provides high-level commands for common accounting and Teamworkbridge workflows.

## Global Options

- `--config <path>`: Override the scopeskill config file path. Defaults to the user's config directory.

## Core Commands

### `auth`

Manage the configured REST refresh token.

- `sv-cli auth login [--force] [--skr=skr03|skr04]`
  Interactive setup. Asks for credentials and saves the durable `REST_REFRESH_TOKEN` to the config. Probes and saves `SKR`.
- `sv-cli auth show`
  Show a redacted view of the configured `REST_REFRESH_TOKEN` and its source.
- `sv-cli auth secret`
  Show the full configured `REST_REFRESH_TOKEN`.
- `sv-cli auth delete`
  Remove the `REST_REFRESH_TOKEN` from the active config.

### Raw API Calls

- `sv-cli get <path> [--query KEY=VALUE]`
  Run an authenticated GET request.
- `sv-cli post <path> --data JSON`
  Run an authenticated POST request. You can pass `--data @file.json` to read from a file.
- `sv-cli download <path> --out <file>`
  Download bytes from an authenticated endpoint.

### `teamwork`

Teamworkbridge document operations.

- `sv-cli teamwork upload <file> [--metadata JSON] [--collection ID] [--tag TAG]`
  Upload a local file to Teamworkbridge, optionally filing it in a collection or tagging it.

## Master Directory (Kontakte)

### `kontakt`

Search and inspect the master directory.

- `sv-cli kontakt search [filters] [--all]`
  Search for Kontakte.
  Filters: `--name`, `--ust-id`, `--email`.
- `sv-cli kontakt show <id>`
  Show a single Kontakt by ID.
- `sv-cli kontakt create --name=STRING [flags] [--dry-run] [--yes]`
  Create a Kontakt via `POST /contact/new`. `--type=company|person` defaults to
  `company`. Optional fields are `--firstname`, `--salutation`, `--vat-id`,
  `--email`, `--street`, `--city`, `--postcode`, `--country`, and
  `--customer-number`. `--data=@file.json|JSON` supplies the complete
  `KontaktForm` and cannot be combined with individual field flags. The command
  previews the request, requires `--yes` or the phrase `create kontakt <name>`,
  and returns the new `contactId` with the created Kontakt. `--dry-run` does
  not write.

## Accounting Commands

Accounting commands query the ledger (`Journal`), accounts (`Sachkonto`, `Debitor`, `Kreditor`), and open items (`Offene Posten`).

`balance` queries SuSa first. If SuSa omits an inactive account, `sv-cli`
checks master data and returns a zeroed Saldo for an existing account; a
missing account returns `<type> <number> not found`. `show` keeps inactive
Saldo fields as `null` so callers can distinguish no activity from net zero.

### `sachkonto`

Search and inspect impersonal G/L accounts.

- `sv-cli sachkonto search [filters] [--all]`
  Search for Sachkonten.
  Filters: `--name`, `--number-prefix`, `--active`, `--type`.
- `sv-cli sachkonto show <Kontonummer>`
  Show a Sachkonto and its current balance.
- `sv-cli sachkonto balance <Kontonummer> [--from=YYYY-MM-DD] [--to=YYYY-MM-DD]`
  Show the balance of a Sachkonto for a specific period.

### `debitor` / `kreditor`

Search and inspect personal accounts linked to a Kontakt.

- `sv-cli debitor search [filters] [--all]` / `sv-cli kreditor search [filters] [--all]`
  Search for Debitoren or Kreditoren.
  Filters: `--name`, `--number`, `--number-prefix`, `--active`.
- `sv-cli debitor show <Kontonummer>` / `sv-cli kreditor show <Kontonummer>`
  Show the account details.
- `sv-cli debitor balance <Kontonummer> [--from=YYYY-MM-DD] [--to=YYYY-MM-DD]` / `sv-cli kreditor balance ...`
  Show the balance of the personal account.
- `sv-cli debitor journal [--all]` / `sv-cli kreditor journal [--all]`
  Search personal Journal entries for the account side.
- `sv-cli debitor bank-connections <Kontonummer>` / `sv-cli kreditor bank-connections <Kontonummer>`
  Show the bank connections for the personal account.
- `sv-cli debitor create --contact-id=N [flags] [--dry-run] [--yes]` /
  `sv-cli kreditor create --contact-id=N [flags] [--dry-run] [--yes]`
  Create a personal account for an existing Kontakt via `POST /createdebitor`
  or `POST /createkreditor`. Optional flags are `--number` for
  `personalAccountNumber`, `--number-range` for `numberRangeNumber`, and
  `--sum-account` for `sumAccountNumber`. If `--number` is omitted, Scopevisio
  assigns the first available number from `--number-range`, or from the first
  configured Nummernkreis when no range is given. `--data=@file.json|JSON`
  supplies the complete `PersonalAccountForm` and cannot be combined with the
  individual field flags; it must include `contactId` for the preflight check.
  The command first validates the Kontakt and returns `already_exists` without
  writing when the account is already linked. Otherwise it previews the
  request and requires `--yes` or `create <kind> <contact-id>`. Created and
  existing results stitch the personal account and Kontakt into stdout.

### `personenkonto`

Search entries in the Personenjournal across Personenkonten.

- `sv-cli personenkonto journal [--all]`
  Search entries in the Personenjournal. These are the contra rows on Debitor
  and Kreditor accounts. Use `journal search` for the corresponding
  impersonal rows on Sachkonten and Sammelkonten, and
  `offene-posten list --seite=debitor|kreditor` for settlement state and
  remaining amounts.

### `buchhaltung`

Inspect basic accounting configuration.

- `sv-cli buchhaltung info`
  Fetch accounting info.
- `sv-cli buchhaltung mapping`
  Fetch account mapping.
- `sv-cli buchhaltung gewinn-und-verlust [--balance-date=DD.MM.YYYY]`
  Fetch gain and loss adjustment accounts.

### `dimension` / `textbaustein`

Inspect dimension metadata and text templates.

- `sv-cli dimension search [--all]`
  Search dimensions.
- `sv-cli dimension entries <dimension> [--page=N] [--page-size=N]`
  Fetch entries for one dimension by name or number.
- `sv-cli textbaustein list`
  List Textbausteine.

### `statistik`

Search and inspect statistical accounts and postings.

- `sv-cli statistik konto search [--all]`
  Search Statistik-Konten.
- `sv-cli statistik konto show <number>`
  Show one Statistik-Konto.
- `sv-cli statistik buchung search [--all]`
  Search Statistik-Buchungen.
- `sv-cli statistik buchung show <rowNumber>`
  Show one Statistik-Buchung.

### Billing and Tax Configuration

Inspect payment terms and VAT configuration.

- `sv-cli zahlungsbedingung list`
  List Zahlungsbedingungen.
- `sv-cli zahlungsbedingung show <id>`
  Show one Zahlungsbedingung.
- `sv-cli steuermatrix list`
  List Steuermatrix entries.
- `sv-cli steuersachverhalt list`
  List Steuersachverhalte.

### `eingangsrechnung`

Search, inspect, and repair vendor-side Belege with stitched Kontakt data.

- `sv-cli eingangsrechnung search [filters] [--all]` / `sv-cli gutschrift search [filters] [--all]`
  Search for Eingangsrechnungen or Gutschriften.
  Filters: `--document-number`, `--vendor-name`, `--content-state`, `--payment-state`, `--posting-state`.
- `sv-cli eingangsrechnung show <Belegnummer>` / `sv-cli gutschrift show <Belegnummer>`
  Show the Beleg and its associated Kontakt when one can be resolved.
- `sv-cli eingangsrechnung file <idOrNumber> [--out <file>]`
  Download the main Local file for an Eingangsrechnung from `/incominginvoice/{idOrNumber}/file`. Without `--out`, use the response filename. The identifier accepts only the internal number or ID, not the external vendor invoice number.
- `sv-cli eingangsrechnung link <idOrNumber>`
  Print the Teamwork web link returned by `/incominginvoice/{idOrNumber}/teamworkFileLink`. The identifier accepts only the internal number or ID, not the external vendor invoice number.
- `sv-cli eingangsrechnung update <idOrNumber> [flags] [--dry-run] [--yes]`
  Repair an unverified Eingangsrechnung via `POST /incominginvoice/{id}`.
  Flags: `--vendor-contact-id=N`, `--document-number=VALUE`,
  `--document-date=YYYY-MM-DD`, `--due-date=YYYY-MM-DD`,
  `--delivery-date-from=YYYY-MM-DD`, `--delivery-date-to=YYYY-MM-DD`,
  `--text=VALUE` (Belegtext). Only the flags you pass are sent; all other
  Beleg fields are left untouched. Dates given as ISO calendar days are sent
  as epoch milliseconds. When every requested change is already present, the
  command reports `already_up_to_date` and writes nothing. The command
  previews the canonical payload on stderr, requires `--yes` or the
  interactive phrase `update <idOrNumber>` in a TTY, sends the write exactly
  once, and reads the Beleg back before reporting `updated`. Stdout statuses:
  `dry_run`, `updated`, `already_up_to_date`, `verification_required`;
  everything except `updated`/`already_up_to_date` exits non-zero.

### `offene-posten`

List, inspect, and clear unsettled Belege (Offene Posten).

- `sv-cli offene-posten list --seite=debitor|kreditor [filters] [--all]`
  List open items. You must specify the side (`--seite=debitor` or `--seite=kreditor`).
  Filters: `--overdue`, `--due-before=YYYY-MM-DD`, `--kontakt-id`, `--konto`.
- `sv-cli offene-posten show <id>`
  Show a single open item by ID.
- `sv-cli offene-posten clear --seite=kreditor --data @clearing.json [--dry-run] [--allow-partial] [--yes]`
  Safely allocate one reviewed creditor payment to one or more explicit
  creditor open items. The normal CLI-owned JSON shape is
  `{"paymentDocumentNumber":"PAY-1","items":[{"documentNumber":"INV-1","clearingAmount":119.00}]}`.
  For documents with earlier allocations, include the reviewed expected
  before-state as `paymentOpenAmount` and every `items[].openAmount`. Supply
  these fields together so an identical retry is distinguishable from a
  second allocation. Amounts are cent-exact. The command reads the
  payment journal document and every affected open item, rejects non-EUR or
  cross-Kreditor allocations,
  prevents over-clearing, and requires `--allow-partial` when an item will
  retain an open balance. It previews the canonical provider request and the
  predicted balances on stderr. Without `--yes`, type
  `clear <paymentDocumentNumber>` in a TTY.
  `--dry-run` performs every read-only check without writing. A real run
  rechecks the live balances, sends at most one
  `POST /openitems/creditor/clearing`, and reads every balance back. Stdout
  contains one JSON object with `dry_run`, `cleared`, `already_cleared`, or
  `verification_required` status. An ambiguous response is never retried.

### `journal` / `buchung`

Search chronological postings (Buchungen).

- `sv-cli journal search [filters] [--all]`
  Search the Journal's impersonal rows. A Buchung touching a Debitor or
  Kreditor appears here only through its aggregate Sammelkonto row; the contra
  row is in the Personenjournal, queried with `personenkonto journal`. Use
  `offene-posten list --seite=debitor|kreditor` for settlement state and the
  remaining amount of each item. `journal search` alone does not return the
  complete Buchung.
  Filters: `--from`, `--to`, `--konto` (Sachkonto only), `--text`, `--belegnr`,
  `--amount-min`, `--amount-max`, `--dim=KEY=VALUE`.
- `sv-cli buchung show <documentNumber>`
  Show a specific booking by its documentNumber.
- `sv-cli buchung create --data @buchung.json [--dry-run] [--yes]`
  Create one reviewed Buchung via `POST /postings/new`. The input carries shared
  document fields plus at least two rows:

  ```json
  {
    "documentNumber": "P-2025-1",
    "postingDate": "2025-06-02",
    "rows": [
      {"account": "4400", "amount": 119.00, "vatKey": "U19"},
      {"account": "1200", "amount": -119.00}
    ]
  }
  ```

  `summaryAccount` is required on rows that post to a Personenkonto. The command
  runs read-only preflight checks for the fiscal period, accounts, tax keys, and
  duplicate Buchungen. It searches the Journal for identical Buchungen when
  Scopevisio assigned a different document number. The command prints the exact
  payload to stderr. Without `--yes`, a TTY user must type `create
  <documentNumber>`. Without a TTY, the command fails before the write. The
  command sends the write exactly once and reads the final Buchung from the
  Journal before it reports success.

  Scopevisio can replace the requested `documentNumber` with an assigned number.
  In stdout, `documentNumber` contains the assigned number. The optional
  `requestedDocumentNumber` contains the input number when the numbers differ.
  Stdout statuses are `dry_run`, `created`, `already_exists`, `conflict`,
  `verification_required`, and `verification_failed`. All statuses except
  `created` and `already_exists` exit non-zero.
- `sv-cli buchung cancel <documentNumber> [--row=N] [--date=YYYY-MM-DD] [--dry-run] [--yes]`
  With no row, cancel the complete active Buchung via `POST
  /journal/<documentNumber>/cancel`. With `--row`, send `documentNumber` and the
  positive `pdeRowNumber` to `POST /posting/cancel`; optional `--date` supplies
  `cancellationDate` in the API's `dd.mm.yyyy` format and requires `--row`.
  Omitting `--date` requests cancellation on today's real date. The command
  first reads the complete original and performs the same fiscal-period,
  account, organisation, preview, confirmation, and dry-run safety checks.
  Whole-document cancellation also searches for documents linked through
  `cancellationNumber` and verifies an exact sign-reversed Storno, making that
  mode safe to retry. Confirmation remains `cancel <documentNumber>`.
  Stdout statuses: `dry_run`, `cancelled`, `already_cancelled`, `conflict`,
  `verification_required`; everything except `cancelled`/`already_cancelled`
  exits non-zero.
- `sv-cli buchung file add <documentNumber> <file> [--dry-run] [--yes]`
  Attach a local Beleg file via `POST
  /journal/<documentNumber>/file/new`. The command reads the file, sends its
  basename and base64-encoded bytes as `FileForm`, previews the exact payload,
  and requires `--yes` or the interactive phrase `attach <documentNumber>`.
  `--dry-run` performs no request. A real run sends the write exactly once.
- `sv-cli buchung file get <documentNumber> [--out <file>] [--with-stamp]`
  Download the attached Beleg via `GET /journal/<documentNumber>/file`.
  Without `--out`, the response filename is used. `--with-stamp` instead uses
  `GET /journal/<documentNumber>/filewithstamp`.
- `sv-cli buchung replace <documentNumber> --data @replacement.json [--dry-run] [--yes]`
  Intentionally unavailable: the atomic semantics of `POST
  /postings/correction` have not passed the mandatory controlled live contract
  test, so the command exits with a conflict without touching the API. The
  safe fallback is a `buchung cancel` and a `buchung create` issued as two
  separately approved and confirmed commands; they are never chained
  automatically.

## Common CLI Patterns

### Search Pagination

Search-style commands support the following pagination flags:

- `--page-size=N`: Override the single-page size (default: 100, max: 1000).
- `--all`: Page through all results automatically at `pageSize=1000`, capped at 10000.
- `--max=N`: Raise the `--all` safety cap (default: 10000).

### Raw Search Body Override (Escape Hatch)

If the provided CLI flags are insufficient, you can bypass them entirely and supply your own raw JSON search body for any `search` or `list` command:

```bash
./bin/sv-cli kontakt search --data @my-search.json
```

## Useful API Patterns

Most list endpoints are `POST` endpoints with a JSON search body. Common fields:

- `page`: starts at `0`
- `pageSize`: defaults to `100`, maximum `1000`
- `fields`: result fields to include
- `search`: array of `{"field", "value", "operator"}` filters
- `order`: array like `["lastname = asc"]`
- `count`: return only the matching count

Fetch the live OpenAPI document when in doubt:

```bash
curl -L https://appload.scopevisio.com/rest/swagger.json > /tmp/scopevisio-swagger.json
jq '.paths["/contacts"]' /tmp/scopevisio-swagger.json
```

## List and Download Teamwork Documents

Start from an already configured `sv-cli`:

```bash
./bin/sv-cli auth show
```

List collections, then copy the `id` from the collection you want:

```bash
./bin/sv-cli get /teamworkbridge/collections --query all=true
```

List documents in that collection:

```bash
./bin/sv-cli get /teamworkbridge/documents \
  --query all=true \
  --query collection=<collection-id>
```

Read one document's metadata:

```bash
./bin/sv-cli get /teamworkbridge/document/<document-id>
```

Download that document's bytes:

```bash
./bin/sv-cli download /teamworkbridge/document/<document-id> --out ./document.pdf
```

If you need folders inside a collection, list top-level folders first:

```bash
./bin/sv-cli get /teamworkbridge/folders \
  --query parent=none \
  --query collection=<collection-id>
```
