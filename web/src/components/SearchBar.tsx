interface SearchBarProps {
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
  loading: boolean;
}

export function SearchBar({ value, onChange, onSubmit, loading }: SearchBarProps) {
  return (
    <form
      className="search-bar"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit();
      }}
    >
      <input
        type="text"
        placeholder="Search files…"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        autoFocus
      />
      <button type="submit" disabled={loading || !value.trim()}>
        {loading ? "…" : "Search"}
      </button>
    </form>
  );
}