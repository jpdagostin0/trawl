import { useState } from "react";
import { Link } from "react-router";
import { proxied, useJSON } from "./util";
import type { Result, WikiArticle } from "./types";

const TOP = 5;
const SHOWN_PARAGRAPHS = 2;

// The server turns away talk pages, files and other namespaces.
const isWikipedia = (u: string) => /^https?:\/\/[a-z-]+\.(m\.)?wikipedia\.org\/wiki\/./i.test(u);

// The first Wikipedia article among the top results, if any.
export function topWikipedia(results: Result[]) {
  return results.slice(0, TOP).find((r) => isWikipedia(r.url))?.url;
}

export function useWikiArticle(url: string | undefined) {
  const { data, loading } = useJSON<WikiArticle>(url ? "/api/wiki?url=" + encodeURIComponent(url) : null);
  return url && !loading ? data : null;
}

// The expanded view for a Wikipedia result in the top five, filling the right
// column: a large picture, the short description, the lead, facts and sections.
export default function WikiPanel({ a }: { a: WikiArticle }) {
  const [more, setMore] = useState(false);
  const paras = more ? a.paragraphs : a.paragraphs.slice(0, SHOWN_PARAGRAPHS);
  const reader = `/read?url=${encodeURIComponent(a.url)}`;
  return (
    <aside className="wiki-panel" aria-label={`${a.title} on Wikipedia`}>
      {a.image && (
        <a href={a.url} rel="noreferrer" className="wp-image">
          <img src={proxied(a.image)} alt="" width={a.image_width} height={a.image_height} />
        </a>
      )}
      <div className="wp-body">
        <h2 className="wp-title">
          <a href={a.url} rel="noreferrer">
            {a.title}
          </a>
        </h2>
        {a.description && <div className="wp-desc">{a.description}</div>}
        <div className="wp-extract">
          {paras.map((p, i) => (
            <p key={i}>{p}</p>
          ))}
          {a.paragraphs.length > SHOWN_PARAGRAPHS && (
            <button type="button" className="wp-more" onClick={() => setMore(!more)} aria-expanded={more}>
              {more ? "Less" : "More"}
            </button>
          )}
        </div>
        <div className="wp-src">
          <a href={a.url} rel="noreferrer">
            Wikipedia
          </a>{" "}
          - <Link to={reader}>Reader view</Link>
        </div>
        {!!a.facts?.length && (
          <table className="wp-facts">
            <tbody>
              {a.facts.map((f) => (
                <tr key={f.label}>
                  <th scope="row">{f.label}</th>
                  <td>{f.value}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {!!a.sections?.length && (
          <nav className="wp-sections" aria-label="Article sections">
            <h3>Contents</h3>
            <ul>
              {a.sections.map((s) => (
                <li key={s.anchor}>
                  <a href={`${a.url}#${s.anchor}`} rel="noreferrer">
                    {s.title}
                  </a>
                </li>
              ))}
            </ul>
          </nav>
        )}
      </div>
    </aside>
  );
}
