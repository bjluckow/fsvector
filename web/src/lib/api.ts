import type { SearchParams, SearchResponse } from "./types";

const API_BASE = "/api";

export async function search(params: SearchParams): Promise<SearchResponse> {
  const res = await fetch(`${API_BASE}/search`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(params),
  });
  if (!res.ok) throw new Error(`Search failed: ${res.status}`);
  return res.json();
}