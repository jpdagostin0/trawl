import { useState, type ReactNode } from "react";
import { useSearchParams } from "react-router";
import { Footer, GBar, Logo, SearchForm, TipsModal } from "./chrome";
import { useConfig } from "./util";

const blurbs: Record<string, ReactNode> = {
  general: (
    <>
      Many engines, no ads, no tracking. Paste a URL to read it in <b>Reader view</b>.
    </>
  ),
  images: "Search for images on the web, fetched privately through trawl.",
  news: "Search news from thousands of sources.",
};

export default function Home() {
  const [params] = useSearchParams();
  const { categories } = useConfig();
  const c = params.get("c");
  const cat = c && categories.some((x) => x.id === c) ? c : "general";
  const [tips, setTips] = useState(false);
  document.title = cat === "general" ? "trawl" : `trawl ${cat[0].toUpperCase() + cat.slice(1)}`;

  return (
    <div className="page-home">
      <GBar cat={cat} onTips={() => setTips(true)} />
      <main className="home-main">
        <Logo size="large" sub={cat} />
        <div className="home-form">
          <SearchForm variant="home" cat={cat} key={cat} onTips={() => setTips(true)} />
        </div>
        <p className="home-blurb">{blurbs[cat]}</p>
      </main>
      <Footer />
      <TipsModal show={tips} onHide={() => setTips(false)} />
    </div>
  );
}
