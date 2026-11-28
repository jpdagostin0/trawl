import { useSyncExternalStore } from "react";

// A JSON value kept in localStorage, shared by every component that uses it and
// kept in step with other tabs. parse cleans up whatever was stored (or null).
export function localSetting<T>(key: string, parse: (stored: unknown) => T) {
  const read = () => {
    try {
      return parse(JSON.parse(localStorage.getItem(key) ?? "null"));
    } catch {
      return parse(null);
    }
  };
  let value = read();
  const listeners = new Set<() => void>();
  const notify = () => listeners.forEach((l) => l());
  addEventListener("storage", (e) => {
    if (e.key !== key && e.key !== null) return;
    value = read();
    notify();
  });
  const get = () => value;
  const subscribe = (l: () => void) => {
    listeners.add(l);
    return () => void listeners.delete(l);
  };
  return {
    get,
    // Throws if the browser's storage is full.
    set(next: T) {
      const clean = parse(next);
      localStorage.setItem(key, JSON.stringify(clean));
      value = clean;
      notify();
    },
    use: () => useSyncExternalStore(subscribe, get),
  };
}
