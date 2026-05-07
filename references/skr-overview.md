# SKR03/SKR04 Reference

Source basis: DATEV product pages for `Kontenrahmen DATEV SKR 03` (Art.-Nr. 11174) and `Kontenrahmen DATEV SKR 04` (Art.-Nr. 11175), plus the public 2025 DATEV Kontenrahmen PDFs mirrored by tax advisory sites. DATEV says the Kontenrahmen show the standard account labels and function assignments, and the end of each Kontenrahmen contains the meanings of tax and correction keys.

## Klassengliederung

SKR03 follows process structure:

- 0: Anlage- und Kapitalkonten
- 1: Finanz- und Privatkonten
- 2: Abgrenzungskonten
- 3: Wareneingangs- und Bestandskonten
- 4: Betriebliche Aufwendungen
- 7: Bestände an Erzeugnissen
- 8: Erlöskonten
- 9: Vortrags- und statistische Konten

SKR04 follows financial statement structure:

- 0: Anlagevermögen
- 1: Umlaufvermögen
- 2: Eigenkapitalkonten
- 3: Fremdkapitalkonten
- 4: Betriebliche Erträge
- 5 and 6: Betriebliche Aufwendungen
- 7: Weitere Erträge und Aufwendungen
- 9: Vortrags- und statistische Konten

## Steuerschlüssel und Konten

For ordinary domestic VAT cases, start from the account's tax-bearing label before choosing or interpreting a booking key:

- SKR03 revenue: `8400` is `Erlöse 19 % USt`, `8300` is `Erlöse 7 % USt`.
- SKR04 revenue: `4400` is `Erlöse 19 % USt`, `4300` is `Erlöse 7 % USt`.
- SKR03 input tax: `1576` is `Abziehbare Vorsteuer 19 %`; SKR04 equivalent: `1406`.
- SKR03 output tax: `1776` is `Umsatzsteuer 19 %`; SKR04 equivalent: `3806`.
- SKR03 goods purchase: `3400` is `Wareneingang 19 % Vorsteuer`, `3300` is `Wareneingang 7 % Vorsteuer`.
- SKR04 goods purchase: `5400` is `Wareneingang 19 % Vorsteuer`, `5300` is `Wareneingang 7 % Vorsteuer`.

Do not infer tax treatment from the account number alone when the booking carries an explicit Scopevisio `vatKey` or DATEV BU key. Prefer the booking row's tax key over a generic account mapping.

## CSV files

Read `references/skr03.csv` or `references/skr04.csv` after checking the configured `SKR`. The CSVs intentionally contain a compact set of high-frequency accounts for agent lookup, not a full replacement for the DATEV Kontenrahmen PDFs.

Columns:

- `Kontonummer`: four-digit DATEV account number.
- `Bezeichnung`: DATEV account label.
- `Klasse`: leading SKR class digit.
- `Steuerschlüssel`: quick hint for common VAT/input-tax cases; blank means no generic tax hint.

## SKR detection fallback

`sv-cli auth login` probes the Unternehmen and stores `SKR=skr03` or `SKR=skr04` in the scopeskill config. Agents should read that value first, for example with:

```bash
sv-cli auth show
```

If `SKR` is missing and the user cannot rerun `auth login`, use the documented fallback heuristic: query `/impersonalaccounts` for standard Erlöskonto numbers. A configured account `4400` with the ordinary 19 percent revenue label indicates SKR04; a configured account `8400` with the ordinary 19 percent revenue label indicates SKR03. Treat neither result as inconclusive and ask for confirmation rather than guessing.
