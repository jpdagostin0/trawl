# trawl

A private metasearch engine with a built-in reader, for humans. Think SearXNG and
Firecrawl's scrape/crawl/map in one static Go binary, wearing the classic 2008 Google
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

Desktop build with a tray icon (shows that trawl is running and its port, with
Reload, which restarts the server with fresh caches, and Quit):

    go build -tags tray -ldflags="-s -w" -o trawl .

On Linux the tray uses StatusNotifierItem over D-Bus (KDE, GNOME with the AppIndicator
extension, waybar's `tray` module, ...) and needs no cgo. Without `-tags tray` (the
default, and the Docker image) trawl runs headless and stops on SIGINT/SIGTERM.

Flags and env vars: `-addr` (`TRAWL_ADDR`), `-base-url` (`TRAWL_BASE_URL`),
`-disable` (`TRAWL_DISABLE`), `-timeout` (upstream timeout per search, default 3s),
`-allow-private` (`TRAWL_ALLOW_PRIVATE=1`, lets the reader fetch LAN/localhost; for
trusted use only).

## Frontend

The UI lives in `frontend/` (React, react-bootstrap, react-router, Vite). It builds
into `internal/web/dist`, which is committed and embedded with `go:embed`, so
`go build` works without Node.

    cd frontend && npm install && npm run build   # then rebuild the Go binary
    npm run dev                                   # Vite on :5173, API proxied to :8088

## API (Firecrawl-ish)

    GET /api/search?q=...&category=general|images|news|code|forums&page=1&engines=bing,duckduckgo
    GET /api/search?q=...&scrape=3          # also returns markdown for the top 3 results
    GET /api/search?q=...&stream=1          # NDJSON: a merged snapshot as each engine answers; last has "done": true
    GET /api/scrape?url=...[&html=1][&links=1]
    GET /api/map?url=...                    # same-site links on a page
    GET /api/crawl?url=...&limit=10&depth=2 # bounded BFS under the start path, respects robots.txt
    GET /read?url=...&format=md             # raw markdown
    GET /api/config                         # tabs that have engines, bangs
    GET /img?u=...                          # image proxy

The fetcher refuses loopback, private, and link-local addresses. The check runs when
the connection is made, so redirects and DNS rebinding can't get around it.

## Layout

    internal/engine   upstream backends (HTML scrapers + JSON APIs)
    internal/search   fan-out, RRF merge, dedupe, circuit breaker, LRU cache
    internal/fetch    safe HTTP client, readability -> markdown, map, crawl, robots.txt
    internal/web      net/http handlers, embedded frontend build, bangs, favicon/image proxy
    frontend          React + Bootstrap UI (classic look)

To add an engine, implement `engine.Engine` (Name/Category/Weight/Search) and add it
to `engine.All()`.

## Limitations

- Scraped engines (Bing, DuckDuckGo, Mojeek) can rate-limit or captcha a server IP.
  Mojeek currently captchas some IPs, and the breaker pauses it automatically.
- Brave and Google aren't queried directly: Brave cloaks result URLs, and Google
  blocks scrapers. Google's index comes in through Startpage.
- The scraper does not run JavaScript, so pages that only render on the client
  extract poorly. Firecrawl uses headless Chrome for these.
