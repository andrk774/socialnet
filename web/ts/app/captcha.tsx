// Яндекс SmartCaptcha на формах регистрации и сброса пароля. Скрипт виджета
// грузится только если капча включена на сервере (иначе CSP его и не пустит).
import { h } from "../vendor/preact/index.js";
import { useEffect, useRef } from "../vendor/preact/hooks.js";
import { get } from "./api.js";

interface SmartCaptcha { render(el: HTMLElement, opts: { sitekey: string; hl?: string; callback?: (token: string) => void }): number; reset(id?: number): void }
declare global { interface Window { smartCaptcha?: SmartCaptcha; __snCaptchaReady?: () => void } }

let loading: Promise<SmartCaptcha> | null = null;
function loadWidget(): Promise<SmartCaptcha> {
  loading ??= new Promise((resolve, reject) => {
    window.__snCaptchaReady = () => (window.smartCaptcha ? resolve(window.smartCaptcha) : reject(new Error("captcha")));
    const s = document.createElement("script");
    s.src = "https://smartcaptcha.cloud.yandex.ru/captcha.js?render=onload&onload=__snCaptchaReady";
    s.defer = true;
    s.onerror = () => { loading = null; reject(new Error("captcha script")); };
    document.head.appendChild(s);
  });
  return loading;
}

let siteKey: Promise<string> | null = null;
export const captchaSiteKey = (): Promise<string> => {
  siteKey ??= get<{ captcha?: { enabled: boolean; site_key: string } }>("/client-config")
    .then((c) => (c.captcha?.enabled ? c.captcha.site_key : ""), () => { siteKey = null; return ""; });
  return siteKey;
};

// tokenFrom — одноразовый токен, который виджет кладёт в скрытое поле smart-token.
export const tokenFrom = (el: HTMLElement | null): string =>
  (el?.querySelector('input[name="smart-token"]') as HTMLInputElement | null)?.value ?? "";

export function Captcha({ siteKey, box }: { siteKey: string; box: { current: HTMLDivElement | null } }) {
  const inner = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!siteKey || !inner.current) return;
    let alive = true;
    loadWidget().then((sc) => { if (alive && inner.current && !inner.current.childElementCount) sc.render(inner.current, { sitekey: siteKey, hl: "ru" }); }, () => {});
    return () => { alive = false; };
  }, [siteKey]);
  if (!siteKey) return null;
  return <div ref={(el) => { box.current = el; }} class="captcha-box"><div ref={inner} class="captcha" /></div>;
}
