# Cleanup and reconciliation checklist

Use this checklist before preparing or executing accounting cleanup. Discovery
and preparation are read-only; every mutation is an explicit, separately
approved step. The checklist applies to the workflows in this directory and
to cleanup work in an external accounting repository.

## 1. Establish the current state

- [ ] Confirm the authenticated Unternehmen and active SKR with `sv-cli auth show`.
- [ ] Confirm the target period and inspect `sv-cli buchhaltung fiscalyears`.
- [ ] Confirm that the target Buchungsperiode is open before preparing a write.
- [ ] Use complete result sets (`--all`) for every completeness-sensitive query.
- [ ] Record the read timestamp and exact source identifiers used.
- [ ] Keep booking date, value date, invoice date, and service period separate.

## 2. Identify the transaction and its accounting role

- [ ] Preserve the source transaction ID, source account, currency, signed amount,
      value date, counterparty, and full remittance text.
- [ ] Classify the movement before matching it: supplier payment, customer
      receipt, bank transfer, card settlement, PayPal movement, refund, fee,
      tax payment, or another category.
- [ ] Confirm the Geldkonto from the configured source mapping. Do not infer it
      from the counterparty name.
- [ ] Check whether the movement is already represented by a Buchung, a card
      settlement, or a duplicate source record.
- [ ] Treat Qonto, Amex, and PayPal overlaps as one economic movement until the
      account-to-account chain proves otherwise.

## 3. Match the Beleg with hard evidence

- [ ] Search in the configured source order: Scopevisio, GetMyInvoices, then
      Google Drive or another configured archive.
- [ ] Inspect the original Beleg, not only OCR, filename, workflow metadata, or
      a previous Buchung.
- [ ] Verify issuing legal entity, supplier Kontakt, invoice number, invoice
      date, service period, currency, net, tax, gross, and recipient.
- [ ] Require a unique match. Amount and date alone are candidates, not proof.
- [ ] Check invoice number, supplier evidence, remittance text, currency, and
      the full amount relationship before accepting a match.
- [ ] Deduplicate copies by invoice number, amount, supplier, file content, and
      workflow state.
- [ ] Record unresolved cases as `mehrdeutig` or `kein_treffer`; never choose a
      plausible candidate silently.

## 4. Determine whether the invoice is already booked

- [ ] Inspect the Eingangsrechnung's independent `contentStateId`,
      `paymentStateId`, and `postingStateId`; do not treat `contentStateId=0`
      alone as unbooked.
- [ ] Search the Journal for the complete Buchung and the Personenjournal for
      the Kreditor or Debitor contra rows.
- [ ] Inspect the relevant Offene Posten on the correct side.
- [ ] Confirm cancellation or correction chains before treating a matching row
      as active.
- [ ] If the invoice is already booked, do not recreate expense or VAT.
- [ ] If only the payment is missing, prepare only the payment and subsequent
      clearing.
- [ ] If the booking is incorrect, stop and use the correction workflow; do not
      hide the error with a second compensating payment.

## 5. Prepare an invoice Buchung

- [ ] Read the Beleg and determine the issuing company's tax treatment: inland,
      EU, or third country.
- [ ] Resolve the Kontakt and Kreditor. Search before proposing new master data.
- [ ] Choose the Sachkonto from the active chart and tax configuration. Treat
      historical supplier patterns as evidence, not as the accounting rule.
- [ ] Check configured confusion pairs, cost-center restrictions, and the
      correct Geldkonto mapping.
- [ ] Resolve one tax strategy explicitly: either provide complete tax rows with
      `autoCreateTax=false`, or use automatic tax creation and inspect its rows.
- [ ] Validate that all rows balance to zero unless automatic tax creation
      supplies the tax rows, that amounts are cent-exact, and that dimensions
      and `summaryAccount` are correct.
- [ ] Use the company's generated Belegnummer block; do not invent an external
      invoice number as the internal Belegnummer.
