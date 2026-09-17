---
name: scopeskill
description: |
  Automate Scopevisio bookkeeping and Teamwork/CenterDevice document workflows via the sv-cli. Use this skill whenever the user wants to interact with the Scopevisio REST API, fetch chart of accounts (SKR), search master data (Kontakte), query journals, check open items, or upload/download Teamworkbridge documents. Do NOT ask the user for credentials if a valid token is already configured.
---

# scopeskill

Automate Scopevisio bookkeeping and Teamwork/CenterDevice document workflows. The `sv-cli` helper provides specialized subcommands for accounting entities and acts as a generic client for the Scopevisio REST API. 

Run `sv-cli --help` or `sv-cli <command> --help` for full option details.

## Prerequisites

Must be authenticated. Check the active token and chart of accounts (SKR) configuration:

```bash
sv-cli auth show
```

Output example:
```
REST_REFRESH_TOKEN: 8f4c... [source: config]
SKR: skr04 [source: config]
```

If the token is missing or invalid, perform the interactive setup. **Never ask the user for their password in chat.**

```bash
sv-cli auth login
```

## Workflow

Follow this escalation pattern when interacting with Scopevisio:

1. **Verify Context** - Check `auth show` to determine the active `SKR`. Interpreting Kontonummern requires knowing the SKR (reference `references/skr03.csv` or `references/skr04.csv`).
2. **Dedicated Commands** - Prefer specific `sv-cli` subcommands (e.g., `sachkonto search`, `offene-posten list`) over raw generic endpoints, as they handle complex pagination and filtering automatically.
3. **Generic API Calls** - If a dedicated command doesn't exist, use `sv-cli get` or `sv-cli post` with raw paths.
4. **Escape Hatch** - If CLI flags don't support a specific search filter, use the `--data` flag on search endpoints to inject a raw JSON query body.

| Need                                    | Command                                  | When                                                                |
| --------------------------------------- | ---------------------------------------- | ------------------------------------------------------------------- |
| Find a contact/customer/vendor          | `kontakt search`                         | You need the Kontakt ID to query linked personal accounts           |
| Create a missing Kontakt               | `kontakt create --name=... --dry-run`, then `--yes` | Adding reviewed master data before a personal account |
| Create a Debitor or Kreditor            | `debitor create` / `kreditor create --contact-id=N --dry-run`, then `--yes` | Linking a reviewed Kontakt to the required account side |
| Update a Debitor or Kreditor            | `debitor update` / `kreditor update <nr> --file=changes.json --dry-run`, then `--yes` | Correcting reviewed Personenkonto master data |
| Check a specific impersonal account     | `sachkonto show` / `balance`             | Investigating G/L (General Ledger) accounts                         |
| Fetch a BWA, Bilanz, or GuV             | `bericht show` / `bericht export`         | Need the provider's structured report JSON or an unchanged CSV/PDF export |
| Check a customer/vendor account         | `debitor show` / `kreditor show`         | Investigating personal accounts linked to a Kontakt                 |
| Find open invoices/vouchers             | `offene-posten list --seite=...`         | Looking for unsettled items on either the debitor or kreditor side  |
| Clear reviewed creditor open items      | `offene-posten clear --seite=kreditor --data @f --dry-run`, then `--yes` | Only from an approved payment-to-Beleg mapping |
| Search chronological postings           | `journal search`                         | You need to see the ledger entries (Buchungen)                      |
| Search Personenjournal postings         | `personenkonto journal`                  | You need to see postings for a Debitor or Kreditor in the Personenjournal |
| Create one reviewed Buchung             | `buchung create --data @f --dry-run`, then `--yes`  | Only from an approved Buchungssatz; never invent accounts/tax keys   |
| Cancel one reviewed Buchung             | `buchung cancel <nr> --dry-run`, then `--yes`       | Only after the user approved cancelling this exact documentNumber     |
| Update posting document numbers         | `buchung update <nr> [flags] --dry-run`, then `--yes` | Updating internal or external document numbers on an existing Buchung |
| Attach a Beleg to a Buchung             | `buchung file add <nr> <file> --dry-run`, then `--yes` | Only after matching the reviewed file to the exact documentNumber |
| Retrieve a Buchung's Beleg              | `buchung file get <nr> [--with-stamp]`   | Reading the attached original or its stamped rendering             |
| View an incoming invoice                | `eingangsrechnung show`                  | Investigating vendor-side Belege (documents)                        |
| Ingest an incoming invoice PDF          | `eingangsrechnung import --file=f.pdf --dry-run`, then `--yes` | Uploading a vendor invoice PDF to Scopevisio |
| Repair an incoming invoice's vendor or dates | `eingangsrechnung update <id> --dry-run`, then `--yes` | Fixing a missing `vendorContactId` or implausible `documentDate` on an unverified Beleg |
| Fetch accounting metadata               | `buchhaltung info` / `fiscalyears` / `dimension search`  | Need context on how the system is configured or open periods |
| Browse Teamworkbridge collections       | `get /teamworkbridge/collections`        | Navigating the remote CenterDevice document tree                    |
| Upload a local file to Teamwork         | `teamwork upload <file>`                 | Pushing a file, optionally to a specific `--collection`             |
| Download a Teamwork document            | `download /teamworkbridge/document/<id>` | Pulling a file from CenterDevice to the local disk                  |

