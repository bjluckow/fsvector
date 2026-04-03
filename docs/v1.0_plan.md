# fsvector v1 Plan

## What It Is

A local-first, Dockerized toolset for applying vector embeddings to file system (and eventually S3)
directory trees — enabling semantic search, classification, and transformation of unstructured files.
It never modifies source data. Everything runs locally via a single `docker compose up`.

---

## Binaries

**`fsvectord`** — long-running daemon. Watches a source directory (mounted read-only), reconciles
filesystem state against Postgres on startup, then hands off to `fsnotify` for incremental changes.
Drives the full pipeline: detect → convert → embed → store. Soft-deletes removed files.

**`fsvector`** — query CLI. Pure reader. Talks only to Postgres. Subcommands: `search`, `ls`,
`show`, `stats`.

---

## Services

| Service | Image / Build | Role |
|---|---|---|
| `postgres` | `pgvector/pgvector:pg16` | Vector store + file index |
| `embedsvc` | custom Python/FastAPI | Loads one model on startup, exposes `/embed` and `/health` |
| `convertd` | custom Python/FastAPI | Wraps ImageMagick + Pandoc, exposes `/convert` and `/health` |
| `fsvectord` | custom Go | The daemon (compose service) |
| `fsvector` | custom Go | Query CLI (one-shot `docker compose run --rm`) |

---

## Pipeline (per file event)

```
FS event / startup walk
  → Detect      mime type via magic bytes
  → Route       is this type supported? skip + log if not
  → Convert     POST file to convertd → normalized artifact (HEIC→JPEG, DOCX→text, etc.)
  → Chunk       split large text into overlapping chunks (optional, configurable)
  → Embed       POST text or image bytes to embedsvc → []float32
  → Store       upsert to pgvector (soft-delete on removal)
```

---

## Startup Reconciliation

On `fsvectord` start, before fsnotify kicks in:

1. Walk FS → `map[path]hash`
2. Query DB → `map[path]{hash, deleted_at}`
3. Diff:
   - In FS, not in DB → insert + embed
   - In FS, in DB, hash changed → re-embed + update
   - In FS, in DB, was soft-deleted → un-delete, re-embed if hash changed
   - In DB (live), not in FS → soft-delete
4. Hand off to fsnotify for incremental events

---

## Directory Structure

```
fsvector/
├── cmd/
│   ├── fsvectord/
│   │   └── main.go               # daemon entrypoint
│   └── fsvector/
│       └── main.go               # query CLI entrypoint
│
├── internal/
│   ├── config/
│   │   └── config.go             # shared env/flag parsing
│   ├── fsindex/
│   │   └── walk.go               # recursive walk, sha256 hash, mime sniff
│   ├── watcher/
│   │   └── watcher.go            # fsnotify wrapper, typed events
│   ├── pipeline/
│   │   └── pipeline.go           # orchestrates convert → embed → store per file
│   ├── convert/
│   │   └── client.go             # HTTP client for convertd
│   ├── embed/
│   │   └── client.go             # HTTP client for embedsvc
│   └── store/
│       ├── upsert.go
│       ├── delete.go
│       ├── query.go              # cosine search, ls, show
│       └── migrate.go            # schema bootstrap
│
├── services/
│   ├── embedsvc/
│   │   ├── main.py               # FastAPI: one model, POST /embed, GET /health
│   │   ├── requirements.txt
│   │   └── Dockerfile
│   └── convertd/
│       ├── main.py               # FastAPI: ImageMagick + Pandoc, POST /convert
│       ├── requirements.txt
│       └── Dockerfile
│
├── sql/
│   └── schema.sql
│
├── docker/
│   └── Dockerfile                # multi-stage: builds both Go binaries
│
├── docs/
│   └── v1_plan.md                # this file
│
├── docker-compose.yml
├── install.sh                    # copies fsvector shell wrapper to ~/bin
├── .env.example
├── Makefile
├── go.mod
└── go.sum
```

---

## Schema

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE files (
    id                BIGSERIAL PRIMARY KEY,
    path              TEXT NOT NULL,
    content_hash      TEXT NOT NULL,
    size              BIGINT,
    mime_type         TEXT,
    modality          TEXT NOT NULL,        -- 'text' | 'image'
    embed_model       TEXT NOT NULL,
    embedding         vector(384),
    chunk_index       INT NOT NULL DEFAULT 0,
    metadata          JSONB,
    indexed_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ,
    source            TEXT                  -- 'local' | 's3://...'
);

CREATE UNIQUE INDEX files_path_chunk_idx ON files (path, chunk_index);
CREATE INDEX ON files USING hnsw (embedding vector_cosine_ops)
    WHERE deleted_at IS NULL;
