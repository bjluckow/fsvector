# Known Issues

## Search Quality

### KI-001: Short files produce noisy embeddings
Short files (under ~50 chars) produce weak, generic embeddings that rank
highly across unrelated queries. `text.txt` containing "hello world" ranks
first for almost every search query.

**Fix:** Add a minimum content length threshold in `internal/pipeline/pipeline.go`
before the embed call. Skip files under 50 characters with a `SkipReason`.

---

### KI-002: Text-to-image score gap
Text queries return image results with significantly lower cosine scores
(0.1-0.2) compared to text results (0.5-0.8), even when the image is
semantically relevant. `dog.webp` scores 0.2689 for "a dog running outside"
while unrelated PDFs score 0.5+.

**Cause:** CLIP's cross-modal alignment is weaker with short/single-word
queries vs natural language phrases. Text-to-text similarity naturally
produces higher cosine scores than text-to-image.

**Fix options:**
- Add `--modality` flag to `fsvector search` so users can search images
  only, making relative ranking within modality more meaningful
- Normalize scores separately per modality before returning results
- Document that descriptive phrases ("a photograph of a dog") work better
  than single words ("dog") for image search

---

### KI-003: Corrupt or non-standard PDFs silently fail
`github.pdf` fails with `pdftotext` error:
`Syntax Error: Couldn't find trailer dictionary`

The file is likely corrupt or saved in a non-standard format. The pipeline
logs the error and moves on, but the file is never indexed.

**Fix options:**
- Add a fallback to OCR (e.g. `tesseract`) for PDFs that pdftotext cannot
  parse
- Embed filename/metadata only as a fallback so the file is at least
  discoverable by name