## Working with Accounting Data

### Master Data (Kontakte) & Personal Accounts
Before querying personal accounts (Debitor/Kreditor), you often need the `Kontakt`:
```bash
sv-cli kontakt search --name="LinkedIn"
sv-cli kreditor search --name="LinkedIn"
```

When an Eingangsrechnung names a supplier with no matching Kreditor, keep the
two writes separate. First search to avoid duplicate master data. If no Kontakt
exists, preview `sv-cli kontakt create --name="..." --vat-id="..." --dry-run`,
show the payload for approval, then repeat with `--yes`. Take the returned
`contactId`, preview `sv-cli kreditor create --contact-id=N --dry-run`, and
repeat with `--yes` only after approval. Omit `--number` to let Scopevisio
assign the first available account number from its configured Nummernkreis;
use `--number-range=N` when the Unternehmen has multiple Kreditor
Nummernkreise. Never combine these primitives into an implicit create-both
operation.

Updating a Debitor or Kreditor is a separate write operation. Put only the
reviewed modifications in one JSON object, preview them with `debitor update
<nr> --file=changes.json --dry-run` or the matching `kreditor` command, and
repeat with `--yes` only after approval. The command accepts no individual
property flags, sends the write once, and reads the updated Personenkonto back
before reporting success.

### Balances and Open Items
Check balances and unsettled items. You must specify `--seite=debitor` or `--seite=kreditor` for open items:
```bash
sv-cli kreditor balance 70019
sv-cli offene-posten list --seite=kreditor --konto=70019 --all
```

Clearing Offene Posten is a separate write from posting a payment. Build a
CLI-owned payload with one `paymentDocumentNumber` and explicit
`items[].documentNumber` / `items[].clearingAmount` values. For documents with
earlier allocations, carry the approved expected before-state in
`paymentOpenAmount` and every `items[].openAmount`; supply all fields together.
Run `offene-posten clear --seite=kreditor --data @clearing.json --dry-run`,
show the preflight preview to the user, and only re-run with `--yes` after
explicit approval. Add `--allow-partial` only when the approved mapping
intentionally leaves an item balance. The command verifies the live Kreditor,
currency, and balances, sends at most one request, and reads every affected
balance back.

### Financial Reports
Fetch the provider's structured BWA, Bilanz, or GuV without deriving business
answers:
```bash
sv-cli bericht show --type=bwa --name="Standard" --year=2026 --month=9
sv-cli bericht export --type=bilanz --from=01.01.2026 --to=31.12.2026 --format=pdf --out=bilanz-2026.pdf
```

`bericht show` writes the `/proreport` JSON unchanged. `--layout` selects a
column layout and defaults to `Standard`; `--show-accounts` includes account
rows. `bericht export`
writes the `/reports/{type}` bytes unchanged; without `--out`, it uses the
response filename.


