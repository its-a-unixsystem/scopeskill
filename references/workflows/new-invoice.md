# SOP: New Incoming Invoice (Eingangsrechnung)

Standard Operating Procedure for handling an incoming vendor invoice from inbox ingestion to fully booked and reconciled in Scopevisio.

---

## 1. Goal & Trigger

**Trigger:** The user asks to process an incoming invoice, work on an invoice in the inbox, or book an invoice.
**Goal:** Take the invoice from the inbox (*Rechnungseingangsbuch* / ReBu), enrich metadata, determine account assignment (*Kontierung*), seek user confirmation with historical/payment context, post to the Journal, attach the voucher, and reconcile against existing payments if paid.

---

## 2. Procedure

### Phase 0: Establish State

Before any inspection or write preparation:

```bash
sv-cli auth show                 # authenticated Unternehmen and active SKR
sv-cli buchhaltung fiscalyears   # target Buchungsperiode must be open
```

* Record the read timestamp and the exact source identifiers used (Beleg `id`/`number`, external invoice number).
* A write targeting a closed Buchungsperiode is rejected by the provider; surface that before proposing postings.

---

### Phase 1: Ingestion & Inspection

1. **Find Unprocessed Invoice in Inbox**:
   ```bash
   sv-cli eingangsrechnung search --content-state=0 --all
   ```
   Identify the Scopevisio internal `number` (e.g. `2026-1042`) and internal `id`.

2. **Check the Beleg-Status machines**:
   `contentStateId`, `paymentStateId`, and `postingStateId` are independent. `contentStateId=0` alone does not prove the invoice is unbooked — inspect all three before deciding it needs a Buchung.

3. **Read Extracted Data & Inspect the Original Document**:
   * Inspect Scopevisio's OCR extraction:
     ```bash
     sv-cli eingangsrechnung show <number>
     ```
   * Download the original PDF — decisive fields are verified against the Beleg itself, not only OCR, filename, or workflow metadata:
     ```bash
     sv-cli eingangsrechnung file <number> --out ./invoice.pdf
     ```
   * Verify: issuing legal entity, supplier Kontakt, external invoice number (`documentNumber`), invoice date (`documentDate`), service period (`deliveryDateFrom`/`deliveryDateTo`), due date (`dueDate`), currency, net, tax, gross total, and recipient.
   * **Determine the issuing company's tax treatment: inland, EU, or third country.** This drives the Steuerschlüssel and whether VAT rows appear at all.

4. **Master Data Resolution (Vendor / Kreditor)**:
   * Search for existing `Kontakt` and `Kreditor` before proposing new master data:
     ```bash
     sv-cli kontakt search --name="<Vendor Name>"
     sv-cli kreditor search --name="<Vendor Name>"
     ```
   * **🛑 GATE 1 (If Vendor is New):** Stop and ask the user to confirm creating the master data before continuing:
     * Show extracted Name, VAT ID, Address, and Country.
     * Once confirmed:
       ```bash
       sv-cli kontakt create --name="<Vendor>" --vat-id="<VAT_ID>" --dry-run
       sv-cli kontakt create --name="<Vendor>" --vat-id="<VAT_ID>" --yes
       # Then create Kreditor linked to the returned contactId:
       sv-cli kreditor create --contact-id=<contactId> --dry-run
       sv-cli kreditor create --contact-id=<contactId> --yes
       ```

5. **Determine whether the invoice is already booked**:
   * Search the Journal for the complete Buchung, the Personenjournal for the resolved Kreditor's contra rows (no `--konto` flag on `personenkonto journal`; use the raw search body), and the Kreditor Offene Posten:
     ```bash
     sv-cli journal search --belegnr="<Scopevisio ReBu number>" --all
     sv-cli journal search --text="<external invoice number>" --all
     sv-cli personenkonto journal --data '{"pageSize":1000,"search":[{"field":"accountNumber","operator":"equals","value":"<kreditorNumber>"}]}'
     sv-cli offene-posten list --seite=kreditor --konto=<kreditorNumber> --all
     ```
   * Run `sv-cli buchung show <documentNumber>` for each match and treat it as active only when `lifecycle.state` is `active`. Use an Offene-Posten `postingNumber` when it identifies that Buchung; otherwise resolve the `documentNumber` from the Journal or Personenjournal rather than inventing one. When cancelled, read `lifecycle.cancellationDocumentNumber`, `lifecycle.cancellation`, and optional `lifecycle.replacementDocumentNumber`; stop on `conflict`.
   * **If already booked:** do not recreate expense or VAT. If only the payment is missing, prepare only the payment and subsequent clearing ([new-account-movement.md](new-account-movement.md)).
   * **If the booking is incorrect:** stop and use the correction workflow (`buchung cancel` and `buchung create` as separately approved steps). Do not hide the error with a compensating payment.

