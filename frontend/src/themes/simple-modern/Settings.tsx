import { useId, useState, type ChangeEvent } from "react";
import { hasWallpaper, simpleModern, validImage, type Scheme, type SimpleModern } from "./store";

const DEFAULT_ACCENT = "#0061e0";

// A picked file is kept in localStorage as a data URL, so it's scaled down to a
// screen's size first (storage is a few megabytes per site).
async function fileToDataURL(file: File): Promise<string> {
  const bitmap = await createImageBitmap(file);
  const scale = Math.min(1, 2560 / Math.max(bitmap.width, bitmap.height));
  const canvas = document.createElement("canvas");
  canvas.width = Math.round(bitmap.width * scale);
  canvas.height = Math.round(bitmap.height * scale);
  canvas.getContext("2d")!.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
  bitmap.close();
  return canvas.toDataURL("image/jpeg", 0.85);
}

export default function SimpleModernSettings() {
  const s = simpleModern.use();
  const [address, setAddress] = useState(s.image.startsWith("data:") ? "" : s.image);
  const [error, setError] = useState("");
  const id = useId();
  const save = (patch: Partial<SimpleModern>) => {
    try {
      simpleModern.set({ ...s, ...patch });
      setError("");
    } catch {
      setError("This browser's storage for trawl is full; try a smaller image.");
    }
  };
  const applyAddress = () => {
    const a = address.trim();
    if (!validImage(a)) return setError("Use an http(s) image address.");
    save({ wallpaper: "image", image: a });
  };
  const pick = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    try {
      save({ wallpaper: "image", image: await fileToDataURL(file) });
      setAddress("");
    } catch {
      setError("That file couldn't be read as an image.");
    }
  };

  return (
    <>
      <section className="settings-card">
        <h2>Colors</h2>
        <div className="setting">
          <label className="setting-text" htmlFor={id + "scheme"}>
            <span className="setting-label">Appearance</span>
            <span className="setting-desc">Automatic follows your system.</span>
          </label>
          <select id={id + "scheme"} value={s.scheme} onChange={(e) => save({ scheme: e.target.value as Scheme })}>
            <option value="auto">Automatic</option>
            <option value="light">Light</option>
            <option value="dark">Dark</option>
          </select>
        </div>
        <div className="setting">
          <label className="setting-text" htmlFor={id + "accent"}>
            <span className="setting-label">Accent</span>
            <span className="setting-desc">Links, focus and the selected result.</span>
          </label>
          <div className="actions">
            {s.accent && (
              <button type="button" onClick={() => save({ accent: "" })}>
                Reset
              </button>
            )}
            <input id={id + "accent"} type="color" value={s.accent || DEFAULT_ACCENT} onChange={(e) => save({ accent: e.target.value })} />
          </div>
        </div>
      </section>

      <section className="settings-card">
        <h2>Wallpaper</h2>
        <p className="hint">An image behind the page, saved in this browser.</p>
        <div className="setting">
          <label className="setting-text" htmlFor={id + "wallpaper"}>
            <span className="setting-label">Background</span>
          </label>
          <select id={id + "wallpaper"} value={s.wallpaper} onChange={(e) => save({ wallpaper: e.target.value === "image" ? "image" : "none" })}>
            <option value="none">Theme color</option>
            <option value="image">Image</option>
          </select>
        </div>
        {s.wallpaper === "image" && (
          <>
            <label className="field">
              <span>Image address</span>
              <input
                type="text"
                value={address}
                placeholder={s.image.startsWith("data:") ? "Using a picked file" : "https://…"}
                spellCheck={false}
                onChange={(e) => setAddress(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && applyAddress()}
              />
            </label>
            <div className="actions">
              <button type="button" className="primary" onClick={applyAddress}>
                Use address
              </button>
              <label className="file-button">
                Choose a file…
                <input type="file" accept="image/*" onChange={pick} hidden />
              </label>
            </div>
            {hasWallpaper(s) && (
              <>
                <label className="field range">
                  <span>Dim</span>
                  <input type="range" min={0} max={0.8} step={0.05} value={s.dim} onChange={(e) => save({ dim: Number(e.target.value) })} />
                </label>
                <label className="field range">
                  <span>Blur</span>
                  <input type="range" min={0} max={40} step={1} value={s.blur} onChange={(e) => save({ blur: Number(e.target.value) })} />
                </label>
              </>
            )}
          </>
        )}
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
      </section>
    </>
  );
}
