import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { del, fileSize, get, patch, saveFile, upload } from "./api.js";
import { Empty, ErrorLine, Loading, Time, useLoad } from "./ui.js";
export function DocumentsPage() {
    const [q, setQ] = useState(""), [search, setSearch] = useState(""), [kind, setKind] = useState("all"), [busy, setBusy] = useState(false), [error, setError] = useState(null), [loading, setLoading] = useState(false);
    useEffect(() => { const timer = setTimeout(() => setSearch(q.trim()), 250); return () => clearTimeout(timer); }, [q]);
    const key = `${kind}:${search}`;
    const currentKey = useRef(key);
    currentKey.current = key;
    const url = `/documents?limit=50&kind=${kind}&q=${encodeURIComponent(search)}`;
    const list = useLoad(async () => ({ ...await get(url), key }), [key]);
    const data = list.data?.key === key ? list.data : undefined;
    async function add(files) { if (!files || busy)
        return; setBusy(true); setError(null); try {
        for (const f of Array.from(files)) {
            if (f.size > 50 * 1024 * 1024)
                throw new Error(`Файл «${f.name}» превышает 50 МБ.`);
            await upload("/documents", f);
        }
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
        list.reload();
    } }
    async function more() { if (!data?.next_cursor || loading)
        return; setLoading(true); try {
        const page = await get(url + `&before=${data.next_cursor}`);
        if (currentKey.current === key)
            list.set({ ...page, key, items: [...data.items, ...page.items] });
    }
    catch (e) {
        if (currentKey.current === key)
            setError(e);
    }
    finally {
        setLoading(false);
    } }
    async function rename(d) { const title = prompt("Новое название", d.title); if (!title?.trim())
        return; try {
        await patch(`/documents/${d.id}`, { title });
        list.reload();
    }
    catch (e) {
        setError(e);
    } }
    async function remove(d) { if (!confirm(`Удалить «${d.title}»? Файл пропадёт из записей и сообщений.`))
        return; try {
        await del(`/documents/${d.id}`);
        list.reload();
    }
    catch (e) {
        setError(e);
    } }
    return h("div", { class: "files-layout" },
        h("section", { class: "library-panel files-main" },
            h("header", { class: "files-heading" },
                h("h1", null,
                    "\u0424\u0430\u0439\u043B\u044B ",
                    data && h("small", { class: "muted" }, data.total)),
                h("label", { class: "primary file-btn" },
                    busy ? "Загружаем…" : "Загрузить файл",
                    h("input", { type: "file", multiple: true, class: "sr-only", disabled: busy, "aria-label": "\u0417\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044C \u0444\u0430\u0439\u043B", accept: ".pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.odt,.ods,.odp,.epub,.djvu,.rtf,.txt,.md,.csv,.zip,.rar,.7z,.gz,.gif", onChange: e => { void add(e.currentTarget.files); e.currentTarget.value = ""; } }))),
            h("input", { class: "files-search", type: "search", "aria-label": "\u041F\u043E\u0438\u0441\u043A \u0444\u0430\u0439\u043B\u043E\u0432", placeholder: "\u041F\u043E\u0438\u0441\u043A \u0444\u0430\u0439\u043B\u043E\u0432", maxLength: 120, value: q, onInput: e => setQ(e.currentTarget.value) }),
            h(ErrorLine, { error: error || list.error }),
            !data && !list.error ? h(Loading, null) : data && !data.items.length ? h(Empty, null, search ? "Файлы не найдены. Измените запрос или категорию." : "В этой категории файлов пока нет.") : data && h("ul", { class: "files-list" }, data.items.map(d => h("li", { key: d.id },
                h("span", { class: `file-icon ${d.ext === "gif" ? "file-gif" : ""}`, "aria-hidden": "true" }, d.ext.toUpperCase()),
                h("div", { class: "file-info" },
                    h("button", { class: "link file-name", onClick: () => saveFile(d.url, `${d.title}.${d.ext}`).catch(setError) },
                        d.title,
                        ".",
                        d.ext),
                    h("p", { class: "muted" },
                        fileSize(d.size),
                        " \u00B7 ",
                        h(Time, { iso: d.created_at }))),
                h("details", { class: "file-menu" },
                    h("summary", { "aria-label": `Действия с файлом ${d.title}` }, "\u2022\u2022\u2022"),
                    h("div", null,
                        h("button", { onClick: () => rename(d) }, "\u041F\u0435\u0440\u0435\u0438\u043C\u0435\u043D\u043E\u0432\u0430\u0442\u044C"),
                        h("button", { onClick: () => remove(d) }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C")))))),
            data?.next_cursor && h("button", { class: "ghost", disabled: loading, onClick: more }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u0435\u0449\u0451"),
            h("p", { class: "muted files-note" }, "\u0414\u043E\u043A\u0443\u043C\u0435\u043D\u0442\u044B, GIF \u0438 \u0430\u0440\u0445\u0438\u0432\u044B \u2014 \u0434\u043E 50 \u041C\u0411. \u0421\u043A\u0430\u0447\u0430\u0442\u044C \u0444\u0430\u0439\u043B \u043C\u043E\u0433\u0443\u0442 \u0442\u043E\u043B\u044C\u043A\u043E \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u0438, \u043A\u043E\u0442\u043E\u0440\u044B\u043C \u0434\u043E\u0441\u0442\u0443\u043F\u043D\u0430 \u0437\u0430\u043F\u0438\u0441\u044C \u0438\u043B\u0438 \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435 \u0441 \u043D\u0438\u043C.")),
        h("nav", { class: "settings-navigation", "aria-label": "\u041A\u0430\u0442\u0435\u0433\u043E\u0440\u0438\u0438 \u0444\u0430\u0439\u043B\u043E\u0432" }, [["all", "Все файлы"], ["text", "Текстовые"], ["gif", "GIF-изображения"], ["archive", "Архивы"]].map(([id, label]) => h("button", { class: kind === id ? "active" : "", "aria-pressed": kind === id, onClick: () => setKind(id) }, label))));
}
