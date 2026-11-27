// A vote changes ranking across searches, not just the current list.
import { useCallback, useEffect, useState } from "react";
import type { Result } from "./types";

export type Dir = -1 | 0 | 1;

export interface VoteCtx {
  enabled: boolean;
  vote: (r: Result, position: number, dir: Dir) => void;
  dirOf: (r: Result) => Dir;
}

// Refresh after saving even during streaming: the last snapshot might already
// be in flight and contain the old ranking.
export function useVotes(q: string, category: string, first: number, enabled: boolean, refresh: () => void): VoteCtx {
  const [local, setLocal] = useState<Record<string, Dir>>({});
  useEffect(() => setLocal({}), [q, category]);
  const vote = useCallback(
    (r: Result, position: number, dir: Dir) => {
      setLocal((m) => ({ ...m, [r.url]: dir }));
      postVote(q, category, r, first - 1 + position, dir)
        .then(refresh)
        .catch(() =>
          setLocal((m) => {
            const { [r.url]: _, ...rest } = m;
            return rest;
          }),
        );
    },
    [q, category, first, refresh],
  );
  const dirOf = useCallback((r: Result): Dir => local[r.url] ?? r.vote ?? 0, [local]);
  return { enabled, vote, dirOf };
}

export async function postVote(q: string, category: string, r: Result, position: number, dir: Dir) {
  const res = await fetch("/api/vote", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query: q, category, url: r.url, title: r.title, snippet: r.snippet, engines: r.engines, position, dir }),
  });
  if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || res.statusText);
}

export function VoteButtons({ r, i, v }: { r: Result; i: number; v: VoteCtx }) {
  if (!v.enabled) return null;
  const d = v.dirOf(r);
  const btn = (dir: 1 | -1, glyph: string, label: string) => (
    <button
      type="button"
      className={`vbtn${d === dir ? " on" : ""}`}
      title={d === dir ? "Undo" : label}
      aria-label={d === dir ? `Undo ${label.toLowerCase()}` : label}
      aria-pressed={d === dir}
      onClick={() => v.vote(r, i + 1, d === dir ? 0 : dir)}
    >
      {glyph}
    </button>
  );
  return (
    <span className={`votes${d ? " voted" : ""}`}>
      {btn(1, "\u25B2", "Rank higher")}
      {btn(-1, "\u25BC", "Rank lower")}
    </span>
  );
}

// "Ranked higher by your votes" for results moved by votes on other searches or pages.
export function PersonalNote({ r, v }: { r: Result; v: VoteCtx }) {
  if (!v.enabled || !r.personal || v.dirOf(r)) return null;
  return (
    <span className={`personal${r.personal < 0 ? " lower" : ""}`} title={`score x${Math.pow(2, r.personal).toFixed(2)} from your past votes`}>
      {" - "}
      {r.personal > 0 ? "ranked higher" : "ranked lower"} by your votes
    </span>
  );
}