6. **Enrich / Repair Beleg Metadata in ReBu**:
   If OCR missed attributes or vendor was newly resolved, update the inbox record before booking:
   ```bash
   sv-cli eingangsrechnung update <idOrNumber> \
     --vendor-contact-id=<contactId> \
     --document-number="<External Number>" \
     --document-date="YYYY-MM-DD" \
     --due-date="YYYY-MM-DD" \
     --delivery-date-from="YYYY-MM-DD" \
     --delivery-date-to="YYYY-MM-DD" \
     --text="<Belegtext>" \
     --dry-run
   sv-cli eingangsrechnung update <idOrNumber> ... --yes
   ```

---

### Phase 2: Historical Lookup & Payment Detection

1. **Lookup Past Bookings for the Vendor**:
   Check how this vendor was previously booked:
   ```bash
   sv-cli personenkonto journal --data '{"pageSize":1000,"search":[{"field":"accountNumber","operator":"equals","value":"<kreditorNumber>"}]}'
   ```
   * **If past postings exist:** Extract the expense account (*Sachkonto*) and tax key (*Steuerschlüssel*) used previously. Treat historical supplier patterns as **evidence, not the accounting rule** — verify the Sachkonto against the active chart (`references/skr03.csv` or `references/skr04.csv`) and the tax configuration, and check configured confusion pairs and cost-center restrictions.
   * **If no past postings exist:** Inspect the invoice service/product descriptions, consult the active chart of accounts, and form an intelligent suggestion (e.g. SaaS/hosting -> `4980`/`6855`, Office supplies -> `4930`/`6815`, Consulting -> `4955`/`6822`).

2. **Resolve one tax strategy explicitly**:
   * Either provide complete tax rows with `"autoCreateTax": false` in the Buchung payload,
   * or use automatic tax creation (`vatKey` on the expense row, `"autoCreateTax": true`) and inspect the generated tax rows in the created output after the write.
   * Never mix the two strategies implicitly.

3. **Search for Existing Payment (Reconciliation Check)**:
   Determine if this invoice has already been paid (e.g. via credit card, PayPal, direct debit / Lastschrift, or bank transfer):
   * **Check Kreditor Open Items for unallocated payments:**
     ```bash
     sv-cli offene-posten list --seite=kreditor --konto=<kreditorNumber> --all
     ```
   * **Check Bank Journal for matching gross amount around the invoice date:**
     ```bash
     sv-cli journal search --amount-min=<gross-0.01> --amount-max=<gross+0.01> --from=<date-30d> --all
     ```
   * A matched payment must sit on the same Personenkonto and currency as the invoice. Amount and date alone are candidates, not proof.
   * Note the matching `paymentDocumentNumber` if found.

---

### 🛑 GATE 2: Interactive Confirmation & Approval

Stop and present the complete proposal to the user:

```text
Invoice <documentNumber> from <Vendor Name> (Kreditor <konto>):
- Gross: <amount> <currency> (Net: <net> | Tax: <tax> [<taxKey>])
- Tax treatment: <inland | EU | third country> | Strategy: <autoCreateTax=false | automatic, rows inspected>
- Dates: Invoice <docDate> | Service period <deliveryDateFrom>-<deliveryDateTo> | Due <dueDate>
- Defects: <missing/incorrect invoice attributes, if any>

Account Assignment:
- Expense (Soll): <sachkonto> (<sachkontoName>) [<Source: matched N past bookings | intelligent suggestion>]
- Tax Key: <vatKey>
- Creditor (Haben): <kreditorKonto> (<kreditorName>) | Sammelkonto: <sammelkonto>
- Assumptions: <anything inferred rather than read off the Beleg>

Payment & Reconciliation:
- [PAID] Found bank transaction <paymentDocumentNumber> (<amount> <currency> on <date>). Will reconcile immediately after posting.
  -- OR --
- [UNPAID] No matching payment found. Will remain open on Kreditor account.

Do you approve this booking [and reconciliation]?
```

