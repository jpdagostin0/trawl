// Themes. Each one is a folder with a theme.css, loaded after Bootstrap so it can
// override Bootstrap's variables and components. A theme can also have settings
// (shown in the settings drawer), extras rendered while it's active (e.g. a
// wallpaper), and a prepare step run before the first paint.
import type { ComponentType } from "react";
import classic from "./classic/theme.css?url";
import simpleModern from "./simple-modern/theme.css?url";
import SimpleModernExtras from "./simple-modern/Extras";
import SimpleModernSettings from "./simple-modern/Settings";
import { prepareSimpleModern } from "./simple-modern/store";
import { localSetting } from "./local-setting";

interface Theme {
  name: string;
  href: string;
  Settings?: ComponentType;
  Extras?: ComponentType;
  prepare?: () => void;
}

const THEMES = {
  classic: { name: "Classic (2008)", href: classic },
  "simple-modern": {
    name: "Simple modern",
    href: simpleModern,
    Settings: SimpleModernSettings,
    Extras: SimpleModernExtras,
    prepare: prepareSimpleModern,
  },
} satisfies Record<string, Theme>;

export type ThemeId = keyof typeof THEMES;

export const themes: Record<ThemeId, Theme> = THEMES;

const isTheme = (s: unknown): s is ThemeId => typeof s === "string" && Object.hasOwn(THEMES, s);

export const themeSetting = localSetting<ThemeId>("trawl.theme", (s) => (isTheme(s) ? s : "simple-modern"));

// Swaps in a theme's stylesheet, removing the old one once the new one has loaded.
export function applyTheme(id: ThemeId): Promise<void> {
  const root = document.documentElement;
  root.dataset.theme = id;
  const old = [...document.querySelectorAll<HTMLLinkElement>("link[data-theme]")];
  if (old.some((l) => l.dataset.theme === id)) return Promise.resolve();
  const link = document.createElement("link");
  link.rel = "stylesheet";
  link.dataset.theme = id;
  return new Promise((resolve) => {
    link.onload = link.onerror = () => {
      if (root.dataset.theme === id) old.forEach((l) => l.remove());
      else link.remove();
      resolve();
    };
    link.href = themes[id].href;
    document.head.append(link);
  });
}

// Before the first render: the saved theme, fully loaded.
export function startTheme(): Promise<void> {
  const id = themeSetting.get();
  themes[id].prepare?.();
  return applyTheme(id);
}
