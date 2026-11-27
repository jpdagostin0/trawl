import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { Col, Form, Modal, Row } from "react-bootstrap";
import QuickAnswer from "./QuickAnswer";
import { BlueBar, Footer, GBar, Logo, Notice, Pager, SearchForm, TipsModal } from "./chrome";
import { catLabel, displayURL, fileType, Highlight, host, proxied, relTime, searchURL, useConfig, useNDJSON } from "./util";
import type { EngineStatus, Result, SearchResponse } from "./types";

export default function Results() {
  const [params] = useSearchParams();
  const { categories } = useConfig();
  const q = (params.get("q") || "").trim();
  const c = params.get("c") || "general";
  const cat = categories.some((x) => x.id === c) || c === "general" ? c : "general";
  const page = Math.max(1, parseInt(params.get("p") ?? "", 10) || 1);
  const [tips, setTips] = useState(false);
  const nav = useNavigate();

  useEffect(() => {
    if (!q) nav("/", { replace: true });
  }, [q, nav]);

  // Streamed: the first engine's results show right away, the rest merge in as they answer.
  const api = q ? `/api/search?${new URLSearchParams({ q, category: cat, page: String(page), stream: "1" })}` : null;
  const { data, error } = useNDJSON<SearchResponse>(api);
  const r = data && data.query === q && data.category === cat && data.page === page ? data : null;
  document.title = `${q} - trawl ${cat === "general" ? "Search" : catLabel[cat] ?? "Search"}`;

  const results = r?.results ?? [];
  const pending = !error && (!r || !r.done);
  const failed = r?.done && !results.length && r.engines.length > 0 && r.engines.every((e) => e.error);
  const first = r ? (page - 1) * results.length + 1 : 0;

  const href = (p: number) => searchURL(q, cat, p);
  const sub = cat === "general" ? undefined : cat;

  return (
    <div className={`page-results cat-${cat}`}>
      <GBar cat={cat} q={q} onTips={() => setTips(true)} />
      <header className="rhead">
        <Logo size="small" sub={sub} />
        <SearchForm q={q} cat={cat} variant="top" onTips={() => setTips(true)} />
      </header>
      <BlueBar label={catLabel[cat] ?? cat}>
        {pending && !results.length ? (
          <Notice>Loading...</Notice>
        ) : r && results.length ? (
          <>
            Results <b>{first}</b> - <b>{first + results.length - 1}</b> for <b>{q}</b>. (<b>{(r.took_ms / 1000).toFixed(2)}</b> seconds)
            {r.cached && " (cached)"}
            {pending && (
              <>
                {" "}
                <Notice>Loading more...</Notice>
              </>
            )}
          </>
        ) : null}
      </BlueBar>

      <Row className="rbody g-0">
        <Col lg={cat === "images" ? 12 : 8} className="rmain">
          {r?.reader_url && (
            <p className="readcta">
              Open <b>{r.reader_url}</b> in <Link to={`/read?url=${encodeURIComponent(r.reader_url)}`}>Reader view</Link> &raquo;
            </p>
          )}
          {r && <QuickAnswer answer={r.answer} infobox={r.infobox} q={q} />}
          {error && <Sorry>trawl couldn&apos;t complete your search: {error.message}</Sorry>}
          {failed && <Sorry>every engine failed for this query. They may be rate-limiting this server; please try again in a minute.</Sorry>}
          {r?.done && !results.length && !failed && <NoMatch q={q} />}
          {r && results.length > 0 && cat === "images" && <ImageResults q={q} results={results} />}
          {r && results.length > 0 && cat === "news" && <NewsResults q={q} results={results} />}
          {r && results.length > 0 && cat !== "images" && cat !== "news" && <WebResults q={q} results={results} cat={cat} />}
        </Col>
      </Row>

      {r && results.length > 0 && <Pager page={page} hasNext={results.length >= 5} href={href} />}
      {r && <EngineLine engines={r.engines} pending={pending} />}
      <div className="bsf">
        <SearchForm q={q} cat={cat} variant="bottom" />
        <div className="bsf-links">
          <a href="#tips" onClick={(e) => (e.preventDefault(), setTips(true))}>
            Search Tips
          </a>{" "}
          | <Link to={`/?c=${cat}`}>{catLabel[cat] ?? "trawl"} Home</Link>
        </div>
      </div>
      <Footer />
      <TipsModal show={tips} onHide={() => setTips(false)} />
    </div>
  );
}

