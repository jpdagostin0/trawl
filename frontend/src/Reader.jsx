import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { Logo, Notice } from "./chrome.jsx";
import { host, proxied, store, useJSON } from "./util.jsx";

// Route article images through /img and drop srcset (it would bypass the proxy).
function proxyImages(html) {
  const doc = new DOMParser().parseFromString(`<div>${html}</div>`, "text/html");
  for (const img of doc.querySelectorAll("img")) {
    const src = img.getAttribute("src");
    img.removeAttribute("srcset");
    img.setAttribute("loading", "lazy");
    if (src && /^https?:\/\//i.test(src)) img.setAttribute("src", proxied(src));
    else img.remove();
  }
  doc.querySelectorAll("source").forEach((s) => s.remove());
  return doc.body.firstChild.innerHTML;
}

export default function Reader() {
  const [params] = useSearchParams();
  const url = params.get("url") || "";
  const nav = useNavigate();
  const { data: p, error, loading } = useJSON(url ? `/api/scrape?html=1&url=${encodeURIComponent(url)}` : null);
  const [size, setSize] = useState(+store.get("size", 16));
  const [msg, setMsg] = useState("");
  const html = useMemo(() => (p?.html ? proxyImages(p.html) : ""), [p]);

  useEffect(() => store.set("size", size), [size]);
  useEffect(() => {
    document.title = p ? `${p.title} - trawl Reader` : "trawl Reader";
  }, [p]);
  useEffect(() => {
    if (!msg) return;
    const t = setTimeout(() => setMsg(""), 2000);
    return () => clearTimeout(t);
  }, [msg]);

  const md = `/read?url=${encodeURIComponent(url)}&format=md`;
  const copy = async (e) => {
    e.preventDefault();
    try {
      await navigator.clipboard.writeText(await (await fetch(md)).text());
      setMsg("Markdown copied.");
    } catch {
      setMsg("Couldn't copy.");
    }
  };
  const back = (e) => {
    if (history.length > 1) {
      e.preventDefault();
      nav(-1);
    }
  };
  const fetched = p?.fetched_at ? new Date(p.fetched_at).toUTCString() : "";

  return (
    <div className="page-reader">
      <div className="cache-outer">
        <div className="cache-box">
          <Logo size="tiny" />
          <div className="cache-text">
            This is trawl&apos;s <b>reader view</b> of{" "}
            <a href={p?.url || url} rel="noreferrer">
              {p?.url || url}
            </a>
            .{" "}
            {p && (
              <>
                It is a cleaned-up copy of the page as it appeared on {fetched}. The <a href={p.url} rel="noreferrer">current page</a> could
                have changed in the meantime.
              </>
            )}
            <div className="cache-tools">
              Text size:{" "}
              <a href="#smaller" onClick={(e) => (e.preventDefault(), setSize((s) => Math.max(12, s - 1)))} title="Smaller text">
                A&minus;
              </a>{" "}
              <a href="#larger" onClick={(e) => (e.preventDefault(), setSize((s) => Math.min(28, s + 1)))} title="Larger text">
                A+
              </a>
              &nbsp;&nbsp;-&nbsp;&nbsp;<a href={md}>View as Markdown</a>&nbsp;&nbsp;-&nbsp;&nbsp;<a href={md + "&download=1"}>Save .md</a>
              &nbsp;&nbsp;-&nbsp;&nbsp;
              <a href="#copy" onClick={copy}>
                Copy Markdown
              </a>
              &nbsp;&nbsp;-&nbsp;&nbsp;
              <Link to="/" onClick={back}>
                &laquo; Back
              </Link>
              {msg && (
                <>
                  {" "}
                  <Notice>{msg}</Notice>
                </>
              )}
            </div>
          </div>
        </div>
      </div>

      {loading && (
        <p className="reader-loading">
          <Notice>Loading...</Notice>
        </p>
      )}
      {error && (
        <div className="sorry reader-error">
          <b>We&apos;re sorry...</b>
          <p>... but trawl couldn&apos;t open this page in reader view.</p>
          <p className="detail">{error.message}</p>
          <p>
            <a href={url} rel="noreferrer">
              Open the original page instead &raquo;
            </a>
          </p>
        </div>
      )}
      {p && (
        <article className="reader" style={{ fontSize: size }}>
          <h1>{p.title}</h1>
          <p className="reader-src">
            <cite>{p.site_name || host(p.url)}</cite>
            {p.byline && <> - {p.byline}</>}
            {p.published && <> - {p.published}</>} - {p.read_minutes} min read
          </p>
          <div className="reader-content" dangerouslySetInnerHTML={{ __html: html }} />
          <p className="reader-orig">
            <a href={p.url} rel="noreferrer">
              View the original page &raquo;
            </a>
          </p>
        </article>
      )}
    </div>
  );
}
