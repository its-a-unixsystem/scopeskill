# SOP: Check Bookings (Auditing & Consistency Checks)

Standard Operating Procedure for auditing journal entries, verifying GoBD compliance (*"Keine Buchung ohne Beleg"*), cross-checking personal account balances against open items, and flagging accounting anomalies.

---

## 1. Goal & Trigger

**Trigger:** The user asks to audit, check, or verify bookings (e.g. *"Audit Q1 bookings"*, *"Check if any bookings are missing receipts"*, *"Reconcile creditor accounts"*).
**Goal:** Produce a comprehensive audit report detailing missing document attachments, unlinked or unbalanced personal account entries, unresolved transit balances, and anomalous tax keys without modifying any ledger records.

---

## 2. Subagent Architecture (Linter Pattern)

> [!TIP]
> **Subagent Delegation (Parallel Auditors):**
> Auditing large batches of journal entries and checking file attachments burns significant context window tokens. This workflow is ideal for **lightweight, read-only subagents** (e.g. `flash` or `flash_lite`):
>
> * **Audit Dispatch:** The main agent partitions the audit scope into bounded batches keyed by stable transaction or candidate IDs (e.g. by month, quarter, or check type) and spawns parallel subagents.
> * **Subagent Scope:** Read-only queries (`sv-cli journal search`, `sv-cli buchung file get`, `sv-cli offene-posten list`). A delegated result contains only the allowed candidate IDs, evidence, status, and open questions.
> * **Validation:** The main agent validates delegated output against the candidate space and global uniqueness before using it.
> * **No Mutation:** No delegated agent imports, repairs, posts, clears, attaches, cancels, or retries a mutation. The leading agent owns all accounting judgment and post-write verification; all write actions require a separate, user-approved corrective procedure.

---

## 3. Step-by-Step Audit Checks

Record the read timestamp and the exact source identifiers (period, account numbers, query filters) used for every check. Use complete result sets (`--all`) for every completeness-sensitive query.

### Check 1: GoBD Document Verification (*Keine Buchung ohne Beleg*)
Verify that every posting in the target period has a verifiable voucher attached:
1. **Retrieve Journal Entries**:
   ```bash
   sv-cli journal search --from=YYYY-MM-DD --to=YYYY-MM-DD --all
   ```
2. **Check Voucher Attachment**:
   For each distinct `documentNumber`:
   ```bash
   sv-cli buchung file get <documentNumber>
   ```
3. **Flag Discrepancies**:
   * Any entry returning `404` or "no file found" is flagged as a **Missing Beleg finding** — an unresolved finding, not automatically a GoBD violation. Check permissions and external DMS links (e.g. `eingangsrechnung link`) before classifying it.

---

### Check 2: Three-Way Personal Account Consistency
A Buchung touching a Personenkonto must be consistent across all three views:
1. **Sachkonto View (Journal)**:
   ```bash
   sv-cli journal search --konto=<Sammelkonto> --from=... --to=... --all
   ```
   *(Sammelkonto is typically 1600/3300 for Kreditor, 1400/1200 for Debitor).*
2. **Personenjournal Contra-Row** (no `--konto` flag; use the raw search body):
   ```bash
   sv-cli personenkonto journal --data '{"pageSize":1000,"search":[{"field":"accountNumber","operator":"equals","value":"<personalAccountNumber>"}]}'
   ```
3. **Settlement State (Offene Posten)**:
   ```bash
   sv-cli offene-posten list --seite=debitor|kreditor --konto=<personalAccountNumber> --all
   ```
   *Compare all three views using the same cutoff, currency, signs, and opening balance.* Read the fields the response actually carries: Scopevisio may expose an open item as `postingNumber` with a signed `amount` instead of `documentNumber`/`openAmount`.
* **Flag Discrepancies:** Personal account balance $\neq$ sum of open items. Confirm cancellation or correction (Storno/Korrekturbuchung) chains before treating a matching row as active.

---

### Check 3: Transit & Clearing Accounts (*Geldtransit / Klärende Posten*)
Transit accounts are temporary holding accounts expected to clear to zero:
1. **Check Transit Accounts**:
   * SKR03: `1360` (Geldtransit), `1590` (Durchlaufende Posten)
   * SKR04: `1460` (Geldtransit), `1370` (Durchlaufende Posten)
   ```bash
   sv-cli sachkonto balance <transitKonto>
   ```
2. **Flag Discrepancies**:
   * Age every non-zero balance and document legitimate timing differences; a non-zero balance is a finding to investigate, not automatically an error. Uncompleted bank transfers or missing counter-postings are the typical root cause.

---

### Check 4: Fiscal Period Guardrail
Ensure no entries target closed or unapproved periods:
```bash
sv-cli buchhaltung fiscalyears
```
* Compare the `postingDate` of all entries against the list of `open: false` fiscal years.

---

## 4. Output: Audit Report Structure

The agent consolidates all findings into a structured report. Classify each result as `confirmed`, `probable`, `ambiguous`, or `missing`, with the evidence and the next action. Keep correction, opening-balance, and tax-advisor-dependent work separate from independently preparable payment and invoice cleanup.

```markdown
# Accounting Audit Report: [Target Period]

_Read at <timestamp>; sources: <queries/identifiers used>_

### Summary
- Total Postings Examined: [Count]
- Missing Belege (Unresolved Findings): [Count]
- Unreconciled Open Items (>30 Days): [Count]
- Transit Accounts Status: [Balanced (0.00 EUR) | Aged non-zero balance]

### 🔴 Critical Violations (Action Required)
| Document Number | Date | Account | Amount | Status | Evidence | Next Action |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| 2026-104 | 2026-02-10 | 4980 | 350.00 EUR | missing | file get → 404, no DMS link | obtain Beleg, attach |

### 🟡 Warnings & Discrepancies
| Account | Name | Current Balance | Open Items Balance | Status | Discrepancy |
| :--- | :--- | :--- | :--- | :--- | :--- |
| 70015 | Cloudflare Inc. | -119.00 EUR | 0.00 EUR | ambiguous | Balance exists but no open item |

### 🟢 Verified Safe
All [N] standard postings balanced with attached vouchers.
```
