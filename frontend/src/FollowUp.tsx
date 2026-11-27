import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import SummaryMarkdown from "./SummaryMarkdown";
import type { Result } from "./types";
import { readNDJSON } from "./util";

interface Turn {
  question: string;
  answer: string;
  error?: string;
}

type Frame = { answer: string; done: boolean; error?: string };

export default function FollowUp({ session, results }: { session: string; results: Result[] }) {
  const id = useId();
  const active = useRef<AbortController | null>(null);
  const [draft, setDraft] = useState("");
  const [turns, setTurns] = useState<Turn[]>([]);
  const [pending, setPending] = useState<Turn | null>(null);
  useEffect(() => () => active.current?.abort(), []);

  const ask = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const question = draft.trim();
    if (!question || active.current) return;
    const ctrl = new AbortController();
    active.current = ctrl;
    setDraft("");
    setPending({ question, answer: "" });
    let answer = "";
    let done = false;
    try {
      const res = await fetch("/api/summary/follow-up", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ session, question }),
        signal: ctrl.signal,
      });
      for await (const frames of readNDJSON<Frame>(res)) {
        if (ctrl.signal.aborted) return;
        for (const frame of frames) {
          answer = frame.answer;
          done = frame.done;
          if (frame.error) throw new Error(frame.error);
        }
        setPending({ question, answer });
      }
      if (!done) throw new Error("The answer stream ended early. Try again.");
      if (!ctrl.signal.aborted) setTurns((prev) => [...prev, { question, answer }]);
    } catch (error) {
      if (!ctrl.signal.aborted) {
        setTurns((prev) => [...prev, { question, answer, error: error instanceof Error ? error.message : "Could not answer the question." }]);
        setDraft(question);
      }
    } finally {
      if (!ctrl.signal.aborted) setPending(null);
      active.current = null;
    }
  };

  return (
    <div className="summary-follow-up">
      <div aria-live="polite" aria-busy={!!pending}>
        {turns.map((turn, i) => (
          <div className="follow-up-turn" key={i}>
            <p><b>{turn.question}</b></p>
            {turn.answer && <SummaryMarkdown text={turn.answer} results={results} />}
            {turn.error && <p role="alert">{turn.error}</p>}
          </div>
        ))}
        {pending && (
          <div className="follow-up-turn">
            <p><b>{pending.question}</b></p>
            {pending.answer ? <SummaryMarkdown text={pending.answer} results={results} /> : <p>Generating an answer…</p>}
          </div>
        )}
      </div>
      <form onSubmit={ask}>
        <label htmlFor={id}>Ask a follow-up question</label>
        <div className="follow-up-fields">
          <input id={id} type="text" value={draft} maxLength={1500} required onChange={(e) => setDraft(e.target.value)} />
          <button type="submit" disabled={!draft.trim() || !!pending}>Ask</button>
        </div>
      </form>
    </div>
  );
}
