// Simple modern's settings, kept in localStorage: light/dark, the accent color
// and a wallpaper (an image address or a picked file kept as a data URL), with
// dim and blur so the page stays readable over it.
import { localSetting } from "../local-setting";

export type Scheme = "auto" | "light" | "dark";
export type WallpaperKind = "none" | "image";

export interface SimpleModern {
  scheme: Scheme;
  accent: string; // "#rrggbb", or "" for the theme's own
  wallpaper: WallpaperKind;
  image: string;
  dim: number; // 0 - 0.8
  blur: number; // CSS pixels, 0 - 40
}

export const validImage = (s: string) => /^data:image\//.test(s) || /^https?:\/\//i.test(s);

const clamp = (n: unknown, lo: number, hi: number) => (typeof n === "number" && Number.isFinite(n) ? Math.min(hi, Math.max(lo, n)) : lo);

export const simpleModern = localSetting<SimpleModern>("trawl.theme.simple-modern", (stored) => {
  const s = (stored ?? {}) as Partial<SimpleModern>;
  return {
    scheme: s.scheme === "light" || s.scheme === "dark" ? s.scheme : "auto",
    accent: typeof s.accent === "string" && /^#[0-9a-f]{6}$/i.test(s.accent) ? s.accent : "",
    wallpaper: s.wallpaper === "image" ? "image" : "none",
    image: typeof s.image === "string" && validImage(s.image) ? s.image : "",
    dim: clamp(s.dim, 0, 0.8),
    blur: clamp(s.blur, 0, 40),
  };
});

export const darkQuery = matchMedia("(prefers-color-scheme: dark)");

export const hasWallpaper = (s: SimpleModern) => s.wallpaper === "image" && !!s.image;

// The colors go on <html>: Bootstrap's data-bs-theme (the theme's dark colors key
// off it too) and --accent.
export function applyColors(s: SimpleModern) {
  const root = document.documentElement;
  root.dataset.bsTheme = s.scheme === "auto" ? (darkQuery.matches ? "dark" : "light") : s.scheme;
  if (s.accent) root.style.setProperty("--accent", s.accent);
  else root.style.removeProperty("--accent");
  root.classList.toggle("has-wallpaper", hasWallpaper(s));
}

export function clearColors() {
  const root = document.documentElement;
  delete root.dataset.bsTheme;
  root.style.removeProperty("--accent");
  root.classList.remove("has-wallpaper");
}

export const prepareSimpleModern = () => applyColors(simpleModern.get());
