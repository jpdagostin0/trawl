// trawl: progressive enhancement only — every feature works without this file.
(() => {
  const $ = (s, el = document) => el.querySelector(s);
  const $$ = (s, el = document) => [...el.querySelectorAll(s)];
  const store = { get: (k, d) => localStorage.getItem("trawl." + k) ?? d, set: (k, v) => localStorage.setItem("trawl." + k, v) };

  // theme (persisted)
  const themes = ["auto", "light", "dark", "sepia"];
  const applyTheme = (t) => (t === "auto" ? delete document.documentElement.dataset.theme : (document.documentElement.dataset.theme = t));
  applyTheme(store.get("theme", "auto"));

  const typing = (e) => /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName) || e.target.isContentEditable;

  document.addEventListener("DOMContentLoaded", () => {
    const q = $("#q");

    // ---- autocomplete ----
    const box = $("#suggest");
    if (q && box) {
      let timer, items = [], idx = -1, ctrl;
      const close = () => { box.hidden = true; idx = -1; };
      const paint = () => {
        box.replaceChildren(...items.map((s, i) => {
          const li = document.createElement("li");
          li.textContent = s;
          if (i === idx) li.className = "on";
          li.onmousedown = (e) => { e.preventDefault(); q.value = s; q.form.submit(); };
          return li;
        }));
        box.hidden = items.length === 0;
      };
      q.addEventListener("input", () => {
        clearTimeout(timer);
        const v = q.value.trim();
        if (!v || v.startsWith("!")) { items = []; return close(); }
        timer = setTimeout(async () => {
          ctrl?.abort(); ctrl = new AbortController();
          try {
            const r = await fetch("/suggest?q=" + encodeURIComponent(v), { signal: ctrl.signal });
            const j = await r.json();
            items = (j[1] || []).slice(0, 8); idx = -1; paint();
          } catch {}
        }, 120);
      });
      q.addEventListener("keydown", (e) => {
        if (box.hidden) return;
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
          e.preventDefault();
          // cycle through -1 (nothing selected) .. items.length-1
          const n = items.length + 1;
          idx = ((idx + 1 + (e.key === "ArrowDown" ? 1 : -1)) % n + n) % n - 1;
          if (idx >= 0) q.value = items[idx];
          paint();
        } else if (e.key === "Escape") close();
      });
      q.addEventListener("blur", () => setTimeout(close, 100));
    }

    // ---- keyboard navigation on results ----
    const results = $$(".result");
    let sel = -1;
    const select = (i) => {
      if (!results.length) return;
      sel = Math.max(0, Math.min(results.length - 1, i));
      results.forEach((r, j) => r.classList.toggle("sel", j === sel));
      results[sel].scrollIntoView({ block: "nearest" });
      $("h2 a", results[sel]).focus({ preventScroll: true });
    };
    document.addEventListener("keydown", (e) => {
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      if (e.key === "Escape" && typing(e)) { e.target.blur(); return; }
      if (typing(e)) return;
      const cur = results[sel];
      switch (e.key) {
        case "/": if (q) { e.preventDefault(); q.focus(); q.select(); } break;
        case "j": case "ArrowDown": if (results.length) { e.preventDefault(); select(sel + 1); } break;
        case "k": case "ArrowUp": if (results.length) { e.preventDefault(); select(sel - 1); } break;
        case "o": case "Enter": if (cur) { e.preventDefault(); location.href = $("h2 a", cur).href; } break;
        case "O": if (cur) window.open($("h2 a", cur).href, "_blank", "noopener"); break;
        case "r": if (cur) location.href = $(".act-read", cur).href; break;
        case "n": case "l": $("#next")?.click(); break;
        case "p": case "h": $("#prev")?.click(); break;
      }
    });

    // ---- reader tools ----
    const root = document.documentElement;
    let size = +store.get("size", 20);
    const applySize = () => root.style.setProperty("--reader-size", size + "px");
    applySize();
    $("#font-inc")?.addEventListener("click", () => { size = Math.min(28, size + 1); store.set("size", size); applySize(); });
    $("#font-dec")?.addEventListener("click", () => { size = Math.max(14, size - 1); store.set("size", size); applySize(); });
    $("#theme")?.addEventListener("click", () => {
      const t = themes[(themes.indexOf(store.get("theme", "auto")) + 1) % themes.length];
      store.set("theme", t); applyTheme(t); toast("Theme: " + t);
    });
    $("#copy-md")?.addEventListener("click", async (e) => {
      const md = await (await fetch(e.target.dataset.src)).text();
      await navigator.clipboard.writeText(md);
      toast("Markdown copied");
    });
    $("#back")?.addEventListener("click", (e) => { if (history.length > 1) { e.preventDefault(); history.back(); } });
  });

  function toast(msg) {
    const t = document.createElement("div");
    t.className = "toast"; t.textContent = msg;
    document.body.append(t);
    setTimeout(() => t.remove(), 1600);
  }
})();
