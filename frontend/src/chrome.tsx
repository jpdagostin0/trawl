// Page chrome: logo, the black-and-blue link bar, search form with suggest, footer.
import { useEffect, useRef, useState, type FormEvent, type ReactNode, type RefObject } from "react";
import { Link, useNavigate } from "react-router";
import { Button, Modal, Table } from "react-bootstrap";
import NavDropdown from "react-bootstrap/NavDropdown";
import { catLabel, searchURL, useConfig } from "./util";

// t r a w l in blue, red, yellow, blue, green, the way G o o g l e was colored.
const LETTERS = [
  ["t", "lb"],
  ["r", "lr"],
  ["a", "ly"],
  ["w", "lb"],
  ["l", "lg"],
];

export function Logo({ size = "large", sub }: { size?: "large" | "small" | "tiny"; sub?: string }) {
  return (
    <Link to={sub ? `/?c=${sub}` : "/"} className={`logo logo-${size}`} title="trawl Home" aria-label="trawl">
      <span className="logo-word">
        {LETTERS.map(([ch, cls]) => (
          <span key={ch} className={cls}>
            {ch}
          </span>
        ))}
      </span>
      {sub && sub !== "general" && <span className="logo-sub">{catLabel[sub] ?? sub}</span>}
    </Link>
  );
}

const PRIMARY = ["general", "images", "news", "videos"];

// The top-left "Web Images News ... more ▼" bar and the top-right links.
export function GBar({ cat = "general", q = "", onTips }: { cat?: string; q?: string; onTips: () => void }) {
  const { categories } = useConfig();
  const ids = categories.map((c) => c.id);
  const primary = PRIMARY.filter((c) => ids.includes(c));
  const more = ids.filter((c) => !PRIMARY.includes(c));
  const href = (c: string) => (q ? searchURL(q, c) : c === "general" ? "/" : `/?c=${c}`);
  return (
    <div className="gbar">
      <nav className="gbar-left">
        {primary.map((c) =>
          c === cat ? (
            <b key={c}>{catLabel[c]}</b>
          ) : (
            <Link key={c} to={href(c)}>
              {catLabel[c]}
            </Link>
          ),
        )}
        <NavDropdown title={<u>more</u>} id="gbar-more" className="gbar-more">
          {more.map((c) => (
            <NavDropdown.Item key={c} as={Link} to={href(c)} active={c === cat}>
              {catLabel[c] ?? c}
            </NavDropdown.Item>
          ))}
          <NavDropdown.Divider />
          <NavDropdown.Item onClick={onTips}>Bangs &amp; shortcuts</NavDropdown.Item>
          <NavDropdown.Item href="/api/search?q=hello">JSON API</NavDropdown.Item>
        </NavDropdown>
      </nav>
      <nav className="gbar-right">
        {cat === "archive" ? <b>Archive</b> : <Link to="/archive">Archive</Link>} |{" "}
        <a href="/opensearch.xml">Add to browser</a> | <a href="/api/search?q=hello">JSON API</a>
      </nav>
    </div>
  );
}

// Google Suggest, 2008: a black-bordered list under the box, blue selected row.
interface SuggestProps {
  value: string;
  onPick: (s: string, go: boolean) => void;
  inputRef: RefObject<HTMLInputElement | null>;
}

function Suggest({ value, onPick, inputRef }: SuggestProps) {
  const [items, setItems] = useState<string[]>([]);
  const [idx, setIdx] = useState(-1);
  const [open, setOpen] = useState(false);
  const typed = useRef(value); // the box's initial value is not "typing"

  useEffect(() => {
    const v = value.trim();
    if (v === typed.current.trim()) return;
    typed.current = v;
    if (!v || v.startsWith("!")) return setItems([]);
    const ctrl = new AbortController();
    const t = setTimeout(async () => {
      if (document.activeElement !== inputRef.current) return;
      try {
        const j: [string, string[]?] = await (await fetch("/suggest?q=" + encodeURIComponent(v), { signal: ctrl.signal })).json();
        if (document.activeElement !== inputRef.current) return;
        setItems((j[1] || []).slice(0, 10));
        setIdx(-1);
        setOpen(true);
      } catch {}
    }, 120);
    return () => {
      clearTimeout(t);
      ctrl.abort();
    };
  }, [value, inputRef]);

  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    const key = (e: KeyboardEvent) => {
      if (!open || !items.length) return;
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        const n = items.length + 1;
        const next = ((idx + 1 + (e.key === "ArrowDown" ? 1 : -1)) % n + n) % n - 1;
        setIdx(next);
        if (next >= 0) {
          typed.current = items[next];
          onPick(items[next], false);
        }
      } else if (e.key === "Escape") setOpen(false);
    };
    const blur = () => setTimeout(() => setOpen(false), 150);
    const focus = () => items.length > 0 && setOpen(true);
    el.addEventListener("keydown", key);
    el.addEventListener("blur", blur);
    el.addEventListener("focus", focus);
    return () => {
      el.removeEventListener("keydown", key);
      el.removeEventListener("blur", blur);
      el.removeEventListener("focus", focus);
    };
  }, [open, items, idx, inputRef, onPick]);

  if (!open || !items.length) return null;
  return (
    <div className="suggest" role="listbox">
      {items.map((s, i) => (
        <div
          key={s}
          role="option"
          aria-selected={i === idx}
          className={i === idx ? "on" : ""}
          onMouseEnter={() => setIdx(i)}
          onMouseDown={(e) => (e.preventDefault(), onPick(s, true))}
        >
          {s}
        </div>
      ))}
      <div className="suggest-close">
        <a href="#close" onMouseDown={(e) => (e.preventDefault(), setOpen(false))}>
          close
        </a>
      </div>
    </div>
  );
}

