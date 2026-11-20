import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { api, get, post } from "./api.js";
import { query } from "./router.js";
import { ErrorLine, Loading, useLoad } from "./ui.js";
import { LegalIndex } from "./helpcenter.js";
const guides = [
    { id: "start", title: "Начало работы", text: "Создайте приложение и укажите адрес возврата. Пользователь разрешает доступ к своему аккаунту через OAuth. Используйте только необходимые области доступа.", code: "GET /api/v1/users/me\nAuthorization: Bearer <access_token>" },
    { id: "oauth", title: "Авторизация OAuth", text: "Поддерживается authorization code с обязательным PKCE S256. Redirect URI должен точно совпадать с зарегистрированным. Для веб-приложения используйте HTTPS; для локальной разработки допускается loopback HTTP. Client secret хранится только на сервере приложения.", code: "/oauth/authorize?client_id=...&redirect_uri=...&response_type=code&scope=...&state=...&code_challenge=...&code_challenge_method=S256\n\nPOST /api/v1/oauth/token" },
    { id: "requests", title: "Запросы и ошибки", text: "API возвращает JSON. Для JSON-запросов передавайте Content-Type: application/json. Ошибка содержит error.code и error.message. Не повторяйте запросы бесконечно: обрабатывайте 401, 403, 404, 409, 422 и ограничение частоты 429.", code: '{"error":{"code":"forbidden","message":"..."}}' },
    { id: "bots", title: "Боты сообществ", text: "Токены бота доступны через API управления сообществом. Бот может отвечать пользователям, которые уже написали ему. Права токена и роль владельца проверяются сервером.", code: "POST /api/v1/communities/{id}/bot/tokens" },
    { id: "callback", title: "Callback API", text: "Настройте HTTPS-адрес обработчика событий сообщества, затем подтвердите endpoint. Параметры, разрешённые события и ответы приведены в справочнике API.", code: "PUT /api/v1/communities/{id}/callback\nPOST /api/v1/communities/{id}/callback/confirm" },
    { id: "privacy", title: "Доступ и приватность", text: "Токен не отменяет ограничения приватности, блокировки и роли. Не передавайте секреты в URL, публичный frontend или журналы. Пользователь может отозвать доступ приложения.", code: "GET /api/v1/users/me/authorized-apps\nDELETE /api/v1/users/me/authorized-apps/{id}" }
];
function APIReference() {
    const spec = useLoad(() => get("/openapi.json"), []);
    const [q, setQ] = useState(""), [tag, setTag] = useState("all"), [limit, setLimit] = useState(30);
    const methods = ["get", "post", "put", "patch", "delete"];
    const entries = Object.entries(spec.data?.paths || {}).flatMap(([path, ops]) => Object.entries(ops).filter(([m]) => methods.includes(m)).map(([method, op]) => ({ path, method, op })));
    const tags = [...new Set(entries.flatMap(e => e.op.tags || []))];
    const found = entries.filter(e => (tag === "all" || e.op.tags?.includes(tag)) && `${e.path} ${e.method} ${e.op.summary}`.toLowerCase().includes(q.toLowerCase()));
    return h("div", { class: "developer-doc-layout" },
        h("aside", { class: "library-panel" },
            h("label", null,
                "\u041F\u043E\u0438\u0441\u043A API",
                h("input", { type: "search", "aria-label": "\u041F\u043E\u0438\u0441\u043A API", placeholder: "\u041C\u0435\u0442\u043E\u0434, \u043F\u0443\u0442\u044C, \u043E\u043F\u0438\u0441\u0430\u043D\u0438\u0435", value: q, onInput: e => { setQ(e.currentTarget.value); setLimit(30); } })),
            h("label", null,
                "\u0420\u0430\u0437\u0434\u0435\u043B",
                h("select", { "aria-label": "\u0420\u0430\u0437\u0434\u0435\u043B API", value: tag, onChange: e => { setTag(e.currentTarget.value); setLimit(30); } },
                    h("option", { value: "all" }, "\u0412\u0441\u0435 \u0440\u0430\u0437\u0434\u0435\u043B\u044B"),
                    tags.map(t => h("option", null, t)))),
            h("a", { href: "/api/v1/openapi.json", "data-external": true, download: "socialnet-openapi.json" }, "\u0421\u043A\u0430\u0447\u0430\u0442\u044C OpenAPI JSON")),
        h("section", { class: "library-panel" },
            h("h1", null, "\u0421\u043F\u0440\u0430\u0432\u043E\u0447\u043D\u0438\u043A API"),
            h(ErrorLine, { error: spec.error }),
            !spec.data && !spec.error ? h(Loading, null) : h(Fragment, null,
                h("p", { class: "muted" },
                    "\u041D\u0430\u0439\u0434\u0435\u043D\u043E \u043C\u0435\u0442\u043E\u0434\u043E\u0432: ",
                    found.length,
                    ". \u0421\u043F\u0440\u0430\u0432\u043E\u0447\u043D\u0438\u043A \u043F\u043E\u0441\u0442\u0440\u043E\u0435\u043D \u0438\u0437 \u0441\u0445\u0435\u043C\u044B API \u0441\u0435\u0440\u0432\u0435\u0440\u0430."),
                found.slice(0, limit).map(e => h("details", { class: "api-method", key: e.method + e.path },
                    h("summary", null,
                        h("b", null, e.method.toUpperCase()),
                        " ",
                        h("code", null, e.path),
                        h("span", null, e.op.summary)),
                    h("pre", null, JSON.stringify(e.op, null, 2)))),
                found.length > limit && h("button", { class: "ghost", onClick: () => setLimit(limit + 30) }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u0435\u0449\u0451"),
                !found.length && h("p", { class: "empty" }, "\u041C\u0435\u0442\u043E\u0434\u044B \u043D\u0435 \u043D\u0430\u0439\u0434\u0435\u043D\u044B."))));
}
function AppDialog({ app, onClose, onSaved }) {
    const ref = useRef(null);
    const [name, setName] = useState(app?.name || ""), [uris, setUris] = useState(app?.redirect_uris.join("\n") || ""), [confidential, setConfidential] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState(null), [created, setCreated] = useState(null);
    useEffect(() => { ref.current?.showModal(); }, []);
    async function submit(e) { e.preventDefault(); if (busy)
        return; setBusy(true); setError(null); try {
        const body = { name, redirect_uris: uris.split("\n").map(x => x.trim()).filter(Boolean) };
        const result = app ? await api("PATCH", `/apps/${app.id}`, body) : await post("/apps", { ...body, confidential });
        onSaved();
        if (result.client_secret)
            setCreated(result);
        else
            onClose();
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    return h("dialog", { ref: ref, class: "developer-app-dialog", "aria-label": app ? "Изменить приложение" : "Создать приложение", onCancel: e => { e.preventDefault(); if (!busy)
            onClose(); } },
        h("h2", null, created ? "Приложение создано" : app ? "Изменить приложение" : "Создать приложение"),
        h(ErrorLine, { error: error }),
        created ? h(Fragment, null,
            h("p", null, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u0435 \u0441\u0435\u043A\u0440\u0435\u0442 \u0432 \u0431\u0435\u0437\u043E\u043F\u0430\u0441\u043D\u043E\u043C \u0445\u0440\u0430\u043D\u0438\u043B\u0438\u0449\u0435 \u0441\u0435\u0440\u0432\u0435\u0440\u0430. \u041E\u043D \u043F\u043E\u043A\u0430\u0437\u044B\u0432\u0430\u0435\u0442\u0441\u044F \u0442\u043E\u043B\u044C\u043A\u043E \u0441\u0435\u0439\u0447\u0430\u0441."),
            h("label", null,
                "Client ID",
                h("input", { readOnly: true, value: created.client_id })),
            h("label", null,
                "Client secret",
                h("input", { readOnly: true, value: created.client_secret })),
            h("button", { class: "primary", onClick: onClose }, "\u0421\u0435\u043A\u0440\u0435\u0442 \u0441\u043E\u0445\u0440\u0430\u043D\u0451\u043D, \u0437\u0430\u043A\u0440\u044B\u0442\u044C")) : h("form", { onSubmit: submit },
            h("label", null,
                "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435 \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u044F",
                h("input", { required: true, maxLength: 100, value: name, onInput: e => setName(e.currentTarget.value) })),
            h("label", null,
                "Redirect URI, \u043F\u043E \u043E\u0434\u043D\u043E\u043C\u0443 \u043D\u0430 \u0441\u0442\u0440\u043E\u043A\u0443",
                h("textarea", { required: true, rows: 3, value: uris, onInput: e => setUris(e.currentTarget.value), placeholder: "https://example.org/callback" })),
            h("p", { class: "muted" }, "\u041E\u0442 1 \u0434\u043E 10 \u0430\u0434\u0440\u0435\u0441\u043E\u0432. HTTPS \u0438\u043B\u0438 \u043B\u043E\u043A\u0430\u043B\u044C\u043D\u044B\u0439 HTTP \u0434\u043B\u044F \u0440\u0430\u0437\u0440\u0430\u0431\u043E\u0442\u043A\u0438."),
            !app && h("label", { class: "row" },
                h("input", { type: "checkbox", checked: confidential, onChange: e => setConfidential(e.currentTarget.checked) }),
                "\u041F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u0435 \u0438\u043C\u0435\u0435\u0442 \u0441\u0435\u0440\u0432\u0435\u0440 \u0434\u043B\u044F \u0445\u0440\u0430\u043D\u0435\u043D\u0438\u044F \u0441\u0435\u043A\u0440\u0435\u0442\u0430"),
            h("div", { class: "stats-toolbar" },
                h("button", { class: "primary", disabled: busy }, busy ? "Сохраняем…" : app ? "Сохранить" : "Создать"),
                h("button", { type: "button", class: "ghost", disabled: busy, onClick: onClose }, "\u041E\u0442\u043C\u0435\u043D\u0430"))));
}
function MyApps() {
    const apps = useLoad(() => get("/apps"), []);
    const [editing, setEditing] = useState(undefined), [error, setError] = useState(null);
    return h("section", { class: "library-panel" },
        h("header", { class: "files-heading" },
            h("h1", null, "\u041C\u043E\u0438 \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u044F"),
            h("button", { class: "primary", onClick: () => setEditing(null) }, "\u0421\u043E\u0437\u0434\u0430\u0442\u044C \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u0435")),
        h(ErrorLine, { error: error || apps.error }),
        !apps.data && !apps.error ? h(Loading, null) : !apps.data?.items?.length ? h("p", { class: "empty" }, "\u0421\u043E\u0437\u0434\u0430\u0439\u0442\u0435 \u043F\u0435\u0440\u0432\u043E\u0435 \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u0435 \u0434\u043B\u044F \u043F\u043E\u0434\u043A\u043B\u044E\u0447\u0435\u043D\u0438\u044F \u043A API socialnet.") : apps.data.items.map(app => h("article", { class: "developer-app", key: app.id },
            h("h2", null, app.name),
            h("p", { class: "muted" }, app.confidential ? "Серверное приложение" : "Публичный клиент с PKCE"),
            h("label", null,
                "Client ID",
                h("input", { readOnly: true, value: app.client_id })),
            h("ul", null, app.redirect_uris.map(u => h("li", null,
                h("code", null, u)))),
            h("div", { class: "stats-toolbar" },
                h("button", { class: "ghost", onClick: () => setEditing(app) }, "\u0418\u0437\u043C\u0435\u043D\u0438\u0442\u044C"),
                h("button", { class: "link", onClick: async () => { if (!confirm(`Удалить приложение «${app.name}» и отозвать его токены?`))
                        return; try {
                        await api("DELETE", `/apps/${app.id}`);
                        apps.reload();
                    }
                    catch (e) {
                        setError(e);
                    } } }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u0435")))),
        editing !== undefined && h(AppDialog, { app: editing || undefined, onClose: () => setEditing(undefined), onSaved: apps.reload }));
}
function Documentation() { const [q, setQ] = useState(""), [selected, setSelected] = useState("start"); const found = guides.filter(g => `${g.title} ${g.text}`.toLowerCase().includes(q.toLowerCase())); const guide = guides.find(g => g.id === selected); return h("div", { class: "developer-doc-layout" },
    h("aside", { class: "library-panel" },
        h("input", { type: "search", "aria-label": "\u041F\u043E\u0438\u0441\u043A \u0434\u043E\u043A\u0443\u043C\u0435\u043D\u0442\u0430\u0446\u0438\u0438", placeholder: "\u041F\u043E\u0438\u0441\u043A \u0434\u043E\u043A\u0443\u043C\u0435\u043D\u0442\u0430\u0446\u0438\u0438", value: q, onInput: e => setQ(e.currentTarget.value) }),
        h("nav", null,
            found.map(g => h("button", { class: g.id === selected ? "active" : "", onClick: () => setSelected(g.id) }, g.title)),
            !found.length && h("p", null, "\u041D\u0438\u0447\u0435\u0433\u043E \u043D\u0435 \u043D\u0430\u0439\u0434\u0435\u043D\u043E."))),
    h("article", { class: "library-panel developer-guide" },
        h("span", { class: "muted" }, "\u0414\u043E\u043A\u0443\u043C\u0435\u043D\u0442\u0430\u0446\u0438\u044F socialnet"),
        h("h1", null, guide.title),
        h("p", null, guide.text),
        h("pre", null, guide.code),
        h("div", { class: "stats-toolbar" },
            h("a", { href: "/developers?tab=apps" }, "\u041C\u043E\u0438 \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u044F \u2192"),
            h("a", { href: "/developers?tab=api" }, "\u0421\u043F\u0440\u0430\u0432\u043E\u0447\u043D\u0438\u043A API \u2192")))); }
export function DevelopersPage() {
    const tab = query("tab") || "home";
    const cards = [["OAuth", "Вход через socialnet", "Authorization code, PKCE и доступ с согласия пользователя.", "docs"], ["API", "Данные и действия", "Методы, схемы запросов и ответы сервера.", "api"], ["Apps", "Ваши приложения", "Создание клиента и управление адресами возврата.", "apps"], ["Bots", "Боты сообществ", "Интеграция сообщений и событий сообщества.", "docs"]];
    return h("div", { class: "developer-portal" },
        h("header", { class: "developer-nav" },
            h("a", { href: "/developers" },
                h("strong", null, "socialnet"),
                " \u0434\u043B\u044F \u0440\u0430\u0437\u0440\u0430\u0431\u043E\u0442\u0447\u0438\u043A\u043E\u0432"),
            h("nav", null, [["home", "Главная"], ["docs", "Документация"], ["api", "API"], ["apps", "Приложения"], ["rules", "Правила"]].map(([id, label]) => h("a", { class: tab === id ? "active" : "", href: `/developers?tab=${id}` }, label)))),
        tab === "docs" ? h(Documentation, null) : tab === "api" ? h(APIReference, null) : tab === "apps" ? h(MyApps, null) : tab === "rules" ? h(LegalIndex, null) : h(Fragment, null,
            h("section", { class: "developer-hero" },
                h("p", null, "\u041F\u041B\u0410\u0422\u0424\u041E\u0420\u041C\u0410 SOCIALNET"),
                h("h1", null,
                    "\u0421\u043E\u0437\u0434\u0430\u0432\u0430\u0439\u0442\u0435 \u0441\u0435\u0440\u0432\u0438\u0441\u044B",
                    h("br", null),
                    "\u0434\u043B\u044F \u0441\u0432\u043E\u0435\u0433\u043E \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u0430"),
                h("p", null, "\u041F\u043E\u0434\u043A\u043B\u044E\u0447\u0430\u0439\u0442\u0435 \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u044F \u043A socialnet \u0447\u0435\u0440\u0435\u0437 \u043E\u0442\u043A\u0440\u044B\u0442\u044B\u0439 API."),
                h("a", { class: "info-cta", href: "/developers?tab=apps" }, "\u0421\u043E\u0437\u0434\u0430\u0442\u044C \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u0435"),
                h("div", { class: "developer-pixels", "aria-hidden": "true" }, "\u2197")),
            h("section", { class: "developer-products" },
                h("h2", null, "\u0418\u043D\u0441\u0442\u0440\u0443\u043C\u0435\u043D\u0442\u044B \u0434\u043B\u044F \u0432\u0430\u0448\u0438\u0445 \u043F\u0440\u043E\u0435\u043A\u0442\u043E\u0432"),
                h("div", null, cards.map(([icon, title, text, to]) => h("article", null,
                    h("span", { "aria-hidden": "true" }, icon),
                    h("h3", null, title),
                    h("p", null, text),
                    h("a", { href: `/developers?tab=${to}` }, "\u041F\u043E\u0434\u0440\u043E\u0431\u043D\u0435\u0435 \u2192"))))),
            h("section", { class: "library-panel" },
                h("h2", null, "\u041E\u0442 \u0438\u0434\u0435\u0438 \u0434\u043E \u0438\u043D\u0442\u0435\u0433\u0440\u0430\u0446\u0438\u0438"),
                h("ol", { class: "developer-steps" },
                    h("li", null,
                        h("strong", null, "\u0421\u043E\u0437\u0434\u0430\u0439\u0442\u0435 \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u0435"),
                        h("p", null, "\u0417\u0430\u0434\u0430\u0439\u0442\u0435 \u043D\u0430\u0437\u0432\u0430\u043D\u0438\u0435 \u0438 \u0430\u0434\u0440\u0435\u0441 \u0432\u043E\u0437\u0432\u0440\u0430\u0442\u0430.")),
                    h("li", null,
                        h("strong", null, "\u041F\u043E\u0434\u043A\u043B\u044E\u0447\u0438\u0442\u0435 \u0430\u0432\u0442\u043E\u0440\u0438\u0437\u0430\u0446\u0438\u044E"),
                        h("p", null, "\u0418\u0441\u043F\u043E\u043B\u044C\u0437\u0443\u0439\u0442\u0435 PKCE \u0438 \u043D\u0443\u0436\u043D\u044B\u0435 \u043E\u0431\u043B\u0430\u0441\u0442\u0438 \u0434\u043E\u0441\u0442\u0443\u043F\u0430.")),
                    h("li", null,
                        h("strong", null, "\u0420\u0430\u0431\u043E\u0442\u0430\u0439\u0442\u0435 \u0441 API"),
                        h("p", null, "\u041F\u0440\u043E\u0432\u0435\u0440\u044F\u0439\u0442\u0435 \u043E\u0442\u0432\u0435\u0442\u044B, \u043F\u0440\u0430\u0432\u0430 \u0438 \u043E\u0433\u0440\u0430\u043D\u0438\u0447\u0435\u043D\u0438\u044F \u0447\u0430\u0441\u0442\u043E\u0442\u044B.")))),
            h("section", { class: "info-card-grid" },
                h("article", { class: "library-panel" },
                    h("h2", null, "\u0421\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u043E"),
                    h("p", null, "\u0421\u043E\u0437\u0434\u0430\u0439\u0442\u0435 \u043F\u0440\u043E\u0441\u0442\u0440\u0430\u043D\u0441\u0442\u0432\u043E \u0434\u043B\u044F \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u0435\u0439 \u0441\u0432\u043E\u0435\u0433\u043E \u043F\u0440\u043E\u0435\u043A\u0442\u0430."),
                    h("a", { href: "/communities" }, "\u041E\u0442\u043A\u0440\u044B\u0442\u044C \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u0430 \u2192")),
                h("article", { class: "library-panel" },
                    h("h2", null, "\u041C\u0430\u0442\u0435\u0440\u0438\u0430\u043B\u044B"),
                    h("p", null, "\u041F\u0443\u0431\u043B\u0438\u043A\u0443\u0439\u0442\u0435 \u0438\u043D\u0441\u0442\u0440\u0443\u043A\u0446\u0438\u0438 \u0438 \u0434\u0435\u043B\u0438\u0442\u0435\u0441\u044C \u043E\u043F\u044B\u0442\u043E\u043C \u0438\u043D\u0442\u0435\u0433\u0440\u0430\u0446\u0438\u0438."),
                    h("a", { href: "/articles" }, "\u0421\u0442\u0430\u0442\u044C\u0438 socialnet \u2192")),
                h("article", { class: "library-panel" },
                    h("h2", null, "\u041F\u043E\u0434\u0434\u0435\u0440\u0436\u043A\u0430"),
                    h("p", null, "\u041E\u0442\u0432\u0435\u0442\u044B \u043E \u043D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0430\u0445 \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430 \u0438 \u0440\u0430\u0431\u043E\u0442\u0435 \u0441\u0435\u0440\u0432\u0438\u0441\u0430."),
                    h("a", { href: "/help" }, "\u0421\u043F\u0440\u0430\u0432\u043E\u0447\u043D\u044B\u0439 \u0446\u0435\u043D\u0442\u0440 \u2192"))),
            h("footer", { class: "developer-hero" },
                h("h2", null, "\u0420\u0430\u0437\u0432\u0438\u0432\u0430\u0439\u0442\u0435 \u043F\u0440\u043E\u0435\u043A\u0442 \u0432\u043C\u0435\u0441\u0442\u0435 \u0441 socialnet"),
                h("a", { href: "/developers?tab=docs" }, "\u041F\u0435\u0440\u0435\u0439\u0442\u0438 \u043A \u0434\u043E\u043A\u0443\u043C\u0435\u043D\u0442\u0430\u0446\u0438\u0438 \u2192"))));
}