### Ledger Postings (Journal)
Search for specific postings by Sachkonto or amount:
```bash
sv-cli journal search --konto=4400 --amount-min=100.00 --all
```

> **Important:** A Buchung touching a Personenkonto has three distinct views.
> `sv-cli journal search` returns only the impersonal rows on Sachkonten,
> including aggregate Sammelkonto rows. Use `sv-cli personenkonto journal` for
> the contra rows in the Personenjournal on Debitor or Kreditor accounts, and
> `sv-cli offene-posten list --seite=debitor|kreditor` for settlement state and
> each item's remaining amount. `journal search` alone is not the complete
> Buchung.

Cancelling a Buchung is a write operation: always run `buchung cancel <nr>
--dry-run` first, show the preview to the user, and only re-run with `--yes`
after explicit approval. `buchung replace` is gated and refuses to write: the
atomic semantics of `POST /postings/correction` have not passed the required
controlled live contract test. Never call `/postings/correction` (or any raw
endpoint) to cancel or correct a Buchung, and never improvise a replacement by
chaining `buchung cancel` and `buchung create` automatically — issue each
step separately and only after its own confirmation.

Attaching a Beleg is also a write operation. Run `buchung file add <nr> <file>
--dry-run`, inspect the filename and base64 payload preview, and re-run with
`--yes` only after confirming the exact Buchung and file. Retrieve the original
with `buchung file get <nr>` or add `--with-stamp` for the invoice-stamped
rendering. Use `--out` only when the provider response filename is unsuitable.

### Belege (Invoices & Credits)
Search for specific documents or filter by workflow state. Note that workflow states are integers (e.g., `0` = Unbearbeitet):
```bash
# Find all unprocessed (unbearbeitete) incoming invoices
sv-cli eingangsrechnung search --content-state=0 --all
```

**Important Note on Belegnummern:** The `show` command (`sv-cli eingangsrechnung show <number>`) expects the internal Scopevisio `number` (e.g., `2025-42`) or the internal `id`. If you only have the vendor's external invoice number (`documentNumber`), `show` will fail with "not found". In that case, search by `documentNumber` first to find the `id`, then fetch the details:
```bash
# 1. Find the internal ID using the external document number
sv-cli eingangsrechnung search --document-number="INV-1234"

# 2. Fetch the full details using the internal ID
sv-cli get /incominginvoice/<id>
```

Repairing an Eingangsrechnung is a write operation: run `eingangsrechnung
update <idOrNumber> --vendor-contact-id=N --document-date=YYYY-MM-DD ...`
first with `--dry-run`, show the preview to the user, and re-run with `--yes`
only after approval. Only the flags you pass are sent; ISO dates are converted
to epoch milliseconds automatically. The command refuses to write when the
Beleg already carries every requested value (`already_up_to_date`), so a
retried repair is safe. Updates are only possible before the Beleg is
verified.

## Teamworkbridge Integration

Teamworkbridge resources map directly to Scopevisio's `/teamworkbridge/` endpoints. 

**List and Download:**
```bash
# 1. Find the collection ID
sv-cli get /teamworkbridge/collections --query all=true

# 2. Find documents in that collection
sv-cli get /teamworkbridge/documents --query all=true --query collection=<id>

# 3. Download the specific document
sv-cli download /teamworkbridge/document/<doc-id> --out ./downloaded.pdf
```

**Upload:**
```bash
sv-cli teamwork upload ./invoice.pdf --collection <collection-id> --tag "Finance"
```

## Pagination & Advanced Queries

Commands like `search` and `list` support pagination out of the box:
- `--all`: Automatically fetch up to 10,000 results.
- `--page-size=N`: Adjust the chunk size.

If built-in flags are insufficient, use the raw JSON search escape hatch:
```bash
# Pass raw Scopevisio JSON search body
sv-cli kontakt search --data @complex-search.json
```

## Domain Language Rules
Always adhere to the terminology in `docs/agents/domain.md`. For example:
- Use **Unternehmen** (not Mandant).
- Use **Kontakt** (not Customer/Supplier, unless referring specifically to the linked account side).
- Use **Beleg** (not Voucher).