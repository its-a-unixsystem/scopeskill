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

## Accounting Commands

Accounting commands query the ledger (`Journal`), accounts (`Sachkonto`, `Debitor`, `Kreditor`), and open items (`Offene Posten`).

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

### `personenkonto`

Search Journal entries across personal accounts.

- `sv-cli personenkonto journal [--all]`
  Search personal Journal entries.

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

### `eingangsrechnung` / `gutschrift`

Search and inspect vendor-side Belege with stitched Kontakt data.

- `sv-cli eingangsrechnung search [filters] [--all]` / `sv-cli gutschrift search [filters] [--all]`
  Search for Eingangsrechnungen or Gutschriften.
  Filters: `--document-number`, `--vendor-name`, `--content-state`, `--payment-state`, `--posting-state`.
- `sv-cli eingangsrechnung show <Belegnummer>` / `sv-cli gutschrift show <Belegnummer>`
  Show the Beleg and its associated Kontakt when one can be resolved.

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
  Search the ledger for postings.
  Filters: `--from`, `--to`, `--konto`, `--text`, `--belegnr`, `--amount-min`, `--amount-max`, `--dim=KEY=VALUE`.
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
- `sv-cli buchung cancel <documentNumber> [--dry-run] [--yes]`
  Cancel one active Buchung via `POST /journal/<documentNumber>/cancel`. The
  command first reads the complete original, then searches the journal for
  documents linked through `cancellationNumber` matching the original's shared
  `pdeRowNumber`: a linked document whose rows are
  an exact sign reversal of the original counts as the Storno. If a verified
  Storno already exists the command reports `already_cancelled` and writes
  nothing, so it is safe to retry. Before writing it checks that the fiscal
  period is open and all original accounts exist and are active, previews the
  original rows and the active organisation on stderr, and — unless `--yes` is
  given — asks a TTY user to type `cancel <documentNumber>`. The write is sent
  exactly once and success is only reported after the Storno is read back and
  verified. Stdout statuses: `dry_run`, `cancelled`, `already_cancelled`,
  `conflict`, `verification_required`; everything except `cancelled`/
  `already_cancelled` exits non-zero.
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
