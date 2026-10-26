// Закладки, документы и отложенные записи.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { del, fileSize, get, patch, saveFile, upload } from "./api.js";
import { PostView, Video } from "./posts.js";
import { Empty, ErrorLine, Loading, Time, duration, useLoad } from "./ui.js";
const kinds = [["post", "Записи"], ["video", "Видео"], ["track", "Музыка"]];
export function BookmarksPage() {
    const [kind, setKind] = useState("post");
    const list = useLoad(() => get(`/bookmarks?kind=${kind}&limit=50`), [kind]);
    const [error, setError] = useState(null);
    const unsave = (b, target) => del(`/bookmarks/${b.kind}/${target}`).then(list.reload, setError);
    return (h("section", null,
        h("h1", null, "\u0417\u0430\u043A\u043B\u0430\u0434\u043A\u0438"),
        h("div", { class: "tabs", role: "tablist" }, kinds.map(([k, label]) => h("button", { key: k, role: "tab", "aria-selected": kind === k, class: kind === k ? "tab on" : "tab", onClick: () => setKind(k) }, label))),
        h(ErrorLine, { error: error ?? list.error }),
        !list.data ? h(Loading, null) : list.data.items.length === 0 ? h(Empty, null, "\u0417\u0434\u0435\u0441\u044C \u043F\u043E\u044F\u0432\u0438\u0442\u0441\u044F \u0442\u043E, \u0447\u0442\u043E \u0432\u044B \u0441\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u0435 \u043A\u043D\u043E\u043F\u043A\u043E\u0439 \u00AB\u0412 \u0437\u0430\u043A\u043B\u0430\u0434\u043A\u0438\u00BB.") :
            list.data.items.map((b) => b.post ? h(PostView, { key: b.id, initial: b.post }) :
                b.video ? (h("div", { class: "post", key: b.id },
                    h(Video, { src: b.video.hls_url ?? "", mp4: b.video.mp4_url, poster: b.video.poster_url ?? "", title: b.video.title }),
                    h("button", { class: "link", onClick: () => unsave(b, b.video.id) }, "\u0423\u0431\u0440\u0430\u0442\u044C \u0438\u0437 \u0437\u0430\u043A\u043B\u0430\u0434\u043E\u043A"))) : b.track ? (h("div", { class: "track", key: b.id },
                    h("span", null,
                        b.track.artist,
                        " \u2014 ",
                        b.track.title),
                    h("span", { class: "muted" }, duration(b.track.duration_sec)),
                    h("audio", { controls: true, preload: "none", src: b.track.url }),
                    h("button", { class: "link", onClick: () => unsave(b, b.track.id) }, "\u0423\u0431\u0440\u0430\u0442\u044C"))) : null)));
}
export function DocumentsPage() {
    const list = useLoad(() => get("/documents?limit=50"), []);
    const [error, setError] = useState(null);
    const [busy, setBusy] = useState(false);
    async function add(files) {
        if (!files)
            return;
        setBusy(true);
        setError(null);
        for (const f of Array.from(files)) {
            try {
                await upload("/documents", f);
            }
            catch (e) {
                setError(e);
            }
        }
        setBusy(false);
        list.reload();
    }
    async function rename(d) {
        const title = prompt("Новое название", d.title);
        if (title === null || !title.trim())
            return;
        await patch(`/documents/${d.id}`, { title }).then(list.reload, setError);
    }
    async function remove(d) {
        if (!confirm(`Удалить «${d.title}»? Документ пропадёт из записей и сообщений.`))
            return;
        await del(`/documents/${d.id}`).then(list.reload, setError);
    }
    return (h("section", null,
        h("h1", null, "\u0414\u043E\u043A\u0443\u043C\u0435\u043D\u0442\u044B"),
        h("p", { class: "muted" }, "PDF, Word, Excel, PowerPoint, OpenDocument, EPUB, DjVu, RTF, TXT, CSV \u0438 \u0430\u0440\u0445\u0438\u0432\u044B \u2014 \u0434\u043E 50 \u041C\u0411. \u0421\u043A\u0430\u0447\u0430\u0442\u044C \u0434\u043E\u043A\u0443\u043C\u0435\u043D\u0442 \u043C\u043E\u0433\u0443\u0442 \u0442\u043E\u043B\u044C\u043A\u043E \u0442\u0435, \u043A\u043E\u043C\u0443 \u0432\u0438\u0434\u043D\u0430 \u0437\u0430\u043F\u0438\u0441\u044C \u0438\u043B\u0438 \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435 \u0441 \u043D\u0438\u043C."),
        h("label", { class: "ghost file-btn" },
            busy ? "Загружаем…" : "Загрузить документ",
            h("input", { type: "file", multiple: true, class: "sr-only", disabled: busy, onChange: (e) => add(e.target.files) })),
        h(ErrorLine, { error: error ?? list.error }),
        !list.data ? h(Loading, null) : list.data.items.length === 0 ? h(Empty, null, "\u0414\u043E\u043A\u0443\u043C\u0435\u043D\u0442\u043E\u0432 \u043F\u043E\u043A\u0430 \u043D\u0435\u0442.") : (h("ul", { class: "doc-list" }, list.data.items.map((d) => (h("li", { key: d.id, class: "doc" },
            h("span", { class: "doc-ext", "aria-hidden": "true" }, d.ext),
            h("button", { class: "link doc-name", onClick: () => saveFile(d.url, `${d.title}.${d.ext}`).catch(setError) },
                d.title,
                ".",
                d.ext),
            h("span", { class: "muted" },
                fileSize(d.size),
                " \u00B7 ",
                h(Time, { iso: d.created_at })),
            h("button", { class: "link", onClick: () => rename(d) }, "\u041F\u0435\u0440\u0435\u0438\u043C\u0435\u043D\u043E\u0432\u0430\u0442\u044C"),
            h("button", { class: "link", onClick: () => remove(d) }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C"))))))));
}
const localInput = (iso) => { const d = new Date(iso); return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16); };
export function ScheduledPage({ communityId }) {
    const list = useLoad(() => get("/scheduled-posts" + (communityId ? `?community_id=${communityId}` : "")), [communityId]);
    const [error, setError] = useState(null);
    const [editing, setEditing] = useState(null);
    const [when, setWhen] = useState("");
    async function save(s) {
        setError(null);
        try {
            await patch(`/scheduled-posts/${s.id}`, { publish_at: new Date(when).toISOString() });
            setEditing(null);
            list.reload();
        }
        catch (e) {
            setError(e);
        }
    }
    async function cancel(s) {
        if (!confirm("Отменить публикацию?"))
            return;
        await del(`/scheduled-posts/${s.id}`).then(list.reload, setError);
    }
    const summary = (s) => {
        const parts = [];
        if (s.payload.text)
            parts.push(s.payload.text.length > 140 ? s.payload.text.slice(0, 140) + "…" : s.payload.text);
        if (s.payload.poll)
            parts.push(`Опрос: ${s.payload.poll.question}`);
        if (s.payload.photo_ids?.length)
            parts.push(`фото: ${s.payload.photo_ids.length}`);
        if (s.payload.document_ids?.length)
            parts.push(`документов: ${s.payload.document_ids.length}`);
        return parts.join(" · ") || "Запись";
    };
    return (h("section", null,
        !communityId && h("h1", null, "\u041E\u0442\u043B\u043E\u0436\u0435\u043D\u043D\u044B\u0435 \u0437\u0430\u043F\u0438\u0441\u0438"),
        h(ErrorLine, { error: error ?? list.error }),
        !list.data ? h(Loading, null) : list.data.items.length === 0 ? h(Empty, null, "\u041D\u0435\u0442 \u0437\u0430\u043F\u043B\u0430\u043D\u0438\u0440\u043E\u0432\u0430\u043D\u043D\u044B\u0445 \u0437\u0430\u043F\u0438\u0441\u0435\u0439. \u0412 \u0440\u0435\u0434\u0430\u043A\u0442\u043E\u0440\u0435 \u0437\u0430\u043F\u0438\u0441\u0438 \u043D\u0430\u0436\u043C\u0438\u0442\u0435 \u00AB\u041E\u0442\u043B\u043E\u0436\u0438\u0442\u044C\u00BB.") : (h("ul", { class: "plain-list" }, list.data.items.map((s) => (h("li", { key: s.id, class: "scheduled" },
            h("span", null,
                h("strong", null, new Date(s.publish_at).toLocaleString("ru", { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" })),
                s.status === "failed" && h("span", { class: "error" },
                    " \u00B7 \u043D\u0435 \u043E\u043F\u0443\u0431\u043B\u0438\u043A\u043E\u0432\u0430\u043D\u043E: ",
                    s.error),
                h("br", null),
                h("span", { class: "muted" }, summary(s))),
            editing === s.id ? (h("span", { class: "row tight" },
                h("label", { class: "sr-only", for: `when-${s.id}` }, "\u041D\u043E\u0432\u043E\u0435 \u0432\u0440\u0435\u043C\u044F"),
                h("input", { id: `when-${s.id}`, type: "datetime-local", value: when, onInput: (e) => setWhen(e.target.value) }),
                h("button", { class: "primary", onClick: () => save(s) }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C"),
                h("button", { class: "link", onClick: () => setEditing(null) }, "\u041E\u0442\u043C\u0435\u043D\u0430"))) : (h("span", { class: "row tight" },
                h("button", { class: "ghost", onClick: () => { setEditing(s.id); setWhen(localInput(s.publish_at)); } }, s.status === "failed" ? "Повторить" : "Перенести"),
                h("button", { class: "link", onClick: () => cancel(s) }, "\u041E\u0442\u043C\u0435\u043D\u0438\u0442\u044C"))))))))));
}