const buttonLabel: Record<string, string | undefined> = { general: "trawl Search", images: "Search Images", news: "Search News", videos: "Search Video" };

// variant: "home" (big centered box + two buttons), "top" (results header), "bottom" (results footer box)
interface SearchFormProps {
  q?: string;
  cat?: string;
  variant?: "home" | "top" | "bottom";
  onTips?: () => void;
}

export function SearchForm({ q = "", cat = "general", variant = "top", onTips }: SearchFormProps) {
  const [value, setValue] = useState(q);
  const nav = useNavigate();
  const input = useRef<HTMLInputElement>(null);
  const form = useRef<HTMLFormElement>(null);
  useEffect(() => setValue(q), [q]);

  // "/" focuses the box from anywhere (only the top/home form listens).
  useEffect(() => {
    if (variant === "bottom") return;
    const k = (e: KeyboardEvent) => {
      if (e.key !== "/" || /^(INPUT|TEXTAREA|SELECT)$/.test((e.target as HTMLElement).tagName)) return;
      e.preventDefault();
      input.current?.focus();
      input.current?.select();
    };
    document.addEventListener("keydown", k);
    return () => document.removeEventListener("keydown", k);
  }, [variant]);

  const submit = (e: FormEvent<HTMLFormElement>) => {
    const v = value.trim();
    const lucky = ((e.nativeEvent as SubmitEvent).submitter as HTMLButtonElement | null)?.name === "btnI";
    // Bangs and I'm Feeling Lucky are server redirects: let the browser submit normally.
    if (lucky || /(^|\s)!\S/.test(v)) return;
    e.preventDefault();
    if (!v) return;
    input.current?.blur();
    nav(searchURL(v, cat));
  };
  const pick = (s: string, go: boolean) => {
    setValue(s);
    if (go) nav(searchURL(s, cat));
  };

  const home = variant === "home";
  return (
    <form ref={form} action="/search" method="get" role="search" autoComplete="off" onSubmit={submit} className={`sf sf-${variant}`}>
      {cat !== "general" && <input type="hidden" name="c" value={cat} />}
      <div className="sf-row">
        <div className="sf-box">
          <input
            ref={input}
            name="q"
            className="sf-q"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            maxLength={2048}
            title="Search"
            aria-label="Search"
            autoFocus={home || !q}
            spellCheck={false}
          />
          {variant !== "bottom" && <Suggest value={value} onPick={pick} inputRef={input} />}
        </div>
        {!home && (
          <Button type="submit" name="btnG" className="btn-classic">
            {variant === "bottom" ? "Search" : buttonLabel[cat] ?? "Search"}
          </Button>
        )}
        {variant !== "bottom" && (
          <span className="sf-links">
            <a href="#tips" onClick={(e) => (e.preventDefault(), onTips?.())}>
              Search Tips
            </a>
            <br />
            <a href="#tips" onClick={(e) => (e.preventDefault(), onTips?.())}>
              Bangs
            </a>
          </span>
        )}
      </div>
      {home && (
        <div className="sf-buttons">
          <Button type="submit" name="btnG" className="btn-classic">
            {buttonLabel[cat] ?? "trawl Search"}
          </Button>
          {cat === "general" && (
            <Button type="submit" name="btnI" className="btn-classic">
              I&apos;m Feeling Lucky
            </Button>
          )}
        </div>
      )}
    </form>
  );
}

