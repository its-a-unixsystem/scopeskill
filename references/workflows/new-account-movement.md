# SOP: New Account Movement (Transaction-First Matching)

Standard Operating Procedure for processing a bank/payment transaction, searching across internal and external sources for the matching invoice/voucher (*Beleg*), booking the transaction, and reconciling open items.

---

## 1. Goal & Trigger

**Trigger:** The user asks to process a bank transaction or account movement (e.g. from bank feed, CSV, or user prompt: *"Process transaction of -119.00 EUR to Cloudflare on 2026-03-16"*).
**Goal:** Under GoBD (*"Keine Buchung ohne Beleg"*), discover the matching invoice across Scopevisio and external sources, ensure the invoice is booked, post the bank payment, and reconcile the open item.

---

## 2. Procedure

### Phase 0: Establish State

Before any matching or write preparation:

```bash
sv-cli auth show                 # authenticated Unternehmen and active SKR
sv-cli buchhaltung fiscalyears   # target Buchungsperiode must be open
```

* Record the read timestamp and the exact source identifiers used (transaction ID, statement ID, source account).
* A write targeting a closed Buchungsperiode is rejected by the provider; surface that before proposing postings.

---

### Phase 1: Preserve & Classify the Transaction

Preserve the complete source facts before extracting matching clues:

* **Source transaction ID** and **source account**: keep both; they identify the movement in every later step.
* **Currency** and **signed amount** (e.g. `-119.00 EUR` → target gross `119.00 EUR` on the creditor side).
* **Value date** (*Buchungsdatum / Valuta*, e.g. `2026-03-16`).
* **Counterparty**: payee / payer name (e.g. `Cloudflare`, `Telekom`).
* **Full remittance text** (*Verwendungszweck*): keep it complete; candidate invoice numbers (`INV-2026-981`, `RE-1049`), customer IDs, or contract references are extracted *from* it, not instead of it.
* Keep booking date, value date, invoice date, and service period as separate facts; never collapse them into one date.

Then classify the movement **before** matching it:

* Supplier payment, customer receipt, bank transfer between own accounts, card settlement, PayPal movement, refund, fee, tax payment, or another category.
* Resolve the **Geldkonto** from the configured source mapping for the source account — never infer it from the counterparty name.
* Only supplier payments and customer receipts enter the invoice-matching cascade below. Transfers, card settlements, PayPal movements, refunds, fees, and tax payments follow their own booking logic.
* Treat Qonto, Amex, and PayPal overlaps as **one economic movement** until the account-to-account chain proves otherwise.

**Already represented?** Check whether the movement already has a Buchung, a card settlement, or a duplicate source record before searching for an invoice:

```bash
sv-cli journal search --konto=<geldkonto> --from=<date-3d> --to=<date+3d> --all
# contra rows in the Personenjournal (no --konto flag; use the raw search body):
sv-cli personenkonto journal --data '{"pageSize":1000,"search":[{"field":"accountNumber","operator":"equals","value":"<personenkonto>"}]}'
```

If an equivalent payment Buchung already exists, stop and report; do not post a second one.

---

### Phase 2: Cascading Invoice Search

> [!TIP]
> **Subagent Delegation (Voucher Hunter):**
> Phase 2 is read-heavy and token-intensive (reading JSON search dumps, file trees, and PDFs). Delegate it to a read-only subagent in bounded batches keyed by stable transaction or candidate IDs.
>
> * **Subagent Prompt:** *"Find an invoice matching -<amount> <currency> to <counterparty> on/near <valueDate> (Ref: <extractedRef>). Check Scopevisio open items, ReBu inbox, Teamworkbridge, and the configured archives. Return ONLY: status (`unique`, `mehrdeutig`, `kein_treffer`), the candidate IDs, evidence per candidate, and open questions."*
> * Validate delegated output against the candidate space and global uniqueness before using it. The leading agent reads the decisive Beleg itself and owns all accounting judgment, approval presentation, writes, and post-write verification.
> * No delegated agent imports, repairs, posts, clears, attaches, cancels, or retries a mutation.

Search in the configured source order: Scopevisio first, then GetMyInvoices, then Google Drive or another configured archive. Every level returns **candidates**, not postings — downloads, imports, and Buchungen happen only after approval in Phase 4.

#### Level A: Scopevisio Open Items (Already Booked)

Check if an open item already exists waiting for payment:

