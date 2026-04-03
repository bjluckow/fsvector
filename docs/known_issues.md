# Known Issues

### KI-006: /files?path= always returns soft-deleted files
The `/files?path=` endpoint returns file details regardless of
`deleted_at` status. There is no way to distinguish "show me this
file's metadata even if deleted" from "only show live files".

**Fix:** Add `?include_deleted=true` flag to `/files?path=` endpoint,
defaulting to false so deleted files are excluded unless explicitly
requested. Useful for debugging and audit purposes.