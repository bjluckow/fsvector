### Step 3 — verify

```sql
-- row counts should match
SELECT COUNT(DISTINCT path) FROM files_old WHERE deleted_at IS NULL;
SELECT COUNT(*) FROM files WHERE deleted_at IS NULL;

-- chunk counts should match
SELECT COUNT(*) FROM files_old;
SELECT COUNT(*) FROM chunks;

-- spot check a video
SELECT f.path, i.item_type, c.chunk_index, c.chunk_type,
    left(c.text_content, 50)
FROM files f
JOIN items i ON i.file_id = f.id
JOIN chunks c ON c.item_id = i.id
WHERE f.path LIKE '%videoplayback%'
ORDER BY i.item_index, c.chunk_index;
```

### Step 4 — switch application code

Update all packages that reference the old schema:
- `internal/store/` — upsert, delete, migrate, querier
- `internal/search/` — search, list, show, stats, normalize
- `internal/daemon/` — reconcile
- `internal/pipeline/` — all process functions

### Step 5 — drop old table

Only after application code verified working:

```sql
DROP TABLE files_old;
```

---

## Migration Binary — `cmd/migrate/main.go`

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/jackc/pgx/v5/pgxpool"
)

func main() {
    ctx := context.Background()
    
    dbURL := os.Getenv("DATABASE_URL")
    if dbURL == "" {
        dbURL = "postgres://fsvector:fsvector@localhost:5432/fsvector"
    }

    pool, err := pgxpool.New(ctx, dbURL)
    if err != nil {
        fmt.Fprintf(os.Stderr, "connect: %v\n", err)
        os.Exit(1)
    }
    defer pool.Close()

    if err := migrate(ctx, pool); err != nil {
        fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
        os.Exit(1)
    }

    fmt.Println("migration complete")
}

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
    // get all distinct paths from files_old
    rows, err := pool.Query(ctx, `
        SELECT DISTINCT path FROM files_old
        ORDER BY path
    `)
    if err != nil {
        return fmt.Errorf("list paths: %w", err)
    }
    var paths []string
    for rows.Next() {
        var path string
        rows.Scan(&path)
        paths = append(paths, path)
    }
    rows.Close()

    fmt.Printf("migrating %d files...\n", len(paths))

    for i, path := range paths {
        if err := migratePath(ctx, pool, path); err != nil {
            fmt.Fprintf(os.Stderr, "  %s: %v\n", path, err)
            continue
        }
        if i%100 == 0 {
            fmt.Printf("  %d/%d\r", i, len(paths))
        }
    }
    fmt.Printf("  %d/%d\n", len(paths), len(paths))
    return nil
}

func migratePath(ctx context.Context, pool *pgxpool.Pool, path string) error {
    // 1. get all chunks for this path from files_old
    rows, err := pool.Query(ctx, `
        SELECT
            source, canonical_path, content_hash, size, mime_type, modality,
            file_name, file_ext, file_created_at, file_modified_at,
            embed_model, embedding, chunk_index, chunk_type,
            metadata, text_content, indexed_at, deleted_at
        FROM files_old
        WHERE path = $1
        ORDER BY chunk_index
    `, path)
    if err != nil {
        return fmt.Errorf("query chunks: %w", err)
    }
    defer rows.Close()

    type oldRow struct {
        source        string
        canonicalPath *string
        contentHash   string
        size          *int64
        mimeType      *string
        modality      string
        fileName      *string
        fileExt       *string
        fileCreatedAt  *time.Time
        fileModifiedAt *time.Time
        embedModel    string
        embedding     pgvector.Vector
        chunkIndex    int
        chunkType     *string
        metadata      []byte
        textContent   *string
        indexedAt     time.Time
        deletedAt     *time.Time
    }

    var oldRows []oldRow
    for rows.Next() {
        var r oldRow
        if err := rows.Scan(
            &r.source, &r.canonicalPath, &r.contentHash,
            &r.size, &r.mimeType, &r.modality,
            &r.fileName, &r.fileExt, &r.fileCreatedAt, &r.fileModifiedAt,
            &r.embedModel, &r.embedding, &r.chunkIndex, &r.chunkType,
            &r.metadata, &r.textContent, &r.indexedAt, &r.deletedAt,
        ); err != nil {
            return fmt.Errorf("scan: %w", err)
        }
        oldRows = append(oldRows, r)
    }
    rows.Close()

    if len(oldRows) == 0 {
        return nil
    }

    first := oldRows[0]

    // 2. insert into files
    var fileID int64
    err = pool.QueryRow(ctx, `
        INSERT INTO files (
            path, source, canonical_path, modality,
            file_name, file_ext, mime_type, size, content_hash,
            file_created_at, file_modified_at, indexed_at, deleted_at
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
        )
        ON CONFLICT (path) DO UPDATE SET indexed_at = EXCLUDED.indexed_at
        RETURNING id
    `,
        path, first.source, first.canonicalPath, first.modality,
        first.fileName, first.fileExt, first.mimeType, first.size, first.contentHash,
        first.fileCreatedAt, first.fileModifiedAt, first.indexedAt, first.deletedAt,
    ).Scan(&fileID)
    if err != nil {
        return fmt.Errorf("insert file: %w", err)
    }

    // 3. determine items needed
    hasFrames := false
    hasTranscript := false
    for _, r := range oldRows {
        if r.chunkType != nil {
            switch *r.chunkType {
            case "frame":
                hasFrames = true
            case "transcript":
                hasTranscript = true
            }
        }
    }

    // insert items and collect their IDs
    itemIDs := map[string]int64{} // item_type -> id

    if hasFrames {
        var id int64
        pool.QueryRow(ctx, `
            INSERT INTO items (file_id, item_type, item_index)
            VALUES ($1, 'frames', 0) RETURNING id
        `, fileID).Scan(&id)
        itemIDs["frame"] = id
    }
    if hasTranscript {
        idx := 0
        if hasFrames {
            idx = 1
        }
        var id int64
        pool.QueryRow(ctx, `
            INSERT INTO items (file_id, item_type, item_index)
            VALUES ($1, 'transcript', $2) RETURNING id
        `, fileID, idx).Scan(&id)
        itemIDs["transcript"] = id
    }
    if !hasFrames && !hasTranscript {
        var id int64
        pool.QueryRow(ctx, `
            INSERT INTO items (file_id, item_type, item_index)
            VALUES ($1, 'whole', 0) RETURNING id
        `, fileID).Scan(&id)
        itemIDs["whole"] = id
    }

    // 4. insert chunks — reset chunk_index within each item
    itemChunkCount := map[string]int{}

    for _, r := range oldRows {
        itemType := "whole"
        if r.chunkType != nil {
            switch *r.chunkType {
            case "frame":
                itemType = "frame"
            case "transcript":
                itemType = "transcript"
            }
        }

        itemID := itemIDs[itemType]
        chunkIdx := itemChunkCount[itemType]
        itemChunkCount[itemType]++

        _, err = pool.Exec(ctx, `
            INSERT INTO chunks (
                item_id, chunk_index, chunk_type, embed_model,
                embedding, text_content, metadata, indexed_at
            ) VALUES (
                $1, $2, $3, $4, $5, $6, $7, $8
            )
            ON CONFLICT (item_id, chunk_index) DO NOTHING
        `,
            itemID, chunkIdx, r.chunkType, r.embedModel,
            r.embedding, r.textContent, r.metadata, r.indexedAt,
        )
        if err != nil {
            return fmt.Errorf("insert chunk: %w", err)
        }
    }

    return nil
}
```

---

## Makefile

```makefile
FSMIGRATE := $(BINDIR)/fsmigrate

