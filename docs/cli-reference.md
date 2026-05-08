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

List and inspect unsettled invoices/vouchers (Offene Posten).

- `sv-cli offene-posten list --seite=debitor|kreditor [filters] [--all]`
  List open items. You must specify the side (`--seite=debitor` or `--seite=kreditor`).
  Filters: `--overdue`, `--due-before=YYYY-MM-DD`, `--kontakt-id`, `--konto`.
- `sv-cli offene-posten show <id>`
  Show a single open item by ID.

### `journal` / `buchung`

Search chronological postings (Buchungen).

- `sv-cli journal search [filters] [--all]`
  Search the ledger for postings.
  Filters: `--from`, `--to`, `--konto`, `--text`, `--belegnr`, `--amount-min`, `--amount-max`, `--dim=KEY=VALUE`.
- `sv-cli buchung show <documentNumber>`
  Show a specific booking by its documentNumber.

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