```bash
# For outgoing payments (Kreditor):
sv-cli offene-posten list --seite=kreditor --all
# For incoming payments (Debitor):
sv-cli offene-posten list --seite=debitor --all
```

* **Match criteria:** the remittance reference matches the item's Beleg number **and** the open amount matches the signed movement amount in the same currency. Read the fields the response actually carries: Scopevisio may expose the item as `postingNumber` with a signed `amount` instead of `documentNumber`/`openAmount`.
* **Unique match required:** amount and date alone are candidates, not proof. Record unresolved cases as `mehrdeutig` (several candidates) or `kein_treffer` (none); never pick a plausible candidate silently.
* **Result:** one unique open item → the invoice is already booked. Proceed to **Phase 3** and skip invoice posting.

#### Level B: Scopevisio Invoice Inbox (ReBu Belege)

Check if the invoice PDF was already received/uploaded into the *Rechnungseingangsbuch (ReBu)* but not yet posted:

```bash
sv-cli eingangsrechnung search --content-state=0 --all
```

* Or search specifically by extracted invoice number or vendor name:
  ```bash
  sv-cli eingangsrechnung search --document-number="<extractedRef>"
  sv-cli eingangsrechnung search --vendor-name="<counterparty>"
  ```
* Inspect all three Beleg-Status machines (`contentStateId`, `paymentStateId`, `postingStateId`): `contentStateId=0` alone does not prove the invoice is unbooked.
* **Result:** candidate found → mark the Beleg for booking via the `new-invoice` workflow; it is proposed in Phase 3 and posted in Phase 4.

#### Level C: Scopevisio Teamwork / CenterDevice

Check the document tree in Teamworkbridge:

```bash
sv-cli get /teamworkbridge/documents --query text="<extractedRef or counterparty>"
```

* **Result:** candidate found → record the Teamwork document ID. Download and ReBu import are proposed in Phase 3 and executed in Phase 4 (import is a write).

#### Level D: Configured External Archives

If no match was found inside Scopevisio, check the configured external locations:

* Invoice routers / fetchers (GetMyInvoices, invoicefetcher).
* Google Drive or another configured cloud archive or receipt repository.
* Local incoming folders (e.g. `./inbox`, `~/Documents/Invoices/`, downloads).
* Connected email inboxes / mail search.
* **Result:** candidate found → record the Local file path or archive ID. ReBu import is proposed in Phase 3 and executed in Phase 4.

#### Match Evidence (all levels)

Before proposing a candidate, inspect the **original Beleg** — not only OCR, filename, workflow metadata, or a previous Buchung — and verify:

* Issuing legal entity and supplier Kontakt,
* Invoice number, invoice date, and service period,
* Currency, net, tax, and gross,
* Recipient (the authenticated Unternehmen),
* The full amount relationship against the movement (partial payments, refunds, fees).

Deduplicate copies by invoice number, amount, supplier, file content, and workflow state.

#### Level E: Fallback (Missing Beleg)

If no voucher is found across Levels A–D:

* **🛑 STOP & NOTIFY:** Alert the user that no voucher could be found:
  ```text
  No matching voucher found for transaction:
  - Source transaction: <transactionId> on <sourceAccount>
  - Movement: -119.00 EUR on 2026-03-16 (kein_treffer)
  - Payee: Cloudflare Inc.
  - Reference: INV-2026-981

  Options:
  1. Provide the PDF invoice file path to import.
  2. Book provisionally to transit/clearing account (Geldtransit / Klärende Posten).
  3. Hold transaction unposted until voucher is received.
  ```

---

### Phase 3: Interactive Confirmation & Approval

Stop and present the matched findings and proposed postings to the user:

```text
Matched Bank Transaction:
- Source transaction: <transactionId> on <sourceAccount> | Classification: <supplier payment | ...>
- Movement: <signedAmount> <currency> on <valueDate> | Payee: <counterparty>
- Geldkonto: <geldkonto> (from configured source mapping)

Matched Invoice:
- Status: unique | mehrdeutig | kein_treffer
- Source: [Scopevisio Open Items | Scopevisio ReBu Inbox | Teamwork | GetMyInvoices | Archive: <path>]
- Document: <docNumber> (<grossAmount> <currency>) from <vendorName> (Kreditor <kreditorKonto>)
- Evidence: <invoiceNumber>, <invoiceDate>, <servicePeriod>, net <net> / tax <tax> / gross <gross>
- Defects: <missing/incorrect invoice attributes, if any>

Proposed Actions:
1. [If from Teamwork/External]: Import Beleg into ReBu (eingangsrechnung import).
2. [If from Inbox/External]: Book invoice to <sachkonto> (Soll) vs <kreditorKonto> (Haben).
3. Post Bank Payment:
   - Soll:  <kreditorKonto> (<vendorName>) | <absAmount> EUR | Sammelkonto: <sammelkonto>
   - Haben: <geldkonto> (Bank)             | <signedAmount> EUR
4. Reconcile & Clear Open Item:
   - Allocate payment to invoice <docNumber> via offene-posten clear.

Do you approve?
```

