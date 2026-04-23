// -- Common --

export const MODALITIES = ["image", "text", "audio", "video", "email"] as const;
export type Modality = typeof MODALITIES[number];

// -- Search --

export interface SearchResult {
  path: string;
  score: number;
  norm_score: number;
  modality: Modality;
  ext: string;
  size: number;
  text_content: string;
  file_name: string;
  chunk_type: string;
  file_created_at: string;
  file_modified_at: string;
}

export interface SearchResponse {
  results: SearchResult[];
  query: string;
  total: number;
}

export interface SearchParams {
  q: string;
  limit?: number;
  modality?: Modality;
  ext?: string;
}

// -- Status --

export interface ReindexStatus {
  running: boolean;
  total: number;
  indexed: number;
  deleted: number;
  skipped: number;
  errors: string[];
  started_at: string | null;
  finished_at: string | null;
}

export interface StatusResponse {
  status: string;
  source: string;
  started_at: string;
  reindex: ReindexStatus;
}

// -- Stats --

export interface StatsResponse {
  model: string;
  total: number;
  text: number;
  image: number;
  audio: number;
  video: number;
  deleted: number;
  duplicates: number;
}

// -- Grouped (client-side, derived from SearchResult[]) --

export interface FileResult {
  path: string;
  file_name: string;
  modality: Modality;
  ext: string;
  size: number;
  norm_score: number;
  file_created_at: string;
  file_modified_at: string;
  snippets: string[];
}