```

---

## Key Dependencies

**Go**
- `github.com/fsnotify/fsnotify` — filesystem watching
- `github.com/jackc/pgx/v5` — Postgres driver
- `github.com/pgvector/pgvector-go` — vector type for pgx
- `github.com/gabriel-vasile/mimetype` — magic byte mime detection
- `github.com/spf13/cobra` — CLI for `fsvector`
- `github.com/spf13/viper` — config/env

**Python (embedsvc):** `fastapi`, `uvicorn`, `sentence-transformers`, `numpy`

**Python (convertd):** `fastapi`, `uvicorn`, `python-magic` + shells out to `imagemagick` and `pandoc`

---

## Installation & Usage

```bash
# One-time setup
git clone https://github.com/you/fsvector ~/.fsvector
cd ~/.fsvector
cp .env.example .env          # set SOURCE_DIR, EMBED_MODEL, etc.
./install.sh                  # installs fsvector shell wrapper to ~/bin

# Start the stack
fsvector daemon start         # docker compose up -d

# Query
fsvector search "Q3 budget spreadsheet"
fsvector ls --since 24h
fsvector show ~/Documents/report.pdf
fsvector stats

# Stop
fsvector daemon stop
```

`install.sh` places a small shell script in `~/bin/fsvector` that delegates to
`docker compose run --rm fsvector` for queries, and wraps compose lifecycle commands
under `fsvector daemon <start|stop|status|logs>`.

---

## Design Principles

- Source directory always mounted `:ro` — never touched
- Postgres is the working copy and the only shared state
- `fsvectord` and `fsvector` share nothing at runtime except the DB
- `convertd` and `embedsvc` are stateless HTTP services — swappable, replaceable
- One model loaded at startup in `embedsvc` — no runtime model switching
- Soft-delete only — no rows are ever hard-deleted
- Everything runs locally via `docker compose up`
- No Go toolchain required by the end user

---

## Milestones

### M1 — Repo scaffold + Go module compiles
**Goal:** Empty but valid project structure. Both Go binaries build. Nothing runs yet.

Files:
- `go.mod`, `go.sum`
- `cmd/fsvectord/main.go` — prints "fsvectord starting" and exits
- `cmd/fsvector/main.go` — cobra root command, prints help and exits
- `internal/config/config.go` — struct + `Load()` reading env vars
- `Makefile` with `build`, `clean` targets
- `.env.example`
- `docs/v1_plan.md`

**Verify:** `make build` produces two binaries with no errors.

---

### M2 — Postgres + pgvector running, schema applied
**Goal:** Database is up, schema is applied automatically on first start, and can be inspected with `psql`.

Files:
- `sql/schema.sql`
- `docker-compose.yml` (postgres service only)
- `internal/store/migrate.go` — runs schema.sql on connect if tables don't exist

**Verify:**
```bash
docker compose up -d postgres
docker compose exec postgres psql -U fsvector -c "\d files"
# → shows table with vector column
```

---

### M3 — embedsvc running, /health and /embed verified
**Goal:** Python embedding service starts, loads model, responds to requests.

Files:
- `services/embedsvc/main.py`
- `services/embedsvc/requirements.txt`
- `services/embedsvc/Dockerfile`
- Add `embedsvc` to `docker-compose.yml`

**Verify:**
```bash
docker compose up -d embedsvc
curl http://localhost:8000/health
# → {"status":"ok","model":"...","dim":384}
curl -X POST http://localhost:8000/embed \
  -H "Content-Type: application/json" \
  -d '{"texts":["hello world"]}'
# → {"embeddings":[[0.023, ...]]}  (384 floats)
```

---

### M4 — convertd running, /health and /convert verified
**Goal:** Conversion service starts, can convert a HEIC to JPEG and a DOCX to text.

Files:
- `services/convertd/main.py`
- `services/convertd/requirements.txt`
- `services/convertd/Dockerfile`
- Add `convertd` to `docker-compose.yml`

**Verify:**
```bash
docker compose up -d convertd
curl http://localhost:8001/health
# → {"status":"ok","backends":["imagemagick","pandoc"]}
curl -X POST http://localhost:8001/convert \
  -F "file=@test.heic" -F "target_format=jpeg" \
  --output out.jpg
# → valid JPEG on disk
curl -X POST http://localhost:8001/convert \
  -F "file=@test.docx" -F "target_format=txt" \
  --output out.txt
