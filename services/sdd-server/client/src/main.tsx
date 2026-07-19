/**
 * @file main.tsx
 * @description The entry point of the React application that renders the main App component into the root DOM element. It uses React's StrictMode for highlighting potential problems in the application and ensures that the app is rendered in a way that adheres to best practices.
 * @author Son Nguyen <hoangson091104@gmail.com>
 */

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./i18n";
import "./index.css";
import { OIDC_ENABLED, getToken, beginLogin, completeLogin, callbackPath } from "./lib/oauth";

if ("serviceWorker" in navigator) {
  // Detect whether the page is already controlled by an SW *before* we
  // register. On a truly fresh install there is no controller yet, so the
  // first `controllerchange` should NOT reload (the user just opened the page
  // — nothing is stale). On every subsequent rebuild the page is controlled,
  // a new SW takes over, and a one-shot reload picks up the new bundle URLs
  // automatically — no hard refresh needed.
  const wasControlled = !!navigator.serviceWorker.controller;
  let reloaded = false;
  navigator.serviceWorker.addEventListener("controllerchange", () => {
    if (!wasControlled || reloaded) return;
    reloaded = true;
    window.location.reload();
  });
  // Register the SW under the app base path so it scopes correctly when the
  // dashboard is mounted behind a gateway sub-path (e.g. /sdd-monitor/).
  const swUrl = `${import.meta.env.BASE_URL.replace(/\/$/, "")}/sw.js`;
  navigator.serviceWorker
    .register(swUrl)
    .then((reg) => {
      // Poke the registration so a freshly-built SW activates promptly
      // instead of waiting for the browser's lazy update check.
      reg.update().catch(() => {});
    })
    .catch(() => {});
}

const root = document.getElementById("root");
if (!root) throw new Error("Root element not found");

function renderApp() {
  createRoot(root!).render(
    <StrictMode>
      <App />
    </StrictMode>
  );
}

// When OIDC is enabled (central coordination server), require a Vortex login
// before the UI mounts: handle the OAuth callback, else redirect to login when
// no token is held. The per-machine monitor build leaves OIDC off and renders
// immediately.
async function boot() {
  if (!OIDC_ENABLED) return renderApp();
  if (window.location.pathname === callbackPath()) {
    try {
      await completeLogin();
    } catch {
      /* fall through — render; api 401s will re-trigger login */
    }
    return renderApp();
  }
  if (!getToken()) {
    await beginLogin(window.location.pathname);
    return; // browser is navigating to Vortex
  }
  renderApp();
}
boot();
