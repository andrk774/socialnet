import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { get, saveFile, upload } from "./api.js";
import { me } from "./session.js";
import { ErrorLine, Loading, useLoad } from "./ui.js";
import { MediaUpload } from "./uploads.js";
export const stickers = [["hello", "👋", "Привет!"], ["thanks", "🌟", "Спасибо!"], ["hug", "🤗", "Обнимаю"], ["wow", "🚀", "Вот это да!"], ["yes", "🐱", "Всё получится!"], ["sad", "🌧️", "Грустно"]];
export const emptyDraft = () => ({ photos: [], tracks: [], videos: [], documents: [], sticker: "" });
export function Sticker({ id }) { const s = stickers.find(s => s[0] === id); return s ? h("span", { class: `comment-sticker sticker-${s[0]}`, role: "img", "aria-label": s[2] },
    h("span", { "aria-hidden": "true" }, s[1]),
    h("strong", null, s[2])) : null; }
export function CommentMedia({ c }) {
    const [error, setError] = useState(null);
    return h("div", { class: "comment-media" },
        c.sticker && h(Sticker, { id: c.sticker }),
        !!c.photos?.length && h("div", { class: "comment-photo-grid" }, c.photos.map(p => h("a", { key: p.id, href: p.url, target: "_blank", rel: "noopener", "data-external": true },
            h("img", { src: p.thumb_url, alt: "\u0424\u043E\u0442\u043E \u0432 \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438", loading: "lazy" })))),
        c.tracks?.map(t => h("div", { key: t.id },
            h("p", null,
                t.artist,
                " \u2014 ",
                t.title),
            h("audio", { controls: true, preload: "none", src: t.url }))),
        c.videos?.map(v => h("div", { key: v.id },
            h("p", null, v.title),
            h("video", { controls: true, playsInline: true, preload: "none", poster: v.poster_url },
                h("source", { src: v.mp4_url || v.hls_url, type: v.mp4_url ? "video/mp4" : "application/vnd.apple.mpegurl" })))),
        c.documents?.map(d => h("button", { key: d.id, class: "link", onClick: () => saveFile(d.url, `${d.title}.${d.ext}`).catch(setError) },
            "\uD83D\uDCCE ",
            d.title,
            ".",
            d.ext)),
        h(ErrorLine, { error: error }));
}
export function CommentExtras({ value, onChange, disabled = false }) {
    const [kind, setKind] = useState(null), [error, setError] = useState(null), [busy, setBusy] = useState(false), [query, setQuery] = useState(""), [album, setAlbum] = useState("");
    const dialog = useRef(null);
    const total = value.photos.length + value.tracks.length + value.videos.length + value.documents.length;
    const urls = { photos: "/photos?limit=50", tracks: `/users/${me()?.id}/music?limit=50`, videos: `/users/${me()?.id}/videos?limit=50`, documents: "/documents?limit=50" };
    const library = useLoad(async () => { if (!kind || kind === "stickers")
        return { items: [], next_cursor: null }; const page = await get(urls[kind]); return { ...page, items: page.items ?? [] }; }, [kind]);
    const albums = useLoad(() => get("/photo-albums"), []);
    useEffect(() => { if (kind) {
        setQuery("");
        setAlbum("");
        dialog.current?.showModal();
    } }, [kind]);
    const close = () => { if (busy)
        return; dialog.current?.close(); setKind(null); setError(null); };
    function select(item) { if (!kind || kind === "stickers" || total >= 10)
        return; const current = value[kind]; if (current.some(x => x.id === item.id))
        return; onChange({ ...value, [kind]: [...current, item] }); }
    async function more() { if (!kind || kind === "stickers" || !library.data?.next_cursor || busy)
        return; setBusy(true); try {
        const old = library.data;
        const page = await get(urls[kind] + `&${kind === "photos" ? "after" : "before"}=${old.next_cursor}`);
        library.set({ ...page, items: [...old.items, ...page.items] });
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    async function add(file) { if (!file || busy || !kind || kind === "stickers")
        return; setBusy(true); setError(null); try {
        if (total >= 10)
            throw new Error("Можно прикрепить до 10 вложений");
        const item = await upload(kind === "photos" ? "/media" : "/documents", file);
        select(item);
        library.reload();
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    return h("div", { class: "comment-extras" },
        h("div", { class: "comment-attachment-toolbar", "aria-label": "\u0412\u043B\u043E\u0436\u0435\u043D\u0438\u044F \u043A \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u044E" }, [["photos", "Фото"], ["tracks", "Музыка"], ["videos", "Видео"], ["documents", "Файл"], ["stickers", "Стикеры"]].map(([id, label]) => h("button", { type: "button", class: "chip", disabled: disabled, onClick: () => setKind(id) }, label))),
        h("div", { class: "comment-draft" },
            ["photos", "tracks", "videos", "documents"].flatMap(k => value[k].map(x => h("span", { class: "chip", key: x.id },
                "thumb_url" in x ? h("img", { src: x.thumb_url, alt: "\u0412\u044B\u0431\u0440\u0430\u043D\u043D\u043E\u0435 \u0444\u043E\u0442\u043E" }) : "title" in x ? x.title : k,
                h("button", { type: "button", disabled: disabled, "aria-label": "\u0423\u0431\u0440\u0430\u0442\u044C \u0432\u043B\u043E\u0436\u0435\u043D\u0438\u0435", onClick: () => onChange({ ...value, [k]: value[k].filter(y => y.id !== x.id) }) }, "\u00D7")))),
            value.sticker && h("span", null,
                h(Sticker, { id: value.sticker }),
                h("button", { type: "button", disabled: disabled, onClick: () => onChange({ ...value, sticker: "" }) }, "\u0423\u0431\u0440\u0430\u0442\u044C \u0441\u0442\u0438\u043A\u0435\u0440"))),
        h("dialog", { class: "comment-attachment-dialog", ref: dialog, "aria-label": "\u0412\u043B\u043E\u0436\u0435\u043D\u0438\u044F \u043A \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u044E", onCancel: e => { e.preventDefault(); close(); } },
            h("header", null,
                h("h2", null, kind === "stickers" ? "Стикеры socialnet" : "Прикрепить к комментарию"),
                h("button", { type: "button", disabled: busy, "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C \u0432\u043B\u043E\u0436\u0435\u043D\u0438\u044F", onClick: close }, "\u00D7")),
            h(ErrorLine, { error: error || library.error }),
            kind === "stickers" ? h("div", { class: "comment-sticker-grid" }, stickers.map(([id, , label]) => h("button", { type: "button", key: id, "aria-label": label, onClick: () => { onChange({ ...value, sticker: id }); close(); } },
                h(Sticker, { id: id })))) : h(Fragment, null,
                h("input", { type: "search", "aria-label": "\u041F\u043E\u0438\u0441\u043A \u0432\u043B\u043E\u0436\u0435\u043D\u0438\u0439", placeholder: "\u041F\u043E\u0438\u0441\u043A \u043F\u043E \u0437\u0430\u0433\u0440\u0443\u0436\u0435\u043D\u043D\u044B\u043C \u0444\u0430\u0439\u043B\u0430\u043C", value: query, onInput: e => setQuery(e.currentTarget.value) }),
                kind === "photos" && h("label", null,
                    "\u0410\u043B\u044C\u0431\u043E\u043C",
                    h("select", { value: album, onChange: e => setAlbum(e.currentTarget.value) },
                        h("option", { value: "" }, "\u0412\u0441\u0435 \u0444\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0438"),
                        albums.data?.items?.map(a => h("option", { value: a.id }, a.title)))),
                (kind === "photos" || kind === "documents") && h("label", { class: "file-btn" },
                    "\u0417\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044C \u0441 \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0430",
                    h("input", { type: "file", "aria-label": "\u0424\u0430\u0439\u043B \u0434\u043B\u044F \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u044F", disabled: busy || total >= 10, accept: kind === "photos" ? "image/jpeg,image/png,image/gif" : undefined, onChange: e => { void add(e.currentTarget.files?.[0]); e.currentTarget.value = ""; } })),
                (kind === "tracks" || kind === "videos") && h("details", null,
                    h("summary", null, "\u0417\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044C \u0441 \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0430"),
                    h(MediaUpload, { kind: kind === "tracks" ? "music" : "video", onUploaded: library.reload })),
                kind === "videos" && h("p", { class: "muted" },
                    "\u0412\u044B\u0431\u0440\u0430\u0442\u044C \u043C\u043E\u0436\u043D\u043E \u043E\u0431\u0440\u0430\u0431\u043E\u0442\u0430\u043D\u043D\u044B\u0435 \u0432\u0438\u0434\u0435\u043E. ",
                    h("button", { type: "button", class: "link", onClick: library.reload }, "\u041E\u0431\u043D\u043E\u0432\u0438\u0442\u044C \u0441\u043F\u0438\u0441\u043E\u043A")),
                !library.data ? h(Loading, null) : h("div", { class: "comment-library" },
                    library.data.items.filter(x => (!("status" in x) || x.status === "ready") && (!album || x.album_id === Number(album)) && (!query || ("title" in x ? x.title : "").toLocaleLowerCase().includes(query.toLocaleLowerCase()))).map(x => h("button", { type: "button", key: x.id, disabled: total >= 10 || busy || (kind !== null && value[kind].some(y => y.id === x.id)), onClick: () => select(x) },
                        "thumb_url" in x ? h("img", { src: x.thumb_url, alt: "\u0424\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u044F" }) : "poster_url" in x ? h("img", { src: x.poster_url, alt: "" }) : null,
                        h("span", null, "title" in x ? x.title : "Фотография"))),
                    !library.data.items.length && h("p", null, "\u0412 \u044D\u0442\u043E\u0439 \u0431\u0438\u0431\u043B\u0438\u043E\u0442\u0435\u043A\u0435 \u043F\u043E\u043A\u0430 \u043D\u0438\u0447\u0435\u0433\u043E \u043D\u0435\u0442. \u0417\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u0435 \u0444\u0430\u0439\u043B \u0441 \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0430.")),
                library.data?.next_cursor && h("button", { type: "button", disabled: busy, onClick: more }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u0435\u0449\u0451"),
                h("p", { class: "muted" },
                    "\u0412\u044B\u0431\u0440\u0430\u043D\u043E ",
                    total,
                    " \u0438\u0437 10")),
            h("button", { type: "button", class: "primary", disabled: busy, onClick: close }, "\u0413\u043E\u0442\u043E\u0432\u043E")));
}
