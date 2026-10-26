import { h } from "../vendor/preact/index.js";
import { useRef, useState } from "../vendor/preact/hooks.js";
import { api, del, get, post, upload } from "./api.js";
import { me } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, RichText, Time, UserLink, duration, useLoad } from "./ui.js";
const reactions = [
    ["like", "👍", "Нравится"], ["love", "❤️", "Люблю"], ["haha", "😄", "Смешно"],
    ["wow", "😮", "Ого"], ["sad", "😢", "Грустно"], ["angry", "😠", "Возмутительно"],
];
export function Composer({ communityId, onPosted }) {
    const [text, setText] = useState("");
    const [photos, setPhotos] = useState([]);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    const file = useRef(null);
    async function addPhotos(files) {
        if (!files)
            return;
        setError(null);
        for (const f of Array.from(files).slice(0, 10 - photos.length)) {
            try {
                const p = await upload("/media", f);
                setPhotos((xs) => [...xs, p]);
            }
            catch (e) {
                setError(e);
            }
        }
    }
    async function submit(e) {
        e.preventDefault();
        setBusy(true);
        setError(null);
        try {
            const p = await post("/posts", { text, photo_ids: photos.map((x) => x.id), ...(communityId ? { community_id: communityId } : {}) });
            setText("");
            setPhotos([]);
            onPosted(p);
        }
        catch (err) {
            setError(err);
        }
        finally {
            setBusy(false);
        }
    }
    const empty = !text.trim() && photos.length === 0;
    return (h("form", { class: "composer", onSubmit: submit },
        h("label", { class: "sr-only", for: "composer-text" }, "\u0422\u0435\u043A\u0441\u0442 \u0437\u0430\u043F\u0438\u0441\u0438"),
        h("textarea", { id: "composer-text", placeholder: "\u0427\u0442\u043E \u0443 \u0432\u0430\u0441 \u043D\u043E\u0432\u043E\u0433\u043E?", value: text, maxLength: 10000, onInput: (e) => setText(e.target.value), rows: text.split("\n").length > 2 ? 5 : 2 }),
        photos.length > 0 && (h("ul", { class: "composer-photos" }, photos.map((p) => (h("li", { key: p.id },
            h("img", { src: p.thumb_url, alt: "" }),
            h("button", { type: "button", class: "icon-btn", "aria-label": "\u0423\u0431\u0440\u0430\u0442\u044C \u0444\u043E\u0442\u043E", onClick: () => setPhotos(photos.filter((x) => x !== p)) }, "\u00D7")))))),
        h(ErrorLine, { error: error }),
        h("div", { class: "composer-bar" },
            h("input", { ref: file, type: "file", accept: "image/jpeg,image/png,image/gif", multiple: true, hidden: true, onChange: (e) => addPhotos(e.target.files) }),
            h("button", { type: "button", class: "ghost", onClick: () => file.current?.click(), disabled: photos.length >= 10 }, "\u0424\u043E\u0442\u043E"),
            h("button", { type: "submit", class: "primary", disabled: busy || empty }, busy ? "Публикуем…" : "Опубликовать"))));
}
export function PostView({ initial, onDelete }) {
    const [p, setP] = useState(initial);
    const [open, setOpen] = useState(false);
    const [picking, setPicking] = useState(false);
    const mine = me()?.id === p.author.id;
    async function react(r) {
        setPicking(false);
        const res = r === null || r === p.my_reaction
            ? await api("DELETE", `/posts/${p.id}/reactions`)
            : await post(`/posts/${p.id}/reactions`, { reaction: r });
        setP(await get(`/posts/${p.id}`).catch(() => ({ ...p, likes_count: res.likes_count })));
    }
    async function remove() {
        if (!confirm("Удалить запись?"))
            return;
        await del(`/posts/${p.id}`);
        onDelete?.();
    }
    const top = Object.entries(p.reactions).filter(([, n]) => n > 0).sort((a, b) => b[1] - a[1]).slice(0, 3);
    const mineR = reactions.find(([r]) => r === p.my_reaction);
    return (h("article", { class: "post", "aria-labelledby": `post-${p.id}-author` },
        h("header", { class: "post-head" },
            h(Avatar, { user: p.author }),
            h("div", null,
                h("div", { id: `post-${p.id}-author` }, p.community ? h("a", { class: "user-link", href: `/c/${p.community.slug}` }, p.community.name) : h(UserLink, { user: p.author })),
                h("a", { class: "muted", href: `/p/${p.id}` },
                    h(Time, { iso: p.created_at }))),
            mine && h("button", { class: "icon-btn post-menu", "aria-label": "\u0423\u0434\u0430\u043B\u0438\u0442\u044C \u0437\u0430\u043F\u0438\u0441\u044C", onClick: remove }, "\u00D7")),
        p.text && h("p", { class: "post-text" },
            h(RichText, { text: p.text })),
        p.photos.length > 0 && (h("div", { class: `post-photos n${Math.min(p.photos.length, 4)}` }, p.photos.map((ph) => h("a", { key: ph.id, href: ph.url, target: "_blank", rel: "noopener", "data-external": true },
            h("img", { src: ph.thumb_url, alt: "", loading: "lazy" }))))),
        p.tracks.map((t) => (h("div", { class: "track", key: t.id },
            h("span", null,
                t.artist,
                " \u2014 ",
                t.title),
            h("span", { class: "muted" }, duration(t.duration_sec)),
            h("audio", { controls: true, preload: "none", src: t.url })))),
        p.videos.map((v) => h(Video, { key: v.id, src: v.hls_url, mp4: v.mp4_url, poster: v.poster_url, title: v.title })),
        h("footer", { class: "post-actions" },
            h("div", { class: "react-wrap" },
                h("button", { class: p.my_reaction ? "chip active" : "chip", "aria-pressed": !!p.my_reaction, onClick: () => (p.my_reaction ? react(null) : react("like")), onContextMenu: (e) => { e.preventDefault(); setPicking(!picking); } },
                    mineR ? mineR[1] : "👍",
                    " ",
                    p.likes_count > 0 ? p.likes_count : "",
                    h("span", { class: "sr-only" }, mineR ? `Ваша реакция: ${mineR[2]}. Убрать` : "Нравится")),
                h("button", { class: "chip", "aria-expanded": picking, "aria-label": "\u0412\u044B\u0431\u0440\u0430\u0442\u044C \u0440\u0435\u0430\u043A\u0446\u0438\u044E", onClick: () => setPicking(!picking) }, "\u2026"),
                picking && (h("div", { class: "react-picker", role: "menu" }, reactions.map(([r, emoji, label]) => h("button", { role: "menuitem", key: r, title: label, "aria-label": label, onClick: () => react(r) }, emoji)))),
                top.length > 1 && h("span", { class: "muted react-top" }, top.map(([r]) => reactions.find((x) => x[0] === r)[1]).join(""))),
            h("button", { class: "chip", "aria-expanded": open, onClick: () => setOpen(!open) },
                "\u041A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438",
                p.comments_count > 0 ? ` ${p.comments_count}` : "")),
        open && h(Comments, { post: p, onCount: (n) => setP({ ...p, comments_count: n }) })));
}
export function Video({ src, mp4, poster, title }) {
    // Браузер берёт первый источник, который умеет играть: HLS (Safari, iOS,
    // Android — с подстройкой качества под сеть) или MP4 (остальные).
    const hls = document.createElement("video").canPlayType("application/vnd.apple.mpegurl") !== "";
    return (h("figure", { class: "video" },
        hls || mp4 ? (h("video", { controls: true, preload: "none", poster: poster, playsInline: true },
            hls && h("source", { src: src, type: "application/vnd.apple.mpegurl" }),
            mp4 && h("source", { src: mp4, type: "video/mp4" }))) : (h("div", { class: "video-fallback", style: { backgroundImage: `url("${poster}")` } },
            h("p", null, "\u0412\u0438\u0434\u0435\u043E \u0435\u0449\u0451 \u0433\u043E\u0442\u043E\u0432\u0438\u0442\u0441\u044F \u0434\u043B\u044F \u044D\u0442\u043E\u0433\u043E \u0431\u0440\u0430\u0443\u0437\u0435\u0440\u0430. \u041E\u0442\u043A\u0440\u043E\u0439\u0442\u0435 \u0435\u0433\u043E \u043D\u0430 \u0442\u0435\u043B\u0435\u0444\u043E\u043D\u0435 \u0438\u043B\u0438 \u0432 Safari."))),
        title && h("figcaption", null, title)));
}
function Comments({ post: p, onCount }) {
    const roots = useLoad(() => get(`/posts/${p.id}/comments?limit=50`), [p.id]);
    const [replyTo, setReplyTo] = useState(null);
    const [text, setText] = useState("");
    const [error, setError] = useState(null);
    const [expanded, setExpanded] = useState({});
    async function loadReplies(c) {
        const r = await get(`/comments/${c.id}/replies?limit=50`);
        setExpanded((x) => ({ ...x, [c.id]: r.items }));
    }
    async function send(e) {
        e.preventDefault();
        setError(null);
        try {
            await post(`/posts/${p.id}/comments`, { text, ...(replyTo ? { reply_to_id: replyTo.id } : {}) });
            setText("");
            const root = replyTo ? (replyTo.parent_id ?? replyTo.id) : null;
            setReplyTo(null);
            onCount(p.comments_count + 1);
            if (root) {
                const c = roots.data?.items.find((x) => x.id === root);
                if (c)
                    await loadReplies(c);
            }
            roots.reload();
        }
        catch (err) {
            setError(err);
        }
    }
    async function remove(c) {
        await del(`/comments/${c.id}`);
        roots.reload();
        if (c.parent_id)
            setExpanded((x) => ({ ...x, [c.parent_id]: (x[c.parent_id] ?? []).filter((r) => r.id !== c.id) }));
    }
    const one = (c) => (h("li", { key: c.id, class: "comment" },
        h(Avatar, { user: c.author, size: 28 }),
        h("div", null,
            h(UserLink, { user: c.author }),
            c.reply_to && h("span", { class: "muted" },
                " \u043E\u0442\u0432\u0435\u0442 ",
                c.reply_to.first_name),
            h("p", null,
                h(RichText, { text: c.text })),
            h("div", { class: "comment-meta" },
                h(Time, { iso: c.created_at }),
                h("button", { class: "link", onClick: () => { setReplyTo(c); setText(`${c.author.first_name}, `); } }, "\u041E\u0442\u0432\u0435\u0442\u0438\u0442\u044C"),
                me()?.id === c.author.id && h("button", { class: "link", onClick: () => remove(c) }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C")),
            !c.parent_id && c.replies_count > 0 && !expanded[c.id] && (h("button", { class: "link", onClick: () => loadReplies(c) },
                "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u043E\u0442\u0432\u0435\u0442\u044B: ",
                c.replies_count)),
            expanded[c.id] && h("ul", { class: "replies" }, expanded[c.id].map(one)))));
    return (h("section", { class: "comments", "aria-label": "\u041A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438" },
        roots.error ? h(ErrorLine, { error: roots.error }) : !roots.data ? h(Loading, null) : h("ul", null, roots.data.items.map(one)),
        h("form", { class: "comment-form", onSubmit: send },
            replyTo && h("p", { class: "muted" },
                "\u041E\u0442\u0432\u0435\u0442 \u0434\u043B\u044F ",
                replyTo.author.first_name,
                " ",
                h("button", { type: "button", class: "link", onClick: () => setReplyTo(null) }, "\u043E\u0442\u043C\u0435\u043D\u0438\u0442\u044C")),
            h("label", { class: "sr-only", for: `c-${p.id}` }, "\u041A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0439"),
            h("input", { id: `c-${p.id}`, value: text, maxLength: 2000, placeholder: "\u041D\u0430\u043F\u0438\u0441\u0430\u0442\u044C \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0439", onInput: (e) => setText(e.target.value) }),
            h("button", { class: "primary", disabled: !text.trim() }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C")),
        h(ErrorLine, { error: error })));
}
// Лента с подгрузкой по курсору.
export function PostList({ path, empty, header }) {
    const [items, setItems] = useState(null);
    const [cursor, setCursor] = useState(null);
    const [error, setError] = useState(null);
    const [busy, setBusy] = useState(false);
    const [key, setKey] = useState(path);
    async function load(before) {
        setBusy(true);
        try {
            const sep = path.includes("?") ? "&" : "?";
            const r = await get(path + (before ? `${sep}before=${before}` : ""));
            setItems((xs) => (before && xs ? [...xs, ...r.items] : r.items));
            setCursor(r.next_cursor);
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    }
    if (key !== path) {
        setKey(path);
        setItems(null);
    }
    if (items === null && !busy && !error)
        load();
    const add = (p) => setItems((xs) => [p, ...(xs ?? [])]);
    return (h("div", { class: "post-list" },
        header?.(add),
        h(ErrorLine, { error: error }),
        items === null ? h(Loading, null) : items.length === 0 ? h(Empty, null, empty) :
            items.map((p) => h(PostView, { key: p.id, initial: p, onDelete: () => setItems(items.filter((x) => x.id !== p.id)) })),
        cursor && h("button", { class: "ghost more", disabled: busy, onClick: () => load(cursor) }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u0435\u0449\u0451")));
}
