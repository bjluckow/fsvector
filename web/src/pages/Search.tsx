import { useState, useCallback } from "react";
import { search } from "../lib/api";
import { SearchBar } from "../components/SearchBar";
import { ResultCard } from "../components/ResultCard";
import type { FileResult, Modality } from "../lib/types";
import type { SearchResult } from "../lib/types";

function groupByFile(results: SearchResult[]): FileResult[] {
  const map = new Map<string, FileResult>();

  for (const r of results) {
    const existing = map.get(r.path);
    if (existing) {
      if (r.norm_score > existing.norm_score) {
        existing.norm_score = r.norm_score;
      }
      if (r.text_content) {
        existing.snippets.push(r.text_content);
      }
    } else {
      map.set(r.path, {
        path: r.path,
        file_name: r.file_name,
        modality: r.modality,
        ext: r.ext,
        size: r.size,
        norm_score: r.norm_score,
        file_created_at: r.file_created_at,
        file_modified_at: r.file_modified_at,
        snippets: r.text_content ? [r.text_content] : [],
      });
    }
  }

  return Array.from(map.values());
}

export function Search() {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<FileResult[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);
  const [modality, setModality] = useState<Modality | undefined>();

  const handleSearch = useCallback(
    async (q: string) => {
      if (!q.trim()) return;
      setLoading(true);
      setSearched(true);
      try {
        const res = await search({ q, limit: 50, modality });
        setResults(groupByFile(res.results));
        setTotal(res.total);
      } catch (e) {
        console.error(e);
      } finally {
        setLoading(false);
      }
    },
    [modality]
  );

  const filters: (Modality | "all")[] = ["all", "image", "text", "audio", "video", "email"];

  return (
    <div className="search-page">
      <header className="search-header">
        <h1>fsvector</h1>
        <SearchBar
          value={query}
          onChange={setQuery}
          onSubmit={() => handleSearch(query)}
          loading={loading}
        />
        <div className="modality-filters">
          {filters.map((f) => (
            <button
              key={f}
              className={`filter-pill ${(f === "all" && !modality) || f === modality ? "active" : ""}`}
              onClick={() => {
                setModality(f === "all" ? undefined : f);
                if (query.trim()) handleSearch(query);
              }}
            >
              {f}
            </button>
          ))}
        </div>
      </header>

      <main className="search-results">
        {loading && <div className="search-status">Searching…</div>}

        {!loading && searched && results.length === 0 && (
          <div className="search-status">No results found</div>
        )}

        {!loading && results.length > 0 && (
          <>
            <div className="results-count">{total} results</div>
            {results.map((r) => (
              <ResultCard key={r.path} result={r} />
            ))}
          </>
        )}
      </main>
    </div>
  );
}