// Маршрутизация через History API: адреса обычные (/u/anna, /im/12).
import { useEffect, useState } from "../vendor/preact/hooks.js";

const subs = new Set<() => void>();
const notify = () => subs.forEach((fn) => fn());
window.addEventListener("popstate", notify);

export function navigate(to: string, replace = false) {
  if (replace) history.replaceState(null, "", to);
  else history.pushState(null, "", to);
  window.scrollTo(0, 0);
  notify();
}

export function usePath(): string {
  const [path, setPath] = useState(location.pathname + location.search);
  useEffect(() => {
    const fn = () => setPath(location.pathname + location.search);
    subs.add(fn);
    return () => { subs.delete(fn); };
  }, []);
  return path;
}

// Перехват кликов по внутренним ссылкам: без перезагрузки страницы.
document.addEventListener("click", (e) => {
  if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
  const a = (e.target as Element).closest?.("a");
  if (!a || a.target || a.hasAttribute("download") || a.origin !== location.origin || a.dataset.external !== undefined) return;
  e.preventDefault();
  navigate(a.pathname + a.search);
});

export type Params = Record<string, string>;

// match("/u/:name", "/u/anna") → { name: "anna" }
export function match(pattern: string, path: string): Params | null {
  const [p] = path.split("?");
  const a = pattern.split("/"), b = p.split("/");
  if (a.length !== b.length) return null;
  const out: Params = {};
  for (let i = 0; i < a.length; i++) {
    if (a[i].startsWith(":")) out[a[i].slice(1)] = decodeURIComponent(b[i]);
    else if (a[i] !== b[i]) return null;
  }
  return out;
}

export const query = (name: string) => new URLSearchParams(location.search).get(name);
