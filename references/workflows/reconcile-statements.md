# SOP: Reconcile Bank Statements

Terse routine procedure for reconciling imported bank transactions against open items (*Offene Posten*) and clearing balanced entries.

---

## 1. Goal & Triggers
* **Trigger:** User asks to reconcile bank statements, match payments to open items, or clear customer/vendor open accounts.
* **Goal:** Give every statement movement exactly one accounting representation: allocate payments to open items on Debitor or Kreditor accounts, mark them cleared (*Ausgeglichen*), and prove statement opening and closing balances.

---

## 2. Procedure

### Step 0: Establish State
* Confirm the authenticated Unternehmen and active SKR:
  ```bash
  sv-cli auth show
  ```
* Confirm the target Buchungsperiode is open before preparing any clearing write:
  ```bash
  sv-cli buchhaltung fiscalyears
  ```
* Record the read timestamp and the exact source identifiers (statement ID, account, cutoff) used for the reconciliation.

### Step 1: Query Active Open Items
Fetch the complete unsettled item sets on the target side:
```bash
# Creditors (vendor invoices)
sv-cli offene-posten list --seite=kreditor --all

# Debtors (customer receivables)
sv-cli offene-posten list --seite=debitor --all
```
Read the fields the response actually carries: Scopevisio may expose an item as `postingNumber` with a signed `amount` instead of `documentNumber`/`openAmount`.

### Step 2: Match with Bank Payments
Match bank postings from the journal (`sv-cli journal search --konto=<Geldkonto> --all`) against open items by:
1. **Cent-exact amount**: payment amount equals the item's open amount under the same sign convention.
2. **Document / Invoice reference**: matching `externalNumber`, `postingNumber`, or `documentNumber` in the remittance text.
3. **Account & currency link**: payment and open item share the same Personenkonto (Debitor or Kreditor number) and currency.

Rules:
* Amount and date alone are candidates, not proof. Require a unique match; record unresolved cases as `mehrdeutig` (several candidates) or `kein_treffer` (none), and never pick a plausible candidate silently.
* Confirm cancellation or correction (Storno/Korrekturbuchung) chains before treating a matching row as active.
* Investigate duplicates, reversals, refunds, and transfers separately — each statement movement needs exactly one accounting representation.
* Compare Journal, Personenjournal, Offene Posten, and source transaction records using the same cutoff, currency, signs, and opening balance.

### Step 3: Clear Open Items
Record the expected before-balances for the payment and every selected item. Prepare `clearing.json`; for documents with earlier allocations, carry the approved before-state in `paymentOpenAmount` and every `items[].openAmount` (supply all fields together):
```json
{
  "paymentDocumentNumber": "<paymentDocNr>",
  "paymentOpenAmount": 200.00,
  "items": [
    {
      "documentNumber": "<invoiceDocNr>",
      "openAmount": 119.00,
      "clearingAmount": 119.00
    }
  ]
}
```

Run preflight dry-run and inspect the allocation preview:
```bash
sv-cli offene-posten clear --seite=kreditor --data @clearing.json --dry-run
```
* Add `--allow-partial` only when the approved mapping intentionally leaves a remaining balance and the remaining amount is understood.
* Once verified and approved, execute with `--yes`:
```bash
sv-cli offene-posten clear --seite=kreditor --data @clearing.json --yes
```
* Verify the payment and item balances after clearing. If the response is ambiguous (`verification_required`) or the balances changed unexpectedly, **stop without retrying**.

### Step 4: Verify Statement Balances
* Reconcile statement opening and closing balances, not only individual Offene Posten.
* Age any nonzero transit balances and document legitimate timing differences; a nonzero balance is a finding to investigate, not automatically an error.

### Step 5: Report
Report each result as `confirmed`, `probable`, `ambiguous`, or `missing`, with the evidence and the next action. Keep correction, opening-balance, and tax-advisor-dependent work separate from independently preparable clearing. Flag:
* **Orphaned payments:** Bank payments with no matching open item (alert user).
* **Under-/Overpayments:** Propose keeping the remaining balance open (`--allow-partial`) or booking the difference to Skonto / a fee account — as a separately approved decision.

---

## 3. Failures, Retries, Resumption

* `conflict`, `verification_required`, and `verification_failed` are **stop states**: investigate before any retry.
* After an accepted or ambiguous write, search by both the requested and the returned identifiers before considering a retry. Never retry a write blindly after a timeout or transport error.
* Keep clearing, metadata repair, and attachment as separate progress states; resume from the first unverified state.
* Record the exact payload, dry-run result, returned identifiers, verification result, and any remaining action.
