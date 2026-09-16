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
| Check a specific impersonal account     | `sachkonto show` / `balance`             | Investigating G/L (General Ledger) accounts                         |
| Check a customer/vendor account         | `debitor show` / `kreditor show`         | Investigating personal accounts linked to a Kontakt                 |
| Find open invoices/vouchers             | `offene-posten list --seite=...`         | Looking for unsettled items on either the debitor or kreditor side  |
| Clear reviewed creditor open items      | `offene-posten clear --seite=kreditor --data @f --dry-run`, then `--yes` | Only from an approved payment-to-Beleg mapping |
| Search chronological postings           | `journal search`                         | You need to see the ledger entries (Buchungen)                      |
| Create one reviewed Buchung             | `buchung create --data @f --dry-run`, then `--yes`  | Only from an approved Buchungssatz; never invent accounts/tax keys   |
| Cancel one reviewed Buchung             | `buchung cancel <nr> --dry-run`, then `--yes`       | Only after the user approved cancelling this exact documentNumber     |
| Attach a Beleg to a Buchung             | `buchung file add <nr> <file> --dry-run`, then `--yes` | Only after matching the reviewed file to the exact documentNumber |
| Retrieve a Buchung's Beleg              | `buchung file get <nr> [--with-stamp]`   | Reading the attached original or its stamped rendering             |
| View an incoming invoice                | `eingangsrechnung show`                  | Investigating vendor-side Belege (documents)                        |
| Fetch accounting metadata               | `buchhaltung info` / `dimension search`  | Need context on how the system is configured                        |
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

### Ledger Postings (Journal)
Search for specific postings by account or amount:
```bash
sv-cli journal search --konto=70019 --amount-min=100.00 --all
```

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