function Sorry({ children }: { children: ReactNode }) {
  return (
    <div className="sorry">
      <b>We&apos;re sorry...</b>
      <p>... but {children}</p>
    </div>
  );
}

function NoMatch({ q }: { q: string }) {
  return (
    <div className="nomatch">
      <p>
        Your search - <b>{q}</b> - did not match any documents.
      </p>
      <p>Suggestions:</p>
      <ul>
        <li>Make sure all words are spelled correctly.</li>
        <li>Try different keywords.</li>
        <li>Try more general keywords.</li>
      </ul>
    </div>
  );
}

// j/k/o/O/r keyboard navigation; the selected result gets the old blue ► marker.
type OpenKey = "o" | "Enter" | "O" | "r";

function useKeyNav(count: number, open: (i: number, key: OpenKey) => void) {
  const [sel, setSel] = useState(-1);
  useEffect(() => setSel(-1), [count]);
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey || /^(INPUT|TEXTAREA|SELECT)$/.test((e.target as HTMLElement).tagName)) return;
      if (document.querySelector(".modal.show")) return;
      switch (e.key) {
        case "j":
        case "ArrowDown":
          e.preventDefault();
          setSel((s) => Math.min(count - 1, s + 1));
          break;
        case "k":
        case "ArrowUp":
          e.preventDefault();
          setSel((s) => Math.max(0, s - 1));
          break;
        case "o":
        case "Enter":
        case "O":
        case "r":
          if (sel >= 0) {
            e.preventDefault();
            open(sel, e.key as OpenKey);
          }
          break;
        case "n":
          document.getElementById("next")?.click();
          break;
        case "p":
          document.getElementById("prev")?.click();
          break;
      }
    };
    document.addEventListener("keydown", k);
    return () => document.removeEventListener("keydown", k);
  }, [count, sel, open]);
  useEffect(() => {
    document.querySelector(`[data-i="${sel}"]`)?.scrollIntoView({ block: "nearest" });
  }, [sel]);
  return sel;
}

const readerHref = (u: string) => `/read?url=${encodeURIComponent(u)}`;

function useOpen(results: Result[]) {
  const nav = useNavigate();
  return useMemo(
    () => (i: number, key: OpenKey) => {
      const u = results[i]?.url;
      if (!u) return;
      if (key === "r") nav(readerHref(u));
      else if (key === "O") window.open(u, "_blank", "noopener,noreferrer");
      else location.href = u;
    },
    [results, nav],
  );
}

function WebResults({ q, results, cat }: { q: string; results: Result[]; cat: string }) {
  const open = useOpen(results);
  const sel = useKeyNav(results.length, open);
  return (
    <ol className="res">
      {results.map((r, i) => (
        <li key={r.url} className={`g${i === sel ? " sel" : ""}`} data-i={i}>
          {i === sel && <span className="sel-arrow">&#9658;</span>}
          {cat === "videos" && r.thumbnail && (
            <a href={r.url} rel="noreferrer noopener" className="vthumb">
              <img src={proxied(r.thumbnail)} alt="" loading="lazy" />
            </a>
          )}
          <h3 className="r">
            <a href={r.url} rel="noreferrer noopener">
              <Highlight text={r.title || r.url} q={q} />
            </a>
          </h3>
          <div className="s">
            {r.meta && <span className="meta">{r.meta} - </span>}
            <Highlight text={r.snippet} q={q} />
            <br />
            <cite>{displayURL(r.url)}</cite>
            <span className="gl">
              {" - "}
              <Link to={readerHref(r.url)}>Reader</Link>
              {" - "}
              <a href={`https://web.archive.org/web/${r.url}`} rel="noreferrer">
                Archived
              </a>
              <span className="engs"> - {r.engines.join(", ")}</span>
            </span>
          </div>
        </li>
      ))}
    </ol>
  );
}

type SizeFilter = "all" | "large" | "medium" | "small";

const SIZES: Record<SizeFilter, [string, (w: number, h: number) => boolean]> = {
  all: ["All image sizes", () => true],
  large: ["Large images", (w, h) => Math.max(w, h) >= 1000],
  medium: ["Medium images", (w, h) => Math.max(w, h) >= 400 && Math.max(w, h) < 1000],
  small: ["Small images", (w, h) => Math.max(w, h) > 0 && Math.max(w, h) < 400],
};

const dims = (res?: string): [number, number] => {
  const m = /(\d+)\D+(\d+)/.exec(res || "");
  return m ? [+m[1], +m[2]] : [0, 0];
};

function ImageResults({ q, results }: { q: string; results: Result[] }) {
  const [size, setSize] = useState<SizeFilter>("all");
  const [shown, setShown] = useState<Result | null>(null);
  const list = results.filter((r) => SIZES[size][1](...dims(r.resolution)));
  return (
    <>
      <div className="isize">
        Showing:{" "}
        <Form.Select size="sm" value={size} onChange={(e) => setSize(e.target.value as SizeFilter)} aria-label="Image size" className="classic-select">
          {Object.entries(SIZES).map(([k, [label]]) => (
            <option key={k} value={k}>
              {label}
            </option>
          ))}
        </Form.Select>
      </div>
      <div className="igrid">
        {list.map((r) => {
          const [w, h] = dims(r.resolution);
          const ft = fileType(r.image);
          return (
            <div className="icell" key={r.url + r.image}>
              <a
                href={r.image}
                className="ithumb"
                onClick={(e) => {
                  e.preventDefault();
                  setShown(r);
                }}
              >
                <Thumb r={r} />
              </a>
              <div className="ititle">
                <Highlight text={r.title} q={q} />
              </div>
              <div className="imeta">
                {w > 0 && `${w} x ${h}`}
                {w > 0 && ft && " - "}
                {ft}
              </div>
              <cite>{host(r.url)}</cite>
            </div>
          );
        })}
      </div>
      {!list.length && <p className="nomatch">No {SIZES[size][0].toLowerCase()} on this page.</p>}
      <ImageFrame r={shown} onHide={() => setShown(null)} />
    </>
  );
}

// Thumbnail with fallback: thumbnail -> full image -> hide the cell.
function Thumb({ r }: { r: Result }) {
  const srcs = [...new Set([r.thumbnail, r.image].filter(Boolean))];
  const [i, setI] = useState(0);
  return (
    <img
      src={proxied(srcs[i])}
      alt={r.title}
      loading="lazy"
      onError={(e) => (i + 1 < srcs.length ? setI(i + 1) : e.currentTarget.closest(".icell")?.classList.add("d-none"))}
    />
  );
}

// The old "Below is the image at full size" frame, as a modal.
function ImageFrame({ r, onHide }: { r: Result | null; onHide: () => void }) {
  const [fallback, setFallback] = useState(false);
  useEffect(() => setFallback(false), [r]);
  if (!r) return null;
  const [w, h] = dims(r.resolution);
  return (
    <Modal show onHide={onHide} size="xl" centered className="classic-modal iframe-modal">
      <Modal.Header closeButton>
        <div className="iframe-head">
          <img src={proxied(r.thumbnail || r.image)} alt="" className="iframe-thumb" />
          <div>
            <div className="iframe-title">{r.title}</div>
            {w > 0 && (
              <div className="imeta">
                {w} x {h} {fileType(r.image) && `- ${fileType(r.image)}`}
              </div>
            )}
            <cite>{displayURL(r.url)}</cite>
            <div className="iframe-links">
              Below is the image at full size. <a href={r.image} rel="noreferrer">See full-size image.</a>{" "}
              <a href={r.url} rel="noreferrer">
                Visit page
              </a>{" "}
              <Link to={readerHref(r.url)} onClick={onHide}>
                Reader view
              </Link>
            </div>
          </div>
        </div>
      </Modal.Header>
      <Modal.Body className="iframe-body">
        <img src={proxied(fallback ? r.thumbnail : r.image)} alt={r.title} onError={() => !fallback && setFallback(true)} />
      </Modal.Body>
    </Modal>
  );
}

function NewsResults({ q, results }: { q: string; results: Result[] }) {
  const [byDate, setByDate] = useState(false);
  const list = byDate ? [...results].sort((a, b) => (b.published || "").localeCompare(a.published || "")) : results;
  const open = useOpen(list);
  const sel = useKeyNav(list.length, open);
  return (
    <>
      <div className="nsort">
        {byDate ? (
          <>
            <a href="#rel" onClick={(e) => (e.preventDefault(), setByDate(false))}>
              Sort by relevance
            </a>{" "}
            <b>Sorted by date</b>
          </>
        ) : (
          <>
            <b>Sorted by relevance</b>{" "}
            <a href="#date" onClick={(e) => (e.preventDefault(), setByDate(true))}>
              Sort by date
            </a>
          </>
        )}
      </div>
      <div className="res news">
        {list.map((r, i) => (
          <div key={r.url} className={`g n${i === sel ? " sel" : ""}`} data-i={i}>
            {i === sel && <span className="sel-arrow">&#9658;</span>}
            {r.thumbnail && <NewsThumb r={r} />}
            <h3 className="r">
              <a href={r.url} rel="noreferrer noopener">
                <Highlight text={r.title} q={q} />
              </a>
            </h3>
            <div className="nsrc">
              <span className="src">{r.source || host(r.url)}</span>
              {r.published && <> - {relTime(r.published)}</>}
            </div>
            <div className="s">
              <Highlight text={r.snippet} q={q} />
              <br />
              <span className="gl">
                <Link to={readerHref(r.url)}>Reader</Link>
                {" - "}
                <Link to={searchURL(r.title, "news")}>Related articles &raquo;</Link>
              </span>
            </div>
          </div>
        ))}
      </div>
    </>
  );
}

function NewsThumb({ r }: { r: Result }) {
  const [failed, setFailed] = useState(false);
  if (failed) return null;
  return (
    <a href={r.url} rel="noreferrer noopener" className="nthumb">
      <img src={proxied(r.thumbnail)} alt="" loading="lazy" onError={() => setFailed(true)} />
      <span>{r.source || host(r.url)}</span>
    </a>
  );
}

function EngineLine({ engines, pending }: { engines: EngineStatus[]; pending: boolean }) {
  const [open, setOpen] = useState(false);
  if (!engines?.length) return null;
  const ok = engines.filter((e) => !e.error).length;
  return (
    <div className="englines">
      <a href="#engines" onClick={(e) => (e.preventDefault(), setOpen(!open))}>
        {open ? "Hide" : "Show"} engine details
      </a>{" "}
      ({ok} of {engines.length} engines answered{pending && ", more still searching..."})
      {open && (
        <table>
          <tbody>
            {engines.map((e) => (
              <tr key={e.name} className={e.error ? "bad" : ""}>
                <td>{e.name}</td>
                <td>{e.skipped ? "paused" : e.error ? "error" : `${e.results} results`}</td>
                <td>{!e.skipped && `${e.took_ms} ms`}</td>
                <td className="err">{!e.skipped && e.error}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