---

### Phase 3: Execution & Final Verification

Keep invoice posting, payment clearing, metadata repair, and attachment as separate progress states; resume from the first unverified state after any failure.

1. **Create the Journal Posting**:
   Prepare the booking payload (`booking.json`). The `documentNumber` uses the company's generated Belegnummer block (the Scopevisio ReBu number) — never invent the vendor's external invoice number as the internal Belegnummer; it belongs in `externalDocumentNumber`. Validate that all rows balance to zero unless `"autoCreateTax": true` lets Scopevisio create the tax rows, that amounts are cent-exact, and that dimensions and `summaryAccount` are correct:
   ```json
   {
     "documentNumber": "<Scopevisio ReBu Number>",
     "postingDate": "<docDate>",
     "documentDate": "<docDate>",
     "autoCreateTax": true,
     "internalDocumentNumber": "<Scopevisio ReBu Number>",
     "externalDocumentNumber": "<Vendor Invoice Number>",
     "documentText": "<Vendor Name> - <Text>",
     "rows": [
       {
         "account": "<sachkonto>",
         "amount": <netAmount>,
         "vatKey": "<vatKey>",
         "rowText": "<Item description>"
       },
       {
         "account": "<kreditorKonto>",
         "summaryAccount": "<1600 or 3300>",
         "amount": -<grossAmount>,
         "rowText": "Verbindlichkeit"
       }
     ]
   }
   ```
   Execute with dry-run verification first:
   ```bash
   sv-cli buchung create --data @booking.json --dry-run
   sv-cli buchung create --data @booking.json --yes
   ```
   Scopevisio may assign a different Belegnummer: use the **returned** `documentNumber` from stdout for every later operation; never reconstruct it from the request.

2. **Attach Beleg PDF to the Buchung (GoBD)**:
   Attach only after the created Buchung has been verified in the Journal (`buchung create` reads the final Buchung back before reporting `created`; when in doubt re-check with `journal search --belegnr`). Match the reviewed Local file to the exact returned Belegnummer (compare content hash or another stable identity):
   ```bash
   sv-cli buchung file add <assignedDocumentNumber> ./invoice.pdf --dry-run
   sv-cli buchung file add <assignedDocumentNumber> ./invoice.pdf --yes
   ```
   Read the attachment back when it is a required postcondition:
   ```bash
   sv-cli buchung file get <assignedDocumentNumber> --out ./verify.pdf
   ```

3. **Reconcile Payment (If Matched in Phase 2)**:
   Record the expected before-balances for the payment and the invoice. For documents with earlier allocations, carry the approved before-state in `paymentOpenAmount` and `items[].openAmount` (supply all fields together):
   ```json
   {
     "paymentDocumentNumber": "<paymentDocumentNumber>",
     "paymentOpenAmount": <paymentBefore>,
     "items": [
       {
         "documentNumber": "<assignedDocumentNumber>",
         "openAmount": <itemBefore>,
         "clearingAmount": <grossAmount>
       }
     ]
   }
   ```
   Execute clearing; add `--allow-partial` only when the approved mapping intentionally leaves a remaining balance:
   ```bash
   sv-cli offene-posten clear --seite=kreditor --data @clearing.json --dry-run
   sv-cli offene-posten clear --seite=kreditor --data @clearing.json --yes
   ```
   If the clearing response is ambiguous or the observed balances differ from the preview, **stop without retrying**.

4. **Verify Final State**:
   * General ledger entry confirmed:
     ```bash
     sv-cli journal search --belegnr="<assignedDocumentNumber>" --all
     ```
   * Open item state confirmed (cleared if paid, or open if unpaid):
     ```bash
     sv-cli offene-posten list --seite=kreditor --konto=<kreditorKonto> --all
     ```
   * Report completion to the user with document numbers and settlement status.

---

### Phase 4: Failures, Retries, Resumption

* `conflict`, `verification_required`, and `verification_failed` are **stop states**: investigate before any retry.
* After an accepted or ambiguous write, search the Journal by both the requested and the returned identifiers before considering a retry. Never retry a write blindly after a timeout or transport error.
* Resume from the first unverified progress state; do not repeat completed mutations (metadata repair, invoice posting, clearing, attachment).
* Record the exact payload, dry-run result, returned identifiers, verification result, and any remaining action.
