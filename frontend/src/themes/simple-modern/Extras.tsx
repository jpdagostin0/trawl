import { useLayoutEffect } from "react";
import { proxied } from "../../util";
import { applyColors, clearColors, darkQuery, hasWallpaper, simpleModern } from "./store";

// While simple modern is the theme: its colors, following the system's light/dark
// when set to automatic, and the wallpaper behind the page.
export default function SimpleModernExtras() {
  const s = simpleModern.use();
  useLayoutEffect(() => {
    const apply = () => applyColors(s);
    apply();
    darkQuery.addEventListener("change", apply);
    return () => {
      darkQuery.removeEventListener("change", apply);
      clearColors();
    };
  }, [s]);
  if (!hasWallpaper(s)) return null;
  // Web images come through trawl's image proxy (the CSP only allows our own images).
  const src = s.image.startsWith("data:") ? s.image : proxied(s.image);
  return (
    <>
      <div id="wallpaper" style={{ backgroundImage: `url(${JSON.stringify(src)})` }} />
      <div id="wallpaper-dim" style={{ backgroundColor: `rgb(0 0 0 / ${s.dim})`, backdropFilter: s.blur ? `blur(${s.blur}px)` : undefined }} />
    </>
  );
}
