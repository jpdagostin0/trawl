# trawl

A private metasearch engine with a built-in reader, for humans. Think SearXNG and
Firecrawl's scrape/crawl/map in one ~17 MB Go binary. No Python, Redis, Playwright,
or Node.

## Features

- Metasearch: queries Bing, DuckDuckGo, Mojeek, and Wikipedia at the same time
  (tabs: Web, Code = GitHub + Stack Overflow, Forums = Hacker News). Results are
  merged with weighted reciprocal-rank fusion, deduplicated by normalized URL, and
  cleaned of tracking params. Each result shows which engines found it.
- Circuit breaker: an engine that gets rate-limited or captcha'd is paused for a
  while, so it doesn't slow down every search (same idea as SearXNG suspension).
- Reader view: one click turns any result into a clean, readable article (Mozilla
  Readability port). Paste a URL into the search box to read it directly. You can
  change the font size, switch light/dark/sepia themes, and copy or download the
  page as Markdown.
- Quick answers: a Wikipedia infobox for entity queries.
- Bangs: `!g`, `!w`, `!gh`, `!yt`, `!r`, `!so`, `!aw`, `!mdn`, `!pkg`, ... go to the
  external site. `!code` and `!forums` switch tabs without leaving trawl.
- Keyboard: `/` focuses search, `j`/`k` move, `o`/Enter opens, `O` opens in a new
  tab, `r` opens the reader, `n`/`p` change pages.
- Privacy: the browser never talks to a third party. Autocomplete and favicons are
  proxied through trawl. Pages are served with `Referrer-Policy: no-referrer` and a
  strict CSP. Queries are not logged.
- OpenSearch: "Add to browser" makes trawl your default search engine, with
  suggestions.
- Works without JavaScript. JS only adds autocomplete, keyboard nav, and reader tools.

## Run

    go build -o trawl . && ./trawl              # http://127.0.0.1:8088
    ./trawl -addr :8088 -base-url https://search.example.com -disable mojeek

    docker build -t trawl . && docker run -p 8088:8088 trawl

Flags and env vars: `-addr` (`TRAWL_ADDR`), `-base-url` (`TRAWL_BASE_URL`),
`-disable` (`TRAWL_DISABLE`), `-timeout` (upstream timeout per search, default 3s),
`-allow-private` (`TRAWL_ALLOW_PRIVATE=1`, lets the reader fetch LAN/localhost; for
trusted use only).

## API (Firecrawl-ish)

    GET /api/search?q=...&category=general|code|forums&page=1&engines=bing,duckduckgo
    GET /api/search?q=...&scrape=3          # also returns markdown for the top 3 results
    GET /api/scrape?url=...[&html=1][&links=1]
    GET /api/map?url=...                    # same-site links on a page
    GET /api/crawl?url=...&limit=10&depth=2 # bounded BFS under the start path, respects robots.txt
    GET /read?url=...&format=md             # raw markdown

The fetcher refuses loopback, private, and link-local addresses. The check runs when
the connection is made, so redirects and DNS rebinding can't get around it.

## Layout

    internal/engine   upstream backends (HTML scrapers + JSON APIs)
    internal/search   fan-out, RRF merge, dedupe, circuit breaker, LRU cache
    internal/fetch    safe HTTP client, readability -> markdown, map, crawl, robots.txt
    internal/web      net/http handlers, embedded templates/CSS/JS, bangs, favicon proxy

To add an engine, implement `engine.Engine` (Name/Category/Weight/Search) and add it
to `engine.All()`.

## Limitations

- Scraped engines (Bing, DuckDuckGo, Mojeek) can rate-limit or captcha a server IP.
  Mojeek currently captchas some IPs, and the breaker pauses it automatically.
- Brave and Google are not included: Brave cloaks result URLs, and Google blocks
  scrapers.
- The scraper does not run JavaScript, so pages that only render on the client
  extract poorly. Firecrawl uses headless Chrome for these.
