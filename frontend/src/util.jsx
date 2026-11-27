// Small helpers shared by the pages.
import { useEffect, useState } from "react";

export async function getJSON(url, signal) {
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

// Fetch JSON for `url` (null = skip). Aborts when url changes.
export function useJSON(url) {
  const [state, setState] = useState({ data: null, error: null, loading: !!url });
  useEffect(() => {
    if (!url) return;
    const ctrl = new AbortController();
    setState((s) => ({ ...s, loading: true, error: null }));
    getJSON(url, ctrl.signal)
      .then((data) => setState({ data, error: null, loading: false }))
      .catch((error) => error.name !== "AbortError" && setState({ data: null, error, loading: false }));
    return () => ctrl.abort();
  }, [url]);
  return state;
}

// Like useJSON, for newline-delimited JSON: data is replaced by each line as it arrives.
export function useNDJSON(url) {
  const [state, setState] = useState({ data: null, error: null, loading: !!url });
  useEffect(() => {
    if (!url) return;
    const ctrl = new AbortController();
    setState((s) => ({ ...s, loading: true, error: null }));
    (async () => {
      const r = await fetch(url, { signal: ctrl.signal });
      if (!r.ok) {
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
        buf = lines.pop();
        const last = lines.filter((l) => l.trim()).pop();
        if (last) setState({ data: JSON.parse(last), error: null, loading: true });
      }
      setState((s) => ({ ...s, loading: false }));
    })().catch((error) => error.name !== "AbortError" && setState({ data: null, error, loading: false }));
    return () => ctrl.abort();
  }, [url]);
  return state;
}

let configPromise;
const fallbackConfig = { categories: [{ id: "general", engines: [] }], bangs: {}, tab_bangs: {} };
export function useConfig() {
  const [cfg, setCfg] = useState(fallbackConfig);
  useEffect(() => {
    configPromise ??= getJSON("/api/config").catch(() => fallbackConfig);
    configPromise.then(setCfg);
  }, []);
  return cfg;
}

// All third-party images go through trawl's proxy.
export const proxied = (u) => (u ? "/img?u=" + encodeURIComponent(u) : "");

export function host(u) {
  try {
    return new URL(u).hostname.replace(/^www\./, "");
  } catch {
    return u;
  }
}

// The green URL line, 2008 style: host + path, no scheme, cut at ~70 chars.
export function displayURL(u) {
  let s = u.replace(/^https?:\/\//, "");
  try {
    const p = new URL(u);
    s = p.hostname + decodeURIComponent(p.pathname) + (p.search ? p.search : "");
  } catch {}
  return s.length > 72 ? s.slice(0, 69) + "..." : s;
}

export function fileType(u) {
  const m = /\.(jpe?g|png|gif|webp|svg|bmp|tiff?|avif)(?:$|[?#])/i.exec(u || "");
  return m ? m[1].toLowerCase().replace("jpeg", "jpg") : "";
}

export function relTime(iso) {
  if (!iso) return "";
  const t = new Date(iso);
  const mins = Math.round((Date.now() - t) / 60000);
  if (isNaN(mins)) return "";
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins} minute${mins === 1 ? "" : "s"} ago`;
  const h = Math.round(mins / 60);
  if (h < 24) return `${h} hour${h === 1 ? "" : "s"} ago`;
  const d = Math.round(h / 24);
  if (d < 7) return `${d} day${d === 1 ? "" : "s"} ago`;
  return t.toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" });
}

const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

// Bold the query terms, like classic result titles and snippets.
export function Highlight({ text, q }) {
  if (!text) return null;
  const terms = (q || "")
    .toLowerCase()
    .split(/\s+/)
    .filter((t) => t.length > 1 && !t.startsWith("!"));
  if (!terms.length) return text;
  const re = new RegExp(`(${terms.map(escapeRe).join("|")})`, "gi");
  return text.split(re).map((part, i) => (i % 2 ? <b key={i}>{part}</b> : part));
}

export const catLabel = { general: "Web", images: "Images", videos: "Video", news: "News", code: "Code", forums: "Forums", science: "Science" };

export function searchURL(q, cat = "general", page = 1) {
  const p = new URLSearchParams({ q });
  if (cat && cat !== "general") p.set("c", cat);
  if (page > 1) p.set("p", page);
  return "/search?" + p;
}

export const store = {
  get: (k, d) => localStorage.getItem("trawl." + k) ?? d,
  set: (k, v) => localStorage.setItem("trawl." + k, v),
};
