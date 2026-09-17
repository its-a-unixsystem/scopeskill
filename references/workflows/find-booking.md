# SOP: Find / Show Booking

Terse routine procedure for locating specific journal entries, inspecting their three-way views, and retrieving attached Belege.

---

## 1. Goal & Triggers
* **Trigger:** User asks to find, show, or inspect a booking, look up an invoice entry, or download a booking voucher.
* **Goal:** Return the complete posting details across general ledger and subledgers, plus attached documents if requested.

---

## 2. Lookup Strategy

Choose the fastest lookup path based on available query clues:

| Clue | Command | Notes |
| :--- | :--- | :--- |
| **Internal Document Number** | `sv-cli buchung show <documentNumber>` | Direct fetch of canonical posting |
| **External Invoice Number / Text** | `sv-cli journal search --text="<query>" --all` | Searches Belegtext and external references |
| **Amount & Date Window** | `sv-cli journal search --amount-min=X --amount-max=X --from=... --to=...` | Exact cent match on impersonal rows |
| **Personenkonto (Debitor/Kreditor)** | `sv-cli personenkonto journal --data '{"pageSize":1000,"search":[{"field":"accountNumber","operator":"equals","value":"<nr>"}]}'` | Required for debtor/creditor contra-rows (no `--konto` flag; raw search body) |
| **Settlement / Open Item State** | `sv-cli offene-posten list --seite=debitor\|kreditor --konto=<nr>` | Checks if item is open, partial, or cleared |

> [!NOTE]
> `sv-cli journal search` returns impersonal rows on Sachkonten (including aggregate Sammelkonten). To inspect individual Debitor or Kreditor contra-rows, always query `sv-cli personenkonto journal`.

> [!NOTE]
> Before treating a matching row as active, confirm its cancellation/correction chain: a Buchung may be superseded by a Storno plus Korrekturbuchung. For open items, read the fields the response actually carries — Scopevisio may expose the item as `postingNumber` with a signed `amount` instead of `documentNumber`/`openAmount`.

---

## 3. Retrieve Attached Voucher (Optional)
If the user asks to see or download the receipt:
```bash
# Original document
sv-cli buchung file get <documentNumber> --out ./voucher.pdf

# Or rendering with Scopevisio audit stamp
sv-cli buchung file get <documentNumber> --with-stamp --out ./voucher-stamped.pdf
```

A missing attachment is an unresolved finding, not automatically a GoBD violation: check permissions and external DMS links (e.g. `eingangsrechnung link`) before reporting it as absent.

---

## 4. Output Format
Present findings tersely:
* **Belegnummer**: `<documentNumber>` (External: `<externalNumber>`)
* **Date**: `<postingDate>`
* **Rows**:
  - Soll: `<account>` (`<name>`) | `<amount>` EUR | `<vatKey>`
  - Haben: `<account>` (`<name>`) | `<amount>` EUR
* **Settlement**: Cleared / Open (`<remainingAmount>` EUR remaining)
* **Status**: Active / Cancelled by Storno `<nr>` / Superseded by Korrekturbuchung `<nr>`
* **Attached File**: Yes (`<filename>`) / Missing (unresolved finding)