- [ ] Run `sv-cli buchung create --dry-run` and retain its preview.
- [ ] Present the proposed rows, tax treatment, assumptions, evidence, and
      identified invoice defects for approval.

## 6. Prepare a payment Buchung

- [ ] Only prepare a payment after proving that the invoice booking is present
      or that the approved workflow includes the invoice Buchung first.
- [ ] Use the Kreditor or Debitor Personenkonto with its correct `summaryAccount`
      for an Offene-Posten relevant payment.
- [ ] Use the verified Geldkonto and preserve the transaction's signed amount.
- [ ] Confirm that no equivalent payment Buchung already exists.
- [ ] Run `sv-cli buchung create --dry-run` and inspect the complete payload.
- [ ] If the helper returns a provider-assigned Belegnummer, use that returned
      number for every later operation. Never reconstruct it from the request.

## 7. Clear Offene Posten safely

- [ ] Use the correct side (`--seite=kreditor` or `--seite=debitor`).
- [ ] Match the payment and selected Belege to the same Personenkonto and
      currency.
- [ ] Record expected before-balances for the payment and every selected item.
- [ ] Use `--allow-partial` only when the approved mapping intentionally leaves
      a remaining balance and the remaining amount is understood.
- [ ] Run `offene-posten clear --dry-run` and inspect its allocation preview.
- [ ] Do not use `openAmount` or `documentNumber` unless they are present in the
      current API response. Scopevisio may expose the OP as `postingNumber` with
      a signed `amount`.
- [ ] Verify payment and item balances after clearing. If the response is
      ambiguous or the balances changed, stop without retrying.

## 8. Attach and verify the Beleg

- [ ] Match the reviewed Local file to the exact Buchung and check its content
      hash or other stable identity.
- [ ] Preview `buchung file add <belegnummer> <datei> --dry-run`.
- [ ] Attach only after the corresponding Buchung has been verified.
- [ ] Read the attachment back with `buchung file get` when the attachment is a
      required postcondition.
- [ ] Treat a missing attachment as an unresolved finding, not automatically as
      a GoBD violation; check permissions and external DMS links first.

## 9. Handle failures and retries

- [ ] Treat `conflict`, `verification_required`, and `verification_failed` as
      stop states.
- [ ] After an accepted or ambiguous write, search by the requested and returned
      identifiers before considering any retry.
- [ ] Never retry a write blindly after a timeout or transport error.
- [ ] Keep invoice posting, payment posting, clearing, metadata repair, import,
      and attachment as separate progress states.
- [ ] Resume from the first unverified state; do not repeat completed mutations.
- [ ] Record the exact payload, dry-run result, returned identifiers, verification
      result, and any remaining action.

## 10. Reconcile a statement and report cleanup work

- [ ] Reconcile statement opening and closing balances, not only individual
      Offene Posten.
- [ ] Require one accounting representation for every statement movement and
      investigate duplicates, reversals, refunds, and transfers separately.
- [ ] Compare Journal, Personenjournal, Offene Posten, and source transaction
      records using the same cutoff, currency, signs, and opening balance.
- [ ] Age nonzero transit balances and document legitimate timing differences;
      do not classify every nonzero balance as an error.
- [ ] Report each result as `confirmed`, `probable`, `ambiguous`, or `missing`
      with the evidence and the next action.
- [ ] Keep correction, opening-balance, and tax-advisor-dependent work separate
      from independently preparable payment and invoice cleanup.

## 11. Agent delegation boundary

- [ ] Delegate read-only evidence gathering in bounded batches with stable
      transaction or candidate IDs.
- [ ] Require delegated results to contain only allowed candidate IDs, evidence,
      status, and open questions.
- [ ] Validate delegated output against the candidate space and global uniqueness
      before using it.
- [ ] The leading agent reads the decisive Beleg and owns all accounting
      judgment, approval presentation, writes, and post-write verification.
- [ ] No delegated agent imports, repairs, posts, clears, attaches, cancels, or
      retries a mutation.
