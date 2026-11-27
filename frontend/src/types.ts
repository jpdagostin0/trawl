// Shapes of trawl's JSON API, mirroring the Go structs that produce them.

// engine.Result
export interface Result {
  url: string;
  title: string;
  snippet: string;
  meta?: string;
  thumbnail?: string;
  image?: string;
  resolution?: string;
  source?: string;
  published?: string;
  engines: string[];
  score: number;
}

// engine.Infobox
export interface Infobox {
  title: string;
  extract: string;
  url: string;
  image?: string;
}

// answer.Answer
export interface Answer {
  kind: "calc" | "convert";
  expr: string;
  result: string;
}

// search.EngineStatus
export interface EngineStatus {
  name: string;
  results: number;
  took_ms: number;
  error?: string;
  skipped?: boolean;
}

// web.searchResponse (search.Response + UI hints)
export interface SearchResponse {
  query: string;
  category: string;
  page: number;
  results: Result[];
  answer?: Answer;
  infobox?: Infobox;
  engines: EngineStatus[];
  took_ms: number;
  cached: boolean;
  done: boolean;
  reader_url?: string;
}

// fetch.Page, as returned by /api/scrape?html=1
export interface Page {
  url: string;
  title: string;
  byline?: string;
  site_name?: string;
  excerpt?: string;
  image?: string;
  language?: string;
  published?: string;
  markdown: string;
  html?: string;
  links?: string[];
  word_count: number;
  read_minutes: number;
  fetched_at: string;
}

// /api/config
export interface Config {
  categories: { id: string; engines: string[] }[];
  bangs: Record<string, string>;
  tab_bangs: Record<string, string>;
}
