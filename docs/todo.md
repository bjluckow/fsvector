### v1.8 items to add:
- Worker pools for concurrent indexing
- Image search by file upload: `fsvector search --image /path/to/query.jpg`
  → reads image bytes, sends to /embed/image, searches by resulting vector
- Multiple search variations in search/search.go (pure vector, hybrid, full-text)
- fsvector reindex --purge