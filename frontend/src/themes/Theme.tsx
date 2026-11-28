import { useEffect } from "react";
import { applyTheme, themes, themeSetting } from ".";

// Keeps the chosen theme's stylesheet loaded and renders its extras.
export default function Theme() {
  const id = themeSetting.use();
  useEffect(() => void applyTheme(id), [id]);
  const Extras = themes[id].Extras;
  return Extras ? <Extras /> : null;
}