# → readable plain text
```

---

### M5 — Go HTTP clients for embedsvc and convertd
**Goal:** Go code can call both services. No filesystem or DB involvement yet.

Files:
- `internal/embed/client.go` — `Embed(ctx, []string) ([][]float32, error)`
- `internal/convert/client.go` — `Convert(ctx, path, targetFormat) ([]byte, error)`

**Verify:** A small `_test.go` or throwaway `main.go` calls both clients against the live
containers and prints results. Both services must be running.

---

### M6 — fsindex: walk + hash + mime detect
**Goal:** Given a root path, return a list of all files with path, sha256 hash, size, and mime type.

Files:
- `internal/fsindex/walk.go`

**Verify:** Unit test walking a small fixture directory (`testdata/`) with known files.
Output should be deterministic: same files → same hashes every run.

---

### M7 — store: upsert and soft-delete
**Goal:** Go code can write file records (with placeholder embeddings) to Postgres and soft-delete them.

Files:
- `internal/store/upsert.go`
- `internal/store/delete.go`

**Verify:**
```bash
# Run a small integration test that:
# 1. Upserts 3 rows
# 2. Confirms they appear in the DB
# 3. Soft-deletes 1 row
# 4. Confirms deleted_at is set, row still exists
```

---

### M8 — pipeline: wire convert → embed → store for a single file
**Goal:** Given a file path, the full pipeline runs end-to-end and produces a row in Postgres
with a real embedding vector.

Files:
- `internal/pipeline/pipeline.go`

**Verify:**
```bash
# All four services running.
# Point pipeline at a single test file (e.g. a .txt and a .jpg).
# Check DB: row exists, embedding is non-null, modality is correct.
SELECT path, modality, embed_model, deleted_at,
       array_length(embedding::real[], 1) AS dim
FROM files;
```

---

### M9 — fsvectord: startup reconciliation
**Goal:** On start, `fsvectord` walks `WATCH_PATH`, diffs against DB state, and runs the pipeline
for new/changed files. Soft-deletes files no longer on disk. Then exits cleanly (no watcher yet).

Files:
- `cmd/fsvectord/main.go` (reconcile loop, no fsnotify yet)
- Add `fsvectord` to `docker-compose.yml`

**Verify:**
```bash
docker compose up fsvectord
# Check logs: each file processed, summary line at end
# Check DB: all files in SOURCE_DIR have rows with embeddings
# Remove a file, re-run fsvectord
# Check DB: that file now has deleted_at set
```

---

### M10 — fsvectord: fsnotify live watching
**Goal:** After reconciliation, `fsvectord` stays running and reacts to filesystem events in real time.

Files:
- `internal/watcher/watcher.go`
- Update `cmd/fsvectord/main.go` to start watcher after reconcile

**Verify:**
```bash
docker compose up -d fsvectord
# While running:
echo "new content" > $SOURCE_DIR/test_new.txt
sleep 3
# DB should have a new row for test_new.txt

rm $SOURCE_DIR/test_new.txt
sleep 3
# DB row for test_new.txt should now have deleted_at set
```

---

### M11 — fsvector CLI: search, ls, show, stats
**Goal:** `fsvector` can query the DB meaningfully. `search` embeds the query string via embedsvc
and returns nearest neighbours by cosine similarity.

Files:
- `internal/store/query.go`
- `cmd/fsvector/main.go` — cobra subcommands: `search`, `ls`, `show`, `stats`

**Verify:**
```bash
fsvector ls
# → table of indexed files with path, modality, indexed_at

fsvector search "quarterly budget"
# → top 5 results with path and similarity score

fsvector show ./documents/report.pdf
# → metadata: mime, size, model, indexed_at, chunk count

fsvector stats
# → total files, deleted files, modality breakdown, model in use
```

---

### M12 — install.sh + shell wrapper
**Goal:** A user can clone the repo, run `install.sh`, and use `fsvector` as a native-feeling command.

Files:
- `install.sh`
- `bin/fsvector.sh` — the shell wrapper template

**Verify:**
```bash
./install.sh
which fsvector
# → ~/bin/fsvector

fsvector daemon start
fsvector search "test query"
fsvector daemon stop
fsvector daemon logs
```

---

### M13 — Dockerfile + full compose stack verified end-to-end
**Goal:** `docker compose up` from a clean clone brings up the entire stack. No host Go toolchain needed.

Files:
- `docker/Dockerfile` — multi-stage build for both Go binaries
- Final `docker-compose.yml` with all services, healthchecks, and `depends_on` conditions

**Verify:**
```bash
git clone ... && cd fsvector
cp .env.example .env
# Set SOURCE_DIR to a real directory with mixed file types
./install.sh
fsvector daemon start
# Wait for services to be healthy
fsvector stats
fsvector search "some term relevant to your files"
```

All services healthy, embeddings present, search returns results. End-to-end working.