---

### Phase 4: Execution & Settlement

Execute mutations in order; each is a separately approved step. Keep import, invoice posting, payment posting, clearing, and attachment as separate progress states.

1. **Import Beleg (if from Level C or D)**:
   ```bash
   # Teamwork source only: download first
   sv-cli download /teamworkbridge/document/<id> --out ./invoice.pdf

   sv-cli eingangsrechnung import --file=./invoice.pdf --dry-run
   sv-cli eingangsrechnung import --file=./invoice.pdf --yes
   ```

2. **Book Invoice (if from Level B, C, or D)**:
   * Follow the steps in [references/workflows/new-invoice.md](new-invoice.md) to post the invoice and attach the Beleg PDF.
   * This generates the active open item on the creditor account.

3. **Post Bank Payment**:
   Prepare `payment.json`, preserving the transaction's signed amount on the Geldkonto row:
   ```json
   {
     "documentNumber": "PAY-<date>-<ref>",
     "postingDate": "<valueDate>",
     "documentDate": "<valueDate>",
     "externalDocumentNumber": "<extractedRef>",
     "documentText": "Zahlung <counterparty> <extractedRef>",
     "rows": [
       {
         "account": "<kreditorKonto>",
         "summaryAccount": "<1600 or 3300>",
         "amount": <absAmount>,
         "rowText": "Ausgleich Verbindlichkeit"
       },
       {
         "account": "<geldkonto>",
         "amount": <signedAmount>,
         "rowText": "Bankabgang"
       }
     ]
   }
   ```
   Post with dry-run verification:
   ```bash
   sv-cli buchung create --data @payment.json --dry-run
   sv-cli buchung create --data @payment.json --yes
   ```
   `documentNumber` in the request is a request identifier only: Scopevisio may assign a different Belegnummer. Use the **returned** `documentNumber` from stdout for every later operation (clearing, attachment, verification); never reconstruct it from the request.

4. **Reconcile and Clear Open Item**:
   Record the expected before-balances for the payment and every selected item. Prepare `clearing.json`; for documents with earlier allocations, carry the approved before-state in `paymentOpenAmount` and every `items[].openAmount` (supply all fields together):
   ```json
   {
     "paymentDocumentNumber": "<returnedPaymentDocNr>",
     "paymentOpenAmount": <paymentBefore>,
     "items": [
       {
         "documentNumber": "<invoiceDocumentNumber>",
         "openAmount": <itemBefore>,
         "clearingAmount": <absAmount>
       }
     ]
   }
   ```
   Payment and items must share the same Personenkonto and currency. Clear the open item:
   ```bash
   sv-cli offene-posten clear --seite=kreditor --data @clearing.json --dry-run
   sv-cli offene-posten clear --seite=kreditor --data @clearing.json --yes
   ```
   Add `--allow-partial` only when the approved mapping intentionally leaves a remaining balance and the remaining amount is understood.

5. **Verify Settlement**:
   Confirm that the creditor balance and open item reflect the payment:
   ```bash
   sv-cli offene-posten list --seite=kreditor --konto=<kreditorKonto> --all
   sv-cli personenkonto journal --data '{"pageSize":1000,"search":[{"field":"accountNumber","operator":"equals","value":"<kreditorKonto>"}]}'
   ```
   If the clearing response is ambiguous or the observed balances differ from the preview, **stop without retrying** and report the before/after state.

---

### Phase 5: Failures, Retries, Resumption

* `conflict`, `verification_required`, and `verification_failed` are **stop states**: investigate before any retry.
* After an accepted or ambiguous write, search the Journal by both the requested and the returned identifiers before considering a retry. Never retry a write blindly after a timeout or transport error.
* Resume from the first unverified progress state; do not repeat completed mutations (import, invoice posting, payment posting, clearing, attachment).
* Record the exact payload, dry-run result, returned identifiers, verification result, and any remaining action.
