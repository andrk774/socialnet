import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { ApiError, fullName } from "./api.js";
export function Avatar({ user, size = 40 }) {
    const letter = (user.first_name || user.username).slice(0, 1).toUpperCase();
    // оттенок из id — у каждого свой, но в пределах палитры
    const hue = 150 + ((user.id * 47) % 60);
    return user.avatar_url
        ? h("img", { class: "avatar", src: user.avatar_url, alt: "", width: size, height: size, loading: "lazy" })
        : h("span", { class: "avatar avatar-letter", style: { width: size, height: size, fontSize: size * 0.42, background: `hsl(${hue} 35% 38%)` }, "aria-hidden": "true" }, letter);
}
export function UserLink({ user }) {
    return h("a", { class: "user-link", href: `/u/${user.username}` }, fullName(user));
}
const rtf = new Intl.RelativeTimeFormat("ru", { numeric: "auto" });
const dtf = new Intl.DateTimeFormat("ru", { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });
export function Time({ iso }) {
    const d = new Date(iso);
    const sec = (d.getTime() - Date.now()) / 1000;
    let text;
    if (Math.abs(sec) < 60)
        text = "только что";
    else if (Math.abs(sec) < 3600)
        text = rtf.format(Math.round(sec / 60), "minute");
    else if (Math.abs(sec) < 86400)
        text = rtf.format(Math.round(sec / 3600), "hour");
    else
        text = dtf.format(d);
    return h("time", { dateTime: iso, title: d.toLocaleString("ru") }, text);
}
// Текст поста: #хэштеги, @упоминания и http(s)-ссылки. Только текстовые узлы
// и <a> с проверенной схемой — никакого HTML из чужого текста.
const token = /(#[\p{L}\p{N}_]{1,64})|(@[A-Za-z0-9_]{3,32})|(https?:\/\/[^\s<>"']{2,500})/gu;
export function RichText({ text }) {
    const out = [];
    let last = 0;
    for (const m of text.matchAll(token)) {
        if (m.index > last)
            out.push(text.slice(last, m.index));
        if (m[1])
            out.push(h("a", { href: `/tag/${encodeURIComponent(m[1].slice(1))}` }, m[1]));
        else if (m[2])
            out.push(h("a", { href: `/u/${m[2].slice(1)}` }, m[2]));
        else {
            let url = m[3].replace(/[.,!?)]+$/, "");
            try {
                const u = new URL(url);
                out.push(h("a", { href: u.href, target: "_blank", rel: "noopener noreferrer nofollow ugc", "data-external": true }, url));
            }
            catch {
                out.push(url);
            }
            if (url.length < m[3].length)
                out.push(m[3].slice(url.length));
        }
        last = m.index + m[0].length;
    }
    out.push(text.slice(last));
    return h(Fragment, null, out);
}
export function errorText(e) {
    if (e instanceof ApiError) {
        const f = Object.values(e.fields);
        return f.length ? f.join("; ") : messages[e.code] ?? e.message;
    }
    return "Нет связи с сервером. Проверьте интернет и попробуйте ещё раз.";
}
const messages = {
    invalid_credentials: "Неверная почта или пароль.",
    rate_limited: "Слишком много попыток. Подождите минуту.",
    email_taken: "Эта почта уже зарегистрирована.",
    username_taken: "Это имя пользователя уже занято.",
    privacy: "Пользователь ограничил, кто может ему писать.",
    banned: "Аккаунт заблокирован.",
    payments_disabled: "Покупка монет пока не подключена.",
    email_unverified: "Сначала подтвердите почту — ссылка в письме после регистрации.",
    email_not_verified: "Сначала подтвердите почту — ссылка в письме после регистрации.",
    wallet_debt: "Сначала нужно погасить долг за возвращённую покупку.",
    already_claimed: "Бонус уже получен сегодня. Возвращайтесь завтра.",
};
export function ErrorLine({ error }) {
    return error ? h("p", { class: "error", role: "alert" }, errorText(error)) : null;
}
// useLoad: загрузка данных с повтором по reload().
export function useLoad(fn, deps) {
    const [data, setData] = useState(undefined);
    const [error, setError] = useState(null);
    const [n, setN] = useState(0);
    useEffect(() => {
        let live = true;
        setError(null);
        fn().then((d) => live && setData(d), (e) => live && setError(e));
        return () => { live = false; };
    }, [...deps, n]);
    return { data, error, reload: () => setN((x) => x + 1), set: setData };
}
export function Empty({ children }) {
    return h("p", { class: "empty" }, children);
}
export function Loading() {
    return h("p", { class: "loading", "aria-live": "polite" }, "\u0417\u0430\u0433\u0440\u0443\u0437\u043A\u0430\u2026");
}
export const duration = (s) => `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
