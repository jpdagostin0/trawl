// Small helpers shared by the pages.
import { useEffect, useState } from "react";
import type { Config } from "./types";

export async function getJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const r = await fetch(url, { signal });
  let j;
  try {
    j = await r.json();
  } catch {
    throw new Error(`${r.status} ${r.statusText}`);
  }
  if (!r.ok) throw new Error(j.error || `${r.status} ${r.statusText}`);
  return j;
}

export interface Fetched<T> {
  data: T | null;
  error: Error | null;
  loading: boolean;
}

const isAbort = (e: unknown) => e instanceof DOMException && e.name === "AbortError";

// Fetch JSON for `url` (null = skip). Aborts when url changes.
export function useJSON<T>(url: string | null): Fetched<T> {
  const [state, setState] = useState<Fetched<T>>({ data: null, error: null, loading: !!url });
  useEffect(() => {
    if (!url) return;
    const ctrl = new AbortController();
    setState((s) => ({ ...s, loading: true, error: null }));
    getJSON<T>(url, ctrl.signal)
      .then((data) => setState({ data, error: null, loading: false }))
      .catch((error: Error) => !isAbort(error) && setState({ data: null, error, loading: false }));
    return () => ctrl.abort();
  }, [url]);
  return state;
}

// Fold newline-delimited events in order; frames in one browser read can share a render.
export function useNDJSON<T, Frame>(url: string | null, merge: (data: T | null, frame: Frame) => T | null): Fetched<T> {
  const [state, setState] = useState<Fetched<T>>({ data: null, error: null, loading: !!url });
  useEffect(() => {
    if (!url) return;
    const ctrl = new AbortController();
    setState((s) => ({ ...s, loading: true, error: null }));
    let data: T | null = null;
    (async () => {
      const r = await fetch(url, { signal: ctrl.signal });
      if (!r.ok || !r.body) {
        const j = await r.json().catch(() => ({}));
        throw new Error(j.error || `${r.status} ${r.statusText}`);
      }
      const reader = r.body.pipeThrough(new TextDecoderStream()).getReader();
      let buf = "";
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        buf += value;
        const lines = buf.split("\n");
        buf = lines.pop() ?? "";
        if (ctrl.signal.aborted) return;
        for (const line of lines) {
          if (line.trim()) data = merge(data, JSON.parse(line) as Frame);
        }
        if (lines.length) setState({ data, error: null, loading: true });
      }
      if (!ctrl.signal.aborted) setState({ data, error: null, loading: false });
    })().catch((error: Error) => !ctrl.signal.aborted && setState({ data, error, loading: false }));
    return () => ctrl.abort();
  }, [url, merge]);
  return state;
}

let configPromise: Promise<Config> | undefined;
const fallbackConfig: Config = { categories: [{ id: "general", engines: [] }], bangs: {}, tab_bangs: {} };
export function useConfig() {
  const [cfg, setCfg] = useState(fallbackConfig);
  useEffect(() => {
    configPromise ??= getJSON<Config>("/api/config").catch(() => fallbackConfig);
    configPromise.then(setCfg);
  }, []);
  return cfg;
}

// All third-party images go through trawl's proxy.
export const proxied = (u?: string) => (u ? "/img?u=" + encodeURIComponent(u) : "");

export function host(u: string) {
  try {
    return new URL(u).hostname.replace(/^www\./, "");
  } catch {
    return u;
  }
}

// The green URL line, 2008 style: host + path, no scheme, cut at ~70 chars.
export function displayURL(u: string) {
  let s = u.replace(/^https?:\/\//, "");
  try {
    const p = new URL(u);
    s = p.hostname + decodeURIComponent(p.pathname) + (p.search ? p.search : "");
  } catch {}
  return s.length > 72 ? s.slice(0, 69) + "..." : s;
}

export function fileType(u?: string) {
  const m = /\.(jpe?g|png|gif|webp|svg|bmp|tiff?|avif)(?:$|[?#])/i.exec(u || "");
  return m ? m[1].toLowerCase().replace("jpeg", "jpg") : "";
}

export function relTime(iso?: string) {
  if (!iso) return "";
  const t = new Date(iso);
  const mins = Math.round((Date.now() - t.getTime()) / 60000);
  if (isNaN(mins)) return "";
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins} minute${mins === 1 ? "" : "s"} ago`;
  const h = Math.round(mins / 60);
  if (h < 24) return `${h} hour${h === 1 ? "" : "s"} ago`;
  const d = Math.round(h / 24);
  if (d < 7) return `${d} day${d === 1 ? "" : "s"} ago`;
  return t.toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" });
}

const escapeRe = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

// Bold the query terms, like classic result titles and snippets.
export function Highlight({ text, q }: { text?: string; q?: string }) {
  if (!text) return null;
  const terms = (q || "")
    .toLowerCase()
    .split(/\s+/)
    .filter((t) => t.length > 1 && !t.startsWith("!"));
  if (!terms.length) return text;
  const re = new RegExp(`(${terms.map(escapeRe).join("|")})`, "gi");
  return text.split(re).map((part, i) => (i % 2 ? <b key={i}>{part}</b> : part));
}

export const catLabel: Record<string, string | undefined> = { general: "Web", images: "Images", videos: "Video", news: "News", code: "Code", forums: "Forums", science: "Science" };

export function searchURL(q: string, cat = "general", page = 1) {
  const p = new URLSearchParams({ q });
  if (cat && cat !== "general") p.set("c", cat);
  if (page > 1) p.set("p", String(page));
  return "/search?" + p;
}

export const store = {
  get: (k: string, d: string) => localStorage.getItem("trawl." + k) ?? d,
  set: (k: string, v: string) => localStorage.setItem("trawl." + k, v),
};
