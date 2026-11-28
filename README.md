# trawl

A private metasearch engine with a built-in reader, for humans. Think SearXNG and
Firecrawl's scrape/crawl/map in one Go binary, wearing the classic 2008 Google
look. No Python, Redis or Playwright at runtime; Node is only needed to rebuild the UI.

## Features

- Metasearch: queries Startpage (Google's index), Bing, DuckDuckGo, Mojeek, and
  Wikipedia at the same time. Tabs: Web, Images (Bing Images + Wikimedia Commons),
  News (Bing News + DuckDuckGo News), Code (GitHub + Stack Overflow), Forums (Hacker
  News). Results are merged with weighted reciprocal-rank fusion and deduplicated by
  normalized URL. Each result shows which engines found it.
- ClearURLs: every result URL is cleaned with the [ClearURLs](https://docs.clearurls.xyz)
  rule set (tracking params, referral tags, redirect wrappers like `google.com/url?q=`),
  compiled into the binary.
- Classic UI: React + Bootstrap, styled like google.com circa 2008. Colored logo,
  Suggest dropdown, I'm Feeling Lucky, the blue results bar, and a "Trawwwwwl" pager.
  Images get the thumbnail grid with a size filter and a full-size frame. News gets
  source/age lines and a relevance or date sort.
- Your ranking: the ▲/▼ next to each web or news result moves it up or down, and
  the list re-sorts as soon as you click (SearchWiki-style). Votes are stored in
  DuckDB (`~/.local/share/trawl/trawl.duckdb`), together with embeddings of the
  voted query and page from a local Ollama model (`embeddinggemma` by default).
  Each search is matched against them with vector search in SQL
  (`list_cosine_similarity`). Every vote affects every search, at full strength on
  the search you voted on and half strength on unrelated ones:
  - Votes on the same page: 4x (or 1/4x) on the voted search, 2x (1/2x) elsewhere.
  - Votes on other pages of the same site: a small nudge.
  - Votes on other pages whose title and snippet embed close to this result's: up
    to about 2.8x, so pages like ones you upvoted rise on searches you never voted on.

  The `votes` table keeps every click with the query, URL, title, snippet, engines
  and the position it was shown at, so it can be mined or used to train a reranker.
  Only voted queries and pages are stored. Other searches are embedded in memory
  and never written. Results stream immediately; if vectors are still being
  computed, the stream waits up to five seconds for a final personalized order.
  Without Ollama, ranking falls back to word overlap and exact page/site matches. Delete the file to forget everything.
- Circuit breaker: an engine that gets rate-limited or captcha'd is paused for a
  while, so it doesn't slow down every search (same idea as SearXNG suspension).
- Reader view: one click turns any result into a clean, readable article (Mozilla
  Readability port), presented like the old "cached page" banner. Paste a URL into the
  search box to read it directly. You can change the font size, and copy or download
  the page as Markdown.
- Quick answers: the old Google calculator onebox above the results. Math
  (`5 + 5`, `2^10`, `sqrt(2) * pi`, `what is 12*12`) is evaluated with
  [expr](https://github.com/expr-lang/expr), restricted to arithmetic and math
  functions; unit conversions (`10 km in miles`, `72 f to c`, `how many feet in a mile`)
  use [go-units](https://github.com/bcicen/go-units); definite integrals
  (`integrate x over 1, 5`, `integral of sin(t) dt from 0 to pi`, `∫ exp(-x^2) from -inf to inf`)
  are computed with [gonum](https://www.gonum.org/)'s Gauss-Legendre quadrature. Other queries get a Wikipedia
  summary in the same spot.
- Bangs: `!g`, `!w`, `!gh`, `!yt`, `!r`, `!so`, `!aw`, `!mdn`, `!pkg`, ... go to the
  external site. `!i`, `!n`, `!code`, and `!forums` switch tabs without leaving trawl.
- Keyboard: `/` focuses search, `j`/`k` move, `o`/Enter opens, `O` opens in a new
  tab, `r` opens the reader, `n`/`p` change pages.
- Privacy: the browser never talks to a third party. Autocomplete, thumbnails, infobox
  pictures, and reader-view images are proxied through trawl (`/img`, which uses the
  same SSRF guard as the reader). Pages are served with `Referrer-Policy: no-referrer`
  and a strict CSP (`img-src 'self'`). Queries are not logged.
- OpenSearch: "Add to browser" makes trawl your default search engine, with
  suggestions.
- The web UI needs JavaScript; the JSON API doesn't. Bangs and I'm Feeling Lucky
  (`/search?q=...&btnI=1`) are plain server redirects.

## Run

    go build -o trawl . && ./trawl              # http://127.0.0.1:8088
    ./trawl -addr :8088 -base-url https://search.example.com -disable mojeek

    docker build -t trawl . && docker run -p 8088:8088 trawl

Enable “Local search summaries” in the tray to summarize the top five results
on the first page of text searches. Results appear immediately while trawl
fetches the readable article text, checking `robots.txt` before every request and
redirect. When fetching is blocked or fails, the summary uses the result snippet.
The model receives up to 12 KB of readable text per article to bound its context;
the summary streams above the results and supports Markdown and LaTeX math.
This uses `lfm3:1.7b` through Ollama (`OLLAMA_HOST`); `-summary-model`
(`TRAWL_SUMMARY_MODEL`) overrides the model. Once a summary completes, you can
ask follow-up questions using the same sources and recent conversation. This
context stays in memory for up to 30 minutes. Summaries and follow-ups share
one first-come, first-served queue, so the model answers one request at a time
at full speed instead of splitting it between users. The checkbox starts off unless
`-summary` or `TRAWL_SUMMARY=1` is set.

The Archive tab shows locally saved, readable Markdown snapshots of pages
successfully fetched for summaries. It keeps one snapshot per URL, including the
full extracted article even when the model input was shortened. The archive
uses `~/.local/share/trawl/archive` (or `$XDG_DATA_HOME/trawl/archive`), with a
4 GB cap on stored files. It purges the oldest snapshots first. Override the
path via `-archive-dir` / `TRAWL_ARCHIVE_DIR`, or set `-archive-dir=` to turn
storage off. Anyone who can access this trawl instance can read its archive.

Settings (top right, next to Archive) picks the theme: Classic (2008), or Simple
modern, with light/dark, an accent color and an optional wallpaper image with
dim and blur. Settings are saved in the browser's localStorage. Themes live in
`frontend/src/themes/<name>/` and are registered in
`frontend/src/themes/index.ts`.

Desktop build with a tray icon (shows that trawl is running and its port, with
Reload, which restarts the server with fresh caches, and Quit):

    go build -tags tray -ldflags="-s -w" -o trawl .

On Linux the tray uses StatusNotifierItem over D-Bus (KDE, GNOME with the AppIndicator
extension, waybar's `tray` module, ...) and needs no cgo. Without `-tags tray` (the
default, and the Docker image) trawl runs headless and stops on SIGINT/SIGTERM.

Flags and env vars: `-addr` (`TRAWL_ADDR`), `-base-url` (`TRAWL_BASE_URL`),
`-disable` (`TRAWL_DISABLE`), `-timeout` (upstream timeout per search, default 3s),
`-allow-private` (`TRAWL_ALLOW_PRIVATE=1`, lets the reader fetch LAN/localhost; for
trusted use only), `-db` (`TRAWL_DB`, DuckDB file for votes and vectors; empty disables voting),
`-embed-model` (`TRAWL_EMBED_MODEL`, Ollama embedding model reached via `OLLAMA_HOST`;
empty disables embeddings).

## Frontend

The UI lives in `frontend/` (React, react-bootstrap, react-router, Vite). It builds
into `internal/web/dist`, which is committed and embedded with `go:embed`, so
`go build` works without Node.

    cd frontend && npm install && npm run build   # then rebuild the Go binary
    npm run dev                                   # Vite on :5173, API proxied to :8088

## API (Firecrawl-ish)

    GET /api/search?q=...&category=general|images|news|code|forums&page=1&engines=bing,duckduckgo
    GET /api/search?q=...&scrape=3          # also returns markdown for the top 3 results
    GET /api/search?q=...&stream=1          # NDJSON: snapshots as engines answer and personalization settles; last has "done": true
    GET /api/search?q=...&stream=1&summary=1 # optional summary-only frames follow the ranked snapshots
    POST /api/summary/follow-up           # {"session":"<summary_session>","question":"..."}; streamed answer
    GET /api/archive?offset=0             # snapshots and archive usage
    GET /api/archive/{id}                  # saved Markdown and source metadata
    GET /api/wiki?url=<wikipedia article>  # expanded panel: description, image, lead, infobox facts, sections
    GET /api/scrape?url=...[&html=1][&links=1]
    GET /api/map?url=...                    # same-site links on a page
    GET /api/crawl?url=...&limit=10&depth=2 # bounded BFS under the start path, respects robots.txt
    GET /read?url=...&format=md             # raw markdown
    GET /api/config                         # tabs that have engines, bangs, whether voting is on
    POST /api/vote   {"query","url","dir":1|-1|0,"category","title","snippet","engines","position"}
                                            # JSON only (blocks cross-site form posts); dir 0 clears
    GET /api/votes                          # current votes, newest first
    GET /img?u=...                          # image proxy

The fetcher refuses loopback, private, and link-local addresses. The check runs when
the connection is made, so redirects and DNS rebinding can't get around it.

## Layout

    internal/engine   upstream backends (HTML scrapers + JSON APIs)
    internal/search   fan-out, RRF merge, dedupe, circuit breaker, LRU cache
    internal/feedback votes + vectors in DuckDB, Ollama embeddings, vote-based re-ranking
    internal/fetch    safe HTTP client, readability -> markdown, map, crawl, robots.txt
    internal/web      net/http handlers, embedded frontend build, bangs, favicon/image proxy
    frontend          React + Bootstrap UI; themes in src/themes (classic look)

To add an engine, implement `engine.Engine` (Name/Category/Weight/Search) and add it
to `engine.All()`.

## Limitations

- Scraped engines (Bing, DuckDuckGo, Mojeek) can rate-limit or captcha a server IP.
  Mojeek currently captchas some IPs, and the breaker pauses it automatically.
- Brave and Google aren't queried directly: Brave cloaks result URLs, and Google
  blocks scrapers. Google's index comes in through Startpage.
- The scraper does not run JavaScript, so pages that only render on the client
  extract poorly. Firecrawl uses headless Chrome for these.
