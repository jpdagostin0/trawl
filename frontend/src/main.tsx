import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Route, Routes } from "react-router";
import "bootstrap/dist/css/bootstrap.min.css";
import { startTheme } from "./themes";
import Theme from "./themes/Theme";
import Settings from "./Settings";
import Home from "./Home";
import Results from "./Results";
import Reader from "./Reader";
import Archive from "./Archive";

// The theme's stylesheet loads first, so the page never shows unstyled.
startTheme().then(() =>
  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <BrowserRouter>
        <Theme />
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/search" element={<Results />} />
          <Route path="/read" element={<Reader />} />
          <Route path="/archive" element={<Archive />} />
          <Route path="*" element={<Home />} />
        </Routes>
        <Settings />
      </BrowserRouter>
    </StrictMode>,
  ),
);
