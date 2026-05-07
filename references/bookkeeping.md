# Bookkeeping System

Scopevisio is organized around business objects with profile-based access rights. Check the live Swagger before writing because required profiles and payload shapes vary by endpoint.

## Main Areas

- Contacts: `/contacts`, `/contact/new`, `/contact/{id}`.
- Products: `/products`, `/product/new`, `/product/{id}`.
- Projects: `/projects`, `/project/new`, `/project/{id}`.
- Opportunities, offers, orders, dispatches, outgoing invoices, credits, recurring invoices: document lifecycle endpoints support retrieval, positions, parent links, and conversions.
- Incoming invoices: `/incominginvoices`, `/incominginvoice/new`, `/incominginvoice/{id}`.
- Accounting: journal, postings, debitor/kreditor accounts, dimensions, fiscal years, open items, DATEV export.
- Tasks and activities: `/tasks`, `/task/new`, `/task/{id}`, comments, subtasks, file links.
- Reports and data sources: `/reports/{type}`, `/datasource/...`.

## Search Pattern

Many list endpoints are `POST` endpoints with a JSON body. Common keys:

```json
{
  "page": 0,
  "pageSize": 100,
  "fields": ["id", "name"],
  "search": [
    {"field": "name", "operator": "contains", "value": "Acme"}
  ],
  "order": ["name = asc"]
}
```

Use `pageSize <= 1000`. Use `count` only when the endpoint docs say count is supported.

Search operators use Scopevisio's no-space vocabulary, for example `contains`, `startswith`, `endswith`, `equals`, and `notequals`. Do not insert spaces inside operator names.

## Updates

Omit fields that should stay unchanged. Scopevisio treats some nullable fields differently: for nullable fields, `null` or `""` can clear data; for mandatory fields, `null` or `""` is ignored.

Custom field API names are not display names. They use `custom<field type><field id>`, for example `customText7`, `customLong3`, or `customDateTime4`. Multiple selected text values use `§§` as separator.

## Guardrails

- Before mutating bookkeeping data, read the endpoint from the live Swagger JSON.
- Confirm identifiers: many endpoints accept either internal IDs or document numbers, but not always both.
- Treat `404` as "not found or authorization missing".
- Preserve document numbers and accounting dates exactly.
- Prefer narrow helper commands and explicit JSON files over long inline JSON for complex writes.