export function Footer() {
  return (
    <footer className="foot">
      <div>
        <Link to="/">trawl Home</Link> - <a href="/api/search?q=hello">JSON API</a> - <a href="/opensearch.xml">Add to browser</a> -{" "}
        <a href="https://github.com/jpdagostin0/trawl" rel="noreferrer">About trawl</a>
      </div>
      <div className="copy">&copy;{new Date().getFullYear()} trawl - No ads. No tracking. No logs.</div>
    </footer>
  );
}

const KEYS = [
  ["/", "focus the search box"],
  ["j / k", "next / previous result"],
  ["o or Enter", "open the selected result"],
  ["O", "open it in a new tab"],
  ["r", "open it in Reader view"],
  ["n / p", "next / previous page"],
];

export function TipsModal({ show, onHide }: { show: boolean; onHide: () => void }) {
  const { bangs, tab_bangs } = useConfig();
  return (
    <Modal show={show} onHide={onHide} size="lg" scrollable className="classic-modal">
      <Modal.Header closeButton>
        <Modal.Title>Search Tips</Modal.Title>
      </Modal.Header>
      <Modal.Body>
        <p>
          Paste a URL into the box to open it in <b>Reader view</b>. Put a <b>!bang</b> anywhere in your query to search
          another site directly, e.g. <code>!w alan turing</code>.
        </p>
        <h6>Tab bangs</h6>
        <p>
          {Object.entries(tab_bangs)
            .sort()
            .map(([b, c]) => (
              <span key={b} className="bang">
                <code>!{b}</code> {catLabel[c] ?? c}
              </span>
            ))}
        </p>
        <h6>Site bangs</h6>
        <Table size="sm" className="bangs">
          <tbody>
            {Object.entries(bangs)
              .sort()
              .map(([b, site]) => (
                <tr key={b}>
                  <td>
                    <code>!{b}</code>
                  </td>
                  <td>{site}</td>
                </tr>
              ))}
          </tbody>
        </Table>
        <h6>Keyboard shortcuts</h6>
        <Table size="sm" className="bangs">
          <tbody>
            {KEYS.map(([k, d]) => (
              <tr key={k}>
                <td>
                  <kbd>{k}</kbd>
                </td>
                <td>{d}</td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Modal.Body>
    </Modal>
  );
}

// "Trawwwwwl" pager: one w per page, the current one red, with numbers underneath.
export function Pager({ page, hasNext, href }: { page: number; hasNext: boolean; href: (page: number) => string }) {
  const start = Math.max(1, page - 5);
  const end = hasNext ? Math.max(start + 9, page + 4) : page;
  const pages = [];
  for (let i = start; i <= end; i++) pages.push(i);
  return (
    <nav className="pager" aria-label="Pages">
      <table>
        <tbody>
          <tr className="pager-letters">
            <td className="pl-first">
              {page > 1 ? (
                <Link to={href(page - 1)} tabIndex={-1}>
                  <span className="lb">T</span>
                  <span className="lr">r</span>
                  <span className="ly">a</span>
                </Link>
              ) : (
                <>
                  <span className="lb">T</span>
                  <span className="lr">r</span>
                  <span className="ly">a</span>
                </>
              )}
            </td>
            {pages.map((i) => (
              <td key={i}>
                {i === page ? (
                  <span className="lr">w</span>
                ) : (
                  <Link to={href(i)} tabIndex={-1}>
                    <span className="ly">w</span>
                  </Link>
                )}
              </td>
            ))}
            <td className="pl-last">
              {hasNext ? (
                <Link to={href(page + 1)} tabIndex={-1}>
                  <span className="lg">l</span>
                </Link>
              ) : (
                <span className="lg">l</span>
              )}
            </td>
          </tr>
          <tr className="pager-nums">
            <td className="pl-first">
              {page > 1 && (
                <Link to={href(page - 1)} id="prev">
                  <span className="arrow">&lsaquo;</span> Previous
                </Link>
              )}
            </td>
            {pages.map((i) => (
              <td key={i}>{i === page ? <span className="cur">{i}</span> : <Link to={href(i)}>{i}</Link>}</td>
            ))}
            <td className="pl-last">
              {hasNext && (
                <Link to={href(page + 1)} id="next">
                  Next <span className="arrow">&rsaquo;</span>
                </Link>
              )}
            </td>
          </tr>
        </tbody>
      </table>
    </nav>
  );
}

// Light blue bar under the header: "Web    Results 1 - 10 for q. (0.23 seconds)"
export function BlueBar({ label, children }: { label: string; children?: ReactNode }) {
  return (
    <div className="ssb">
      <div className="ssb-label">{label}</div>
      <div className="ssb-info">{children}</div>
    </div>
  );
}

// Yellow notice, like the old "Loading..." and message bars.
export function Notice({ children }: { children?: ReactNode }) {
  return <span className="notice-y">{children}</span>;
}