$(FSMIGRATE):
	@mkdir -p $(BINDIR)
	go build -o $@ ./cmd/migrate

build: tidy $(FSVECTORD) $(FSVECTOR) $(FSCLUSTER) $(FSMIGRATE)
```

---

## Rollback

If migration fails or verification fails:

```sql
-- restore old table
DROP TABLE chunks;
DROP TABLE items;
DROP TABLE files;
ALTER TABLE files_old RENAME TO files;
```

Application code unchanged until Step 4, so rollback is instant.

---

## Milestones

### M-migrate.1 — backup and new schema
**Goal:** pg_dump complete. New tables created alongside `files_old`.
No data moved yet.

**Verify:**
```bash
# backup exists and is complete
ls -lh backup-20260414.sql
awk '/^COPY/,/^\\\./' backup-20260414.sql | grep -c "^[^\\]"
# → 27456

# new tables exist
docker compose exec postgres psql -U fsvector -c "\dt"
# → files, files_old, items, chunks
```

---

### M-migrate.2 — migration binary
**Goal:** `cmd/migrate/main.go` runs successfully against a test DB.

**Verify on test DB first:**
```bash
# create test DB from backup
createdb fsvector_test
psql fsvector_test < backup-20260414.sql

# run migration against test DB
DATABASE_URL=postgres://fsvector:fsvector@localhost:5432/fsvector_test \
./bin/fsmigrate

# verify counts
docker compose exec postgres psql -U fsvector_test -c "
SELECT COUNT(*) FROM files;    -- should match distinct paths
SELECT COUNT(*) FROM items;    -- should match file count + video extras
SELECT COUNT(*) FROM chunks;   -- should match 27456"
```

---

### M-migrate.3 — verify migration correctness
**Goal:** Spot-check data integrity across all modalities.

```sql
-- image: 1 file, 1 item, N chunks
SELECT f.path, i.item_type, COUNT(c.id) as chunks
FROM files f
JOIN items i ON i.file_id = f.id
JOIN chunks c ON c.item_id = i.id
WHERE f.modality = 'image'
GROUP BY f.path, i.item_type
LIMIT 5;

-- video: 1 file, 2 items (frames + transcript), N+M chunks
SELECT f.path, i.item_type, COUNT(c.id) as chunks
FROM files f
JOIN items i ON i.file_id = f.id
JOIN chunks c ON c.item_id = i.id
WHERE f.modality = 'video'
GROUP BY f.path, i.item_type
ORDER BY f.path, i.item_type;

-- embeddings preserved
SELECT COUNT(*) FROM chunks WHERE embedding IS NOT NULL;
-- should match old: SELECT COUNT(*) FROM files_old WHERE embedding IS NOT NULL
```

---

### M-migrate.4 — update application code
**Goal:** All store/, search/, pipeline/ updated to use new schema.
fsvectord and fsvector work identically to before.

**Verify:**
```bash
./bin/fsvector search "dog" --modality image
./bin/fsvector stats
./bin/fsvector ls
curl http://localhost:8080/search -X POST \
  -H "Content-Type: application/json" \
  -d '{"query":"roof damage","limit":5}'
```

---

### M-migrate.5 — drop files_old
**Goal:** Old table removed after 48h of verified operation.

```sql
DROP TABLE files_old;
```