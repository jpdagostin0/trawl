import { useEffect, useId, useRef, useSyncExternalStore, type MouseEvent } from "react";
import { themes, themeSetting, type ThemeId } from "./themes";

// Whether the drawer is open, shared with the Settings link in the top bar.
let isOpen = false;
const listeners = new Set<() => void>();
const setOpen = (open: boolean) => {
  isOpen = open;
  listeners.forEach((l) => l());
};
const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => void listeners.delete(l);
};

// The top bar's "Settings" link, next to Archive.
export function SettingsLink() {
  const open = useSyncExternalStore(subscribe, () => isOpen);
  const click = (e: MouseEvent) => {
    e.preventDefault();
    setOpen(!isOpen);
  };
  return (
    <a href="#settings" className="settings-link" data-settings-link aria-expanded={open} onClick={click}>
      Settings
    </a>
  );
}

// The settings drawer: the theme and its own settings, all kept in this
// browser's localStorage.
export default function Settings() {
  const open = useSyncExternalStore(subscribe, () => isOpen);
  const theme = themeSetting.use();
  const drawer = useRef<HTMLElement>(null);
  const id = useId();

  useEffect(() => {
    if (!open) return;
    const opener = document.activeElement as HTMLElement | null;
    drawer.current?.querySelector<HTMLElement>(".close")?.focus({ preventScroll: true });
    const key = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    const down = (e: PointerEvent) => {
      const t = e.target as Element;
      if (!drawer.current?.contains(t) && !t.closest?.("[data-settings-link]")) setOpen(false);
    };
    addEventListener("keydown", key);
    addEventListener("pointerdown", down);
    return () => {
      removeEventListener("keydown", key);
      removeEventListener("pointerdown", down);
      if (drawer.current?.contains(document.activeElement)) opener?.focus({ preventScroll: true });
    };
  }, [open]);

  const ThemeSettings = themes[theme].Settings;
  return (
    <aside ref={drawer} id={id} className={`drawer settings-drawer${open ? " open" : ""}`} aria-label="Settings" inert={!open}>
      <div className="drawer-head">
        <h2>Settings</h2>
        <button className="close" type="button" title="Close" onClick={() => setOpen(false)}>
          ×
        </button>
      </div>
      <div className="drawer-body">
        <section className="settings-card">
          <h2>Theme</h2>
          <div className="setting">
            <label className="setting-text" htmlFor={id + "theme"}>
              <span className="setting-label">Look</span>
              <span className="setting-desc">Saved in this browser.</span>
            </label>
            <select id={id + "theme"} value={theme} onChange={(e) => themeSetting.set(e.target.value as ThemeId)}>
              {Object.entries(themes).map(([t, { name }]) => (
                <option key={t} value={t}>
                  {name}
                </option>
              ))}
            </select>
          </div>
        </section>
        {ThemeSettings && <ThemeSettings />}
      </div>
    </aside>
  );
}
