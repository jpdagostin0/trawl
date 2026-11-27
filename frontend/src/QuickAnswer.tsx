import { Link } from "react-router";
import { Highlight, host, proxied } from "./util";
import type { Answer, Infobox } from "./types";

const readerHref = (u: string) => `/read?url=${encodeURIComponent(u)}`;

// The onebox above the results: the old calculator line for math and unit
// conversions, or a Wikipedia summary for things.
export default function QuickAnswer({ answer, infobox, q }: { answer?: Answer; infobox?: Infobox; q: string }) {
  if (answer) return <Calc a={answer} />;
  if (infobox) return <Wiki ib={infobox} q={q} />;
  return null;
}

function Calc({ a }: { a: Answer }) {
  return (
    <table className="onebox calc" role="presentation">
      <tbody>
        <tr>
          <td className="ob-icon">
            <img src="/static/calc.svg" width="40" height="30" alt="" />
          </td>
          <td>
            <h2 className="r">
              <b>
                {a.expr} = {a.result}
              </b>
            </h2>
            <div className="ob-more">{a.kind === "convert" ? "Unit conversion" : "Calculator"}</div>
          </td>
        </tr>
      </tbody>
    </table>
  );
}

function Wiki({ ib, q }: { ib: Infobox; q: string }) {
  const text = ib.extract.length > 420 ? ib.extract.slice(0, 417).replace(/\s+\S*$/, "") + " ..." : ib.extract;
  return (
    <table className="onebox wiki" role="presentation">
      <tbody>
        <tr>
          {ib.image && (
            <td className="ob-icon">
              <a href={ib.url} rel="noreferrer">
                <img src={proxied(ib.image)} alt="" loading="lazy" />
              </a>
            </td>
          )}
          <td>
            <h3 className="r">
              <a href={ib.url} rel="noreferrer">
                <Highlight text={ib.title} q={q} />
              </a>{" "}
              <span className="ob-src">- Wikipedia</span>
            </h3>
            <div className="s">
              {text}
              <br />
              <cite>{host(ib.url)}</cite> - <Link to={readerHref(ib.url)}>Reader</Link>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  );
}
