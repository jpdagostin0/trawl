import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { BlueBar, Footer, GBar, Logo, Notice, TipsModal } from "./chrome";
import { host, useJSON } from "./util";
import type { ArchiveArticle, ArchiveList } from "./types";

function size(bytes: number) {
  if (bytes >= 1_000_000_000) return `${(bytes / 1_000_000_000).toLocaleString(undefined, { maximumFractionDigits: 2 })} GB`;
  if (bytes >= 1_000_000) return `${(bytes / 1_000_000).toLocaleString(undefined, { maximumFractionDigits: 1 })} MB`;
  if (bytes >= 1000) return `${(bytes / 1000).toLocaleString(undefined, { maximumFractionDigits: 1 })} KB`;
  return `${bytes} B`;
}

const listURL = (offset: number) => offset > 0 ? `/archive?offset=${offset}` : "/archive";

export default function Archive() {
  const [params] = useSearchParams();
  const [tips, setTips] = useState(false);
  const id = params.get("id");
  const offset = Math.max(0, parseInt(params.get("offset") ?? "", 10) || 0);
  const list = useJSON<ArchiveList>(id ? null : `/api/archive?offset=${offset}`);
  const article = useJSON<ArchiveArticle>(id ? `/api/archive/${encodeURIComponent(id)}` : null);
  const page = article.data?.entry.id === id ? article.data : null;
  document.title = page ? `${page.entry.title} - trawl Archive` : "trawl Archive";
  return (
    <div>
      <GBar cat="archive" onTips={() => setTips(true)} />
      <header className="rhead"><Logo size="small" sub="archive" /></header>
      <BlueBar>{list.data ? `${size(list.data.used_bytes)} of ${size(list.data.limit_bytes)} used` : null}</BlueBar>
      <main className="archive-main">
        {id ? (
          <>
            <Link to={listURL(offset)}>« Back to archive</Link>
            {article.error && <p role="alert">{article.error.message}</p>}
            {!page && !article.error && <Notice>Loading archived page…</Notice>}
            {page && (
              <article className="archive-article">
                <h1>{page.entry.title || page.entry.url}</h1>
                <p>Saved {new Date(page.entry.fetched_at).toLocaleString()} · <a href={page.entry.url} rel="noreferrer noopener">Original page</a></p>
                <Markdown remarkPlugins={[remarkGfm]} skipHtml disallowedElements={["img"]}>
                  {page.markdown}
                </Markdown>
              </article>
            )}
          </>
        ) : (
          <>
            <p>Readable copies of pages fetched for local search summaries. The oldest are removed first when the archive fills up.</p>
            {list.error && <p role="alert">{list.error.message}</p>}
            {list.loading && <Notice>Loading archive…</Notice>}
            {list.data && !list.loading && !list.error && (
              <>
                {!list.data.entries.length && <p>No pages saved on this page.</p>}
                <ul className="res">
                  {list.data.entries.map((entry) => (
                    <li key={entry.id} className="g">
                      <h2 className="r"><Link to={`/archive?${new URLSearchParams({ id: entry.id, offset: String(offset) })}`}>{entry.title || entry.url}</Link></h2>
                      <small>{host(entry.url)} · {new Date(entry.fetched_at).toLocaleString()} · {size(entry.size_bytes)}</small>
                    </li>
                  ))}
                </ul>
                <nav aria-label="Archive pages">
                  {offset > 0 && <Link to={listURL(offset - 40)}>« Newer</Link>}{" "}
                  {list.data.has_more && <Link to={listURL(offset + list.data.entries.length)}>Older »</Link>}
                </nav>
              </>
            )}
          </>
        )}
      </main>
      <Footer />
      <TipsModal show={tips} onHide={() => setTips(false)} />
    </div>
  );
}
