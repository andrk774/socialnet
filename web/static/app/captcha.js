// Яндекс SmartCaptcha на формах регистрации и сброса пароля. Скрипт виджета
// грузится только если капча включена на сервере (иначе CSP его и не пустит).
import { h } from "../vendor/preact/index.js";
import { useEffect, useRef } from "../vendor/preact/hooks.js";
import { get } from "./api.js";
let loading = null;
function loadWidget() {
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
let siteKey = null;
export const captchaSiteKey = () => {
    siteKey ??= get("/client-config")
        .then((c) => (c.captcha?.enabled ? c.captcha.site_key : ""), () => { siteKey = null; return ""; });
    return siteKey;
};
// tokenFrom — одноразовый токен, который виджет кладёт в скрытое поле smart-token.
export const tokenFrom = (el) => el?.querySelector('input[name="smart-token"]')?.value ?? "";
export function Captcha({ siteKey, box }) {
    const inner = useRef(null);
    useEffect(() => {
        if (!siteKey || !inner.current)
            return;
        let alive = true;
        loadWidget().then((sc) => { if (alive && inner.current && !inner.current.childElementCount)
            sc.render(inner.current, { sitekey: siteKey, hl: "ru" }); }, () => { });
        return () => { alive = false; };
    }, [siteKey]);
    if (!siteKey)
        return null;
    return h("div", { ref: (el) => { box.current = el; }, class: "captcha-box" },
        h("div", { ref: inner, class: "captcha" }));
}
