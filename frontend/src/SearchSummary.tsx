import FollowUp from "./FollowUp";
import SummaryMarkdown from "./SummaryMarkdown";
import type { SearchResponse } from "./types";

export default function SearchSummary({ response }: { response: SearchResponse }) {
  const { summary, summary_pending, summary_failed, summary_session, results } = response;
  if (!summary && !summary_pending && !summary_failed) return null;
  return (
    <section className="search-summary" aria-label="Local search summary">
      <h2 className="r">Local search summary</h2>
      <div aria-live="polite" aria-busy={summary_pending}>
        {summary_pending && !summary && <p>Summarizing the top results…</p>}
        {summary_failed && <p>Summary unavailable. Check that Ollama is running and the model is installed.</p>}
        {summary && <SummaryMarkdown text={summary} results={results} />}
      </div>
      {summary_session && <FollowUp key={summary_session} session={summary_session} results={results} />}
    </section>
  );
}
