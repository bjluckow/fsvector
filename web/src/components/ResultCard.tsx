import type { FileResult } from "../lib/types";

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

const modalityIcon: Record<string, string> = {
  image: "🖼",
  text: "📄",
  audio: "🎵",
  video: "🎬",
  email: "✉️",
};

export function ResultCard({ result }: { result: FileResult }) {
  const score = Math.round(result.norm_score * 100);

  return (
    <div className={`result-card modality-${result.modality}`}>
      <div className="result-header">
        <span className="result-icon">{modalityIcon[result.modality] ?? "📎"}</span>
        <span className="result-filename">{result.file_name}</span>
        <span className="result-score">{score}%</span>
      </div>
      <div className="result-path">{result.path}</div>
      {result.snippets.length > 0 && (
        <div className="result-snippets">
          {result.snippets.slice(0, 3).map((s, i) => (
            <p key={i} className="result-snippet">
              {s.length > 200 ? s.slice(0, 200) + "…" : s}
            </p>
          ))}
        </div>
      )}
      <div className="result-meta">
        <span>{result.ext}</span>
        <span>{formatSize(result.size)}</span>
       {result.file_modified_at && (
          <span>{new Date(result.file_modified_at).toLocaleDateString()}</span>
        )}
      </div>
    </div>
  );
}