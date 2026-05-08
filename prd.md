## Problem Statement

Agents currently lack the ability to inspect and search for `Beleg` documents, specifically `Eingangsrechnung` (incoming invoices) and `Gutschrift` (credit notes) via the `sv-cli` helper tool. Without this capability, automation workflows involving vendor invoices and credits cannot interact directly with the Scopevisio API to verify workflow states, view metadata, or identify the associated `Kontakt` (supplier).

## Solution

Introduce two new CLI commands in `sv-cli`: `eingangsrechnung` and `gutschrift`. These commands will follow the established `search` and `show` pattern. 
- The `search` subcommand will allow filtering by document number, vendor name, and workflow states, supporting standard pagination. 
- The `show` subcommand will accept a `Belegnummer` and stitch the associated `Kontakt` directly into the response payload so that a single lookup provides the complete context.

## User Stories

1. As an agent/automation script, I want to search for an `Eingangsrechnung` using its `Belegnummer` so that I can find a specific invoice in the system.
2. As an agent/automation script, I want to search for an `Eingangsrechnung` or `Gutschrift` by vendor name so that I can list documents belonging to a specific supplier.
3. As an agent/automation script, I want to filter documents by their `contentStateId`, `paymentStateId`, or `postingStateId` so that I can identify invoices in a specific stage of the workflow (e.g., unpaid, posted).
4. As an agent/automation script, I want to paginate through search results so that I can handle scenarios where a vendor has many invoices.
5. As an agent/automation script, I want to use the escape hatch `--data @file.json` when searching so that I can perform complex queries not covered by the default CLI flags.
6. As an agent/automation script, I want to view the details of an `Eingangsrechnung` using its `Belegnummer` so that I can inspect the full Scopevisio API payload for the document.
7. As an agent/automation script, I want the `show` command to automatically include the `Kontakt` associated with the document so that I do not have to perform a second API call to determine who the vendor is.
8. As an agent/automation script, I want to perform the same search and show operations for a `Gutschrift` so that credit notes can be processed identically to invoices.

## Implementation Decisions

- A new file `cmd/sv-cli/beleg.go` will be created to encapsulate the logic for document-style endpoints.
- A shared configuration struct (`belegKind`) will be used to DRY up the `search` and `show` logic for `eingangsrechnung` and `gutschrift`, similar to the `personalAccountKind` pattern.
- The `search` commands will expose the following flags: `--document-number`, `--vendor-name`, `--content-state`, `--payment-state`, and `--posting-state` (plus standard pagination flags).
- The `show` commands will take a `<Belegnummer>` as a positional argument.
- The `show` commands will implement data stitching (ADR-0006) to embed the `Kontakt` data. The link resolution will first check the `vendorContactId` field on the `Beleg`; if it is null, it will fall back to querying `/kreditoraccounts` using the `vendorPersonalAccountId` to fetch the `contactId`.
- The domain terms `Eingangsrechnung` and `Gutschrift` have been added to the project `CONTEXT.md` to adhere strictly to the German language model.
- The root CLI router in `cmd/sv-cli/main.go` will be updated to include `eingangsrechnung` and `gutschrift`.

## Testing Decisions

- A good test verifies the external behavior (CLI flags and output) and the network interaction without coupling to implementation details.
- Tests will be added in `cmd/sv-cli/beleg_test.go` (or similar) mocking the Scopevisio API endpoints (`/rest/incominginvoices`, `/rest/credits`, `/rest/incominginvoice/{number}`, `/rest/kreditoraccounts`, `/rest/contact/{id}`).
- The tests should verify that:
  - Search flags correctly construct the `SearchRequest` conditions payload.
  - The `show` command correctly stitches the `Kontakt` when `vendorContactId` is present.
  - The `show` command correctly performs the fallback lookup via `vendorPersonalAccountId` when `vendorContactId` is null.
- Prior art: `cmd/sv-cli/offene_posten_test.go` and `cmd/sv-cli/personal_account_test.go` serve as the templates for HTTP mock servers and CLI execution testing.

## Out of Scope

- Creating or updating an `Eingangsrechnung` or `Gutschrift` via the CLI (v1 is read-only for these entities).
- Operations related to `Ausgangsrechnung` (outgoing invoices) or other non-vendor document types.
- Modifying postings or workflow states (the states are read-only filters).

## Further Notes

- The payload for `/credits` is assumed to be structurally identical to `/incominginvoices` with respect to the `vendorContactId` and `vendorPersonalAccountId` fields based on domain knowledge.