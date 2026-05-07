# Teamworkbridge

Scopevisio exposes CenterDevice Teamwork resources under `/teamworkbridge/...`. Use the Scopevisio base URL and Scopevisio bearer token, not the CenterDevice base URL or OAuth flow.

Scopevisio maps the CenterDevice path segments into placeholder endpoints:

- CenterDevice `/document/<document-id>` -> Scopevisio `/teamworkbridge/document/<document-id>`
- CenterDevice `/folders?parent=none&collection=<collection-id>` -> Scopevisio `/teamworkbridge/folders?parent=none&collection=<collection-id>`
- CenterDevice `/documents` -> Scopevisio `/teamworkbridge/documents`

The authenticated user also needs Teamwork rights in Scopevisio: System administration -> DMS Teamwork -> Manage users.

## Access

Retrieve a document resource:

```bash
sv-cli get /teamworkbridge/document/<document-id>
```

List top-level folders of a collection:

```bash
sv-cli get /teamworkbridge/folders \
  --query parent=none \
  --query collection=<collection-id>
```

List collections:

```bash
sv-cli get /teamworkbridge/collections --query all=true
```

## Download

Download the latest document bytes:

```bash
sv-cli download /teamworkbridge/document/<document-id> --out ./document.bin
```

CenterDevice's document download response should include `Content-Disposition`, `Content-Type`, and `Content-Length`, but the current helper writes to the `--out` path supplied by the caller. Choose the extension yourself if the response filename is unavailable.

## Upload

Upload a new document with multipart form data:

```bash
sv-cli teamwork upload ./invoice.pdf \
  --collection <collection-id> \
  --tag scopevisio-test
```

Equivalent metadata shape:

```json
{
  "metadata": {
    "document": {
      "filename": "invoice.pdf",
      "size": 12345,
      "title": "Invoice",
      "author": "Automation"
    },
    "actions": {
      "add-to-collection": ["collection-id"],
      "add-tag": ["scopevisio-test"]
    },
    "extended-metadata": {}
  }
}
```

Provide custom metadata from a file:

```bash
sv-cli teamwork upload ./invoice.pdf --metadata @metadata.json
```

The helper fills `metadata.document.filename` and `metadata.document.size` from the file if omitted. It posts to `/teamworkbridge/documents`.

## Current Command Shape

The current implementation uses generic `get`, `post`, and `download` commands for simple HTTP calls and `teamwork upload` for multipart document upload. Folder and collection reads remain generic JSON calls until live Teamworkbridge tests show a strong reason to add convenience commands.

## New Version

CenterDevice supports uploading a new version by posting multipart content to `/document/<document-id>`. Through Scopevisio, that maps to `/teamworkbridge/document/<document-id>`. The helper does not yet expose a dedicated command for this; add it before doing version writes repeatedly.

## Error Interpretation

- `400`: often invalid payload, missing Teamwork setup, or file size mismatch.
- `401`/`403`: token invalid, insufficient Scopevisio profile, or insufficient Teamwork rights.
- `404`: document/collection/folder does not exist or is not visible to the user.
