// Статьи — длинные отчёты о походах. Сервер присылает текст деревом блоков;
// здесь каждый блок рисуется компонентом, HTML из текста не вставляется.
import { Fragment, h } from "../vendor/preact/index.js";
import { useRef, useState } from "../vendor/preact/hooks.js";
import { del, get, patch, post, upload } from "./api.js";
import { navigate } from "./router.js";
import { RouteCard } from "./routes.js";
import { useMe } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, UserLink, useLoad } from "./ui.js";
const dateFmt = new Intl.DateTimeFormat("ru", { day: "numeric", month: "long", year: "numeric" });
function Spans({ spans }) {
    return h(Fragment, null, (spans ?? []).map((s, i) => s.t === "b" ? h("strong", { key: i }, s.text)
        : s.t === "i" ? h("em", { key: i }, s.text)
            : s.t === "a" ? h("a", { key: i, href: s.href, target: "_blank", rel: "noopener nofollow ugc" }, s.text)
                : h(Fragment, { key: i }, s.text)));
}
export function ArticleBody({ blocks }) {
    return (h("div", { class: "article-body" }, blocks.map((b, i) => {
        switch (b.kind) {
            case "h2": return h("h2", { key: i },
                h(Spans, { spans: b.spans }));
            case "h3": return h("h3", { key: i },
                h(Spans, { spans: b.spans }));
            case "ul": return h("ul", { key: i }, b.items.map((it, j) => h("li", { key: j },
                h(Spans, { spans: it }))));
            case "ol": return h("ol", { key: i }, b.items.map((it, j) => h("li", { key: j },
                h(Spans, { spans: it }))));
            case "quote": return h("blockquote", { key: i },
                h(Spans, { spans: b.spans }));
            case "hr": return h("hr", { key: i });
            case "img": return (h("figure", { key: i },
                h("img", { src: `/media/${b.media_id}/full`, alt: b.caption ?? "", loading: "lazy" }),
                b.caption && h("figcaption", null, b.caption)));
            default: return h("p", { key: i },
                h(Spans, { spans: b.spans }));
        }
    })));
}
function Card({ a }) {
    return (h("li", { class: "article-card" },
        a.cover_thumb_url && h("a", { href: `/articles/${a.id}` },
            h("img", { src: a.cover_thumb_url, alt: "", width: 120, height: 90, loading: "lazy" })),
        h("div", { class: "grow" },
            h("a", { class: "article-title", href: `/articles/${a.id}` }, a.title),
            a.status === "draft" && h("span", { class: "pill" }, "\u0447\u0435\u0440\u043D\u043E\u0432\u0438\u043A"),
            h("p", { class: "muted" },
                a.author.first_name || a.author.username,
                a.published_at ? ` · ${dateFmt.format(new Date(a.published_at))}` : ""),
            a.excerpt && h("p", { class: "article-excerpt" }, a.excerpt))));
}
export function ArticlesPage() {
    const list = useLoad(() => get("/articles?limit=30"), []);
    const mine = useLoad(() => get("/articles/mine"), []);
    const drafts = (mine.data?.items ?? []).filter((a) => a.status === "draft");
    return (h("section", null,
        h("div", { class: "page-head" },
            h("h1", null, "\u0421\u0442\u0430\u0442\u044C\u0438"),
            h("a", { class: "primary button-link", href: "/articles/new" }, "\u041D\u0430\u043F\u0438\u0441\u0430\u0442\u044C \u043E\u0442\u0447\u0451\u0442")),
        drafts.length > 0 && h(Fragment, null,
            h("h2", null, "\u041C\u043E\u0438 \u0447\u0435\u0440\u043D\u043E\u0432\u0438\u043A\u0438"),
            h("ul", { class: "article-list" }, drafts.map((a) => h(Card, { key: a.id, a: a })))),
        list.error ? h(ErrorLine, { error: list.error }) : !list.data ? h(Loading, null) : list.data.items.length === 0 ? h(Empty, null, "\u0421\u0442\u0430\u0442\u0435\u0439 \u043F\u043E\u043A\u0430 \u043D\u0435\u0442. \u0420\u0430\u0441\u0441\u043A\u0430\u0436\u0438\u0442\u0435 \u043E \u0441\u0432\u043E\u0451\u043C \u043F\u043E\u0445\u043E\u0434\u0435 \u043F\u0435\u0440\u0432\u044B\u043C.") : (h("ul", { class: "article-list" }, list.data.items.map((a) => h(Card, { key: a.id, a: a }))))));
}
export function ArticlePage({ id }) {
    const me = useMe();
    const a = useLoad(() => get(`/articles/${id}`), [id]);
    const route = useLoad(() => (a.data?.route_id ? get(`/routes/${a.data.route_id}`) : Promise.resolve(null)), [a.data?.route_id]);
    const [error, setError] = useState(null);
    if (a.error)
        return h(ErrorLine, { error: a.error });
    if (!a.data)
        return h(Loading, null);
    const art = a.data, mine = art.author.id === me?.id;
    return (h("article", { class: "article" },
        h("p", { class: "muted" },
            h("a", { href: "/articles" }, "\u2190 \u0421\u0442\u0430\u0442\u044C\u0438")),
        art.cover_url && h("img", { class: "article-cover", src: art.cover_url, alt: "" }),
        h("h1", null, art.title),
        h("p", { class: "article-meta" },
            h(Avatar, { user: art.author, size: 28 }),
            " ",
            h(UserLink, { user: art.author }),
            art.published_at ? h("span", { class: "muted" },
                " \u00B7 ",
                dateFmt.format(new Date(art.published_at))) : h("span", { class: "pill" }, "\u0447\u0435\u0440\u043D\u043E\u0432\u0438\u043A")),
        mine && h("div", { class: "row" },
            h("a", { class: "ghost button-link", href: `/articles/${art.id}/edit` }, "\u0420\u0435\u0434\u0430\u043A\u0442\u0438\u0440\u043E\u0432\u0430\u0442\u044C"),
            h("button", { class: "ghost danger", onClick: () => confirm("Удалить статью?") && del(`/articles/${art.id}`).then(() => navigate("/articles"), setError) }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C")),
        h(ErrorLine, { error: error }),
        h(ArticleBody, { blocks: art.blocks ?? [] }),
        route.data && h("section", null,
            h("h2", null, "\u041C\u0430\u0440\u0448\u0440\u0443\u0442"),
            h(RouteCard, { route: route.data }))));
}
export function ArticleEditor({ id }) {
    const existing = useLoad(() => (id ? get(`/articles/${id}`) : Promise.resolve(null)), [id]);
    if (id && !existing.data)
        return existing.error ? h(ErrorLine, { error: existing.error }) : h(Loading, null);
    return h(Editor, { initial: existing.data ?? null });
}
function Editor({ initial }) {
    const routes = useLoad(() => get("/routes?limit=100"), []);
    const [title, setTitle] = useState(initial?.title ?? "");
    const [body, setBody] = useState(initial?.body ?? "");
    const [cover, setCover] = useState(initial?.cover_media_id ?? null);
    const [routeId, setRouteId] = useState(initial?.route_id ?? "");
    const [preview, setPreview] = useState(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    const area = useRef(null);
    const insertPhoto = async (file) => {
        setError(null);
        try {
            const m = await upload("/media", file);
            const el = area.current, at = el ? el.selectionStart : body.length;
            const line = `\n![${file.name.replace(/\.[^.]+$/, "").replace(/[\][]/g, "")}](media:${m.id})\n`;
            setBody(body.slice(0, at) + line + body.slice(at));
        }
        catch (e) {
            setError(e);
        }
    };
    const save = async (status) => {
        setBusy(true);
        setError(null);
        const payload = { title, body, cover_media_id: cover, route_id: routeId || null, status };
        try {
            const a = initial ? await patch(`/articles/${initial.id}`, payload) : await post("/articles", payload);
            navigate(`/articles/${a.id}`);
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    };
    return (h("section", { class: "article-editor" },
        h("h1", null, initial ? "Редактирование" : "Новый отчёт"),
        h("label", null,
            "\u0417\u0430\u0433\u043E\u043B\u043E\u0432\u043E\u043A",
            h("input", { value: title, maxLength: 150, required: true, onInput: (e) => setTitle(e.target.value), placeholder: "\u0422\u0440\u0438 \u0434\u043D\u044F \u043D\u0430 \u0422\u0430\u0433\u0430\u043D\u0430\u0435" })),
        h("div", { class: "row" },
            h("label", null,
                "\u041E\u0431\u043B\u043E\u0436\u043A\u0430",
                h("input", { type: "file", accept: "image/jpeg,image/png", onChange: async (e) => {
                        const f = e.target.files?.[0];
                        if (f) {
                            try {
                                setCover((await upload("/media", f)).id);
                            }
                            catch (err) {
                                setError(err);
                            }
                        }
                    } })),
            cover && h("img", { src: `/media/${cover}/thumb`, alt: "\u041E\u0431\u043B\u043E\u0436\u043A\u0430", width: 96, height: 72, class: "cover-preview" }),
            cover && h("button", { class: "link", onClick: () => setCover(null) }, "\u0423\u0431\u0440\u0430\u0442\u044C \u043E\u0431\u043B\u043E\u0436\u043A\u0443")),
        h("label", null,
            "\u041C\u0430\u0440\u0448\u0440\u0443\u0442",
            h("select", { value: routeId, onChange: (e) => setRouteId(e.target.value) },
                h("option", { value: "" }, "\u0431\u0435\u0437 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u0430"),
                routes.data?.items.map((r) => h("option", { key: r.id, value: r.id }, r.title)))),
        h("div", { class: "editor-tabs row tight" },
            h("button", { class: preview ? "chip" : "chip active", onClick: () => setPreview(null) }, "\u0422\u0435\u043A\u0441\u0442"),
            h("button", { class: preview ? "chip active" : "chip", onClick: () => post("/articles/preview", { body }).then((r) => setPreview(r.blocks), setError) }, "\u041F\u0440\u0435\u0434\u043F\u0440\u043E\u0441\u043C\u043E\u0442\u0440"),
            h("label", { class: "chip file-chip" },
                "+ \u0424\u043E\u0442\u043E \u0432 \u0442\u0435\u043A\u0441\u0442",
                h("input", { type: "file", accept: "image/jpeg,image/png", class: "sr-only", onChange: (e) => { const f = e.target.files?.[0]; if (f)
                        insertPhoto(f); } }))),
        preview ? h("div", { class: "preview-box" },
            h(ArticleBody, { blocks: preview })) : (h("label", { class: "sr-only-label" },
            "\u0422\u0435\u043A\u0441\u0442",
            h("textarea", { ref: area, rows: 18, maxLength: 100000, value: body, onInput: (e) => setBody(e.target.value), placeholder: "## День первый\n\nВышли в **7 утра** от Белого ключа…\n\n- вода\n- палатка" }))),
        h("p", { class: "hint" },
            "\u0420\u0430\u0437\u043C\u0435\u0442\u043A\u0430: ",
            h("code", null, "## \u0437\u0430\u0433\u043E\u043B\u043E\u0432\u043E\u043A"),
            ", ",
            h("code", null, "### \u043F\u043E\u0434\u0437\u0430\u0433\u043E\u043B\u043E\u0432\u043E\u043A"),
            ", ",
            h("code", null, "- \u0441\u043F\u0438\u0441\u043E\u043A"),
            ", ",
            h("code", null, "1. \u0441\u043F\u0438\u0441\u043E\u043A"),
            ", ",
            h("code", null, "> \u0446\u0438\u0442\u0430\u0442\u0430"),
            ", ",
            h("code", null, "---"),
            ", ",
            h("code", null, "**\u0436\u0438\u0440\u043D\u044B\u0439**"),
            ", ",
            h("code", null, "*\u043A\u0443\u0440\u0441\u0438\u0432*"),
            ", ",
            h("code", null, "[\u0441\u0441\u044B\u043B\u043A\u0430](https://\u2026)"),
            "."),
        h(ErrorLine, { error: error }),
        h("div", { class: "row" },
            h("button", { class: "primary", disabled: busy, onClick: () => save("published") }, initial?.status === "published" ? "Сохранить" : "Опубликовать"),
            h("button", { class: "ghost", disabled: busy, onClick: () => save("draft") }, initial?.status === "published" ? "Снять в черновики" : "Сохранить черновик"))));
}
