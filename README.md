# fsvector (WORK IN PROGRESS)

A local-first tool for applying vector embeddings to file system directories,
enabling semantic search and classification of unstructured files. Embedding models are hot-swappable, making it easy to test and compare results locally.

## Author's Note

Hi, this is [Ben](https://www.benjaminluckow.com), the owner of this repo and as far as I'm aware, not an AI agent. This project was created for and driven by a real-world use case with lots of multimodal data and strict time constraints. As such, AI tools were used to expedite code generation and solo design iteration. Rest assured that all committed code has been thoroughly reviewed and tested, all design choices directed, and all flaws curiously invented, by me - a human. 

## Overview

`fsvectord` watches a directory, converts and embeds its contents, and stores
the results in a local pgvector database. `fsvector` queries that index with
natural language search, filters, and metadata inspection.

## Requirements

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose
- [Go](https://go.dev/dl/) 1.25+

## Setup
```bash
git clone https://github.com/bjluckow/fsvector
cd fsvector
cp .env.example .env       # set SOURCE_DIR to the path you want indexed
./scripts/install.sh       # builds and installs the fsvector binary
./scripts/start.sh         # starts the docker stack
```

## Usage
```bash
# search
fsvector search "dog" --modality image
fsvector search "quarterly budget report" --ext pdf --since 30d

# browse
fsvector ls
fsvector ls --ext pdf --since 7d
fsvector show /path/to/file.pdf

# stats
fsvector stats
```

## Architecture
```
fsvectord        long-running daemon — watches filesystem, embeds, stores
fsvector         query CLI — search, list, inspect
embedsvc         Python/FastAPI — CLIP embeddings (text + image)
convertsvc       Python/FastAPI — file conversion (ImageMagick + Pandoc)
postgres         pgvector — vector store and file index
```

## Docs

- [v1 Plan](docs/v1_plan.md)
- [v1.1 Plan](docs/v1_1_plan.md)
- [Known Issues](docs/known_issues.md)

## License

MIT
