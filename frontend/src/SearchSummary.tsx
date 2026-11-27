import Markdown from "react-markdown";
import rehypeKatex from "rehype-katex";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import type { SearchResponse } from "./types";

// Ollama can use LaTeX's backslash delimiters even when prompted for dollars.
// Normalize complete pairs before Markdown interprets the backslashes as escapes.
function normalizeMath(text: string) {
  return text.replace(/^ {0,3}(~{3,})[^\n]*\n[\s\S]*?(?:^ {0,3}\1[ \t]*$|(?![\s\S]))|(`+)[\s\S]*?\2|\\\([^\n]*?\\\)|\\\[[\s\S]*?\\\]/gm, (token, _tilde, _ticks, offset: number) => {
    if (!token.startsWith("\\")) return token;
    const math = token.slice(2, -2);
    if (token.startsWith("\\(")) return `$${math}$`;
    if (math.startsWith("\n") && /\n[ \t]*$/.test(math)) return `$$${math}$$`;
    const indent = text.slice(text.lastIndexOf("\n", offset - 1) + 1, offset);
    if (/^[ \t]*$/.test(indent) && /^[ \t]*(?:\n|$)/.test(text.slice(offset + token.length))) {
      return `$$\n${indent}${math.trim()}\n${indent}$$`;
    }
    return `$$${math}$$`;
  });
}

export default function SearchSummary({ response }: { response: SearchResponse }) {
  const { summary, summary_pending, summary_failed, results } = response;
  if (!summary && !summary_pending && !summary_failed) return null;
  const allowed = new Set<string>();
  const refs = results.slice(0, 5).flatMap((result, index) => {
    if (!/^https?:\/\//i.test(result.url) || /[<>\s]/.test(result.url)) return [];
    allowed.add(result.url);
    return [`[${index + 1}]: <${result.url}>`];
  });
  return (
    <section className="search-summary" aria-label="Local search summary" aria-live="polite" aria-busy={summary_pending}>
      <h2 className="r">Local search summary</h2>
      {summary_pending && !summary && <p>Summarizing the top results…</p>}
      {summary_failed && <p>Summary unavailable. Check that Ollama is running and the model is installed.</p>}
      {summary && (
        <div className="summary-content">
          <Markdown
            remarkPlugins={[remarkGfm, remarkMath]}
            // Native MathML doesn't need KaTeX's inline styles, which our CSP blocks.
            rehypePlugins={[[rehypeKatex, { output: "mathml", trust: false }]]}
            skipHtml
            disallowedElements={["img"]}
            urlTransform={(url) => allowed.has(url) ? url : ""}
            components={{ a: ({ href, children }) => href ? <a href={href} rel="noreferrer noopener">{children}</a> : <>{children}</> }}
          >
            {[normalizeMath(summary), refs.join("\n")].join("\n\n")}
          </Markdown>
        </div>
      )}
    </section>
  );
}
