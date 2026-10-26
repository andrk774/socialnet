import { h, Fragment } from "../vendor/preact/index.js";
import { useRef, useState } from "../vendor/preact/hooks.js";
import { api, del, fileSize, fullName, get, patch, post, put, saveFile, upload } from "./api.js";
import { me } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, RichText, Time, UserLink, duration, useLoad } from "./ui.js";
const reactions = [
    ["like", "👍", "Нравится"], ["love", "❤️", "Люблю"], ["haha", "😄", "Смешно"],
    ["wow", "😮", "Ого"], ["sad", "😢", "Грустно"], ["angry", "😠", "Возмутительно"],
];
const audienceLabel = { everyone: "Все", friends: "Друзья", only_me: "Только я", list: "Выбранные друзья" };
// localInput: значение для <input type="datetime-local"> в часовом поясе браузера.
const localInput = (d) => new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
export function Composer({ communityId, onPosted }) {
    const [text, setText] = useState("");
    const [photos, setPhotos] = useState([]);
    const [docs, setDocs] = useState([]);
    const [poll, setPoll] = useState(null);
    const [audience, setAudience] = useState("everyone");
    const [picked, setPicked] = useState([]);
    const [when, setWhen] = useState(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    const [note, setNote] = useState("");
    const file = useRef(null);
    const docFile = useRef(null);
    const friends = useLoad(() => (audience === "list" ? get(`/users/${me().id}/friends?limit=50`) : Promise.resolve(null)), [audience]);
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
    async function addDocs(files) {
        if (!files)
            return;
        setError(null);
        for (const f of Array.from(files).slice(0, 10 - docs.length)) {
            try {
                const d = await upload("/documents", f);
                setDocs((xs) => [...xs, d]);
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
        setNote("");
        const body = { text, photo_ids: photos.map((x) => x.id), document_ids: docs.map((x) => x.id) };
        if (communityId)
            body.community_id = communityId;
        else {
            body.audience = audience;
            if (audience === "list")
                body.audience_ids = picked;
        }
        if (poll)
            body.poll = { ...poll, options: poll.options.filter((o) => o.trim()) };
        if (when)
            body.publish_at = new Date(when).toISOString();
        try {
            const r = await post("/posts", body);
            setText("");
            setPhotos([]);
            setDocs([]);
            setPoll(null);
            setWhen(null);
            if (when)
                setNote(`Запись выйдет ${new Date(r.publish_at).toLocaleString("ru", { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" })}. Её можно перенести в разделе «Отложенные».`);
            else
                onPosted(r);
        }
        catch (err) {
            setError(err);
        }
        finally {
            setBusy(false);
        }
    }
    const setOpt = (i, v) => setPoll(poll && { ...poll, options: poll.options.map((o, j) => (j === i ? v : o)) });
    const empty = !text.trim() && photos.length === 0 && docs.length === 0 && !poll;
    return (h("form", { class: "composer", onSubmit: submit },
        h("label", { class: "sr-only", for: "composer-text" }, "\u0422\u0435\u043A\u0441\u0442 \u0437\u0430\u043F\u0438\u0441\u0438"),
        h("textarea", { id: "composer-text", placeholder: "\u0427\u0442\u043E \u0443 \u0432\u0430\u0441 \u043D\u043E\u0432\u043E\u0433\u043E?", value: text, maxLength: 10000, onInput: (e) => setText(e.target.value), rows: text.split("\n").length > 2 ? 5 : 2 }),
        photos.length > 0 && (h("ul", { class: "composer-photos" }, photos.map((p) => (h("li", { key: p.id },
            h("img", { src: p.thumb_url, alt: "" }),
            h("button", { type: "button", class: "icon-btn", "aria-label": "\u0423\u0431\u0440\u0430\u0442\u044C \u0444\u043E\u0442\u043E", onClick: () => setPhotos(photos.filter((x) => x !== p)) }, "\u00D7")))))),
        docs.length > 0 && (h("ul", { class: "doc-list" }, docs.map((d) => (h("li", { key: d.id, class: "doc" },
            h("span", { class: "doc-ext", "aria-hidden": "true" }, d.ext),
            h("span", { class: "doc-name" },
                d.title,
                h("span", { class: "muted" },
                    " \u00B7 ",
                    fileSize(d.size))),
            h("button", { type: "button", class: "icon-btn", "aria-label": `Убрать документ ${d.title}`, onClick: () => setDocs(docs.filter((x) => x !== d)) }, "\u00D7")))))),
        poll && (h("fieldset", { class: "poll-edit" },
            h("legend", null, "\u041E\u043F\u0440\u043E\u0441"),
            h("label", { class: "sr-only", for: "poll-q" }, "\u0412\u043E\u043F\u0440\u043E\u0441"),
            h("input", { id: "poll-q", placeholder: "\u0412\u043E\u043F\u0440\u043E\u0441", maxLength: 255, value: poll.question, onInput: (e) => setPoll({ ...poll, question: e.target.value }) }),
            poll.options.map((o, i) => (h("div", { class: "row tight", key: i },
                h("label", { class: "sr-only", for: `poll-o${i}` },
                    "\u0412\u0430\u0440\u0438\u0430\u043D\u0442 ",
                    i + 1),
                h("input", { id: `poll-o${i}`, placeholder: `Вариант ${i + 1}`, maxLength: 100, value: o, onInput: (e) => setOpt(i, e.target.value) }),
                poll.options.length > 2 && h("button", { type: "button", class: "icon-btn", "aria-label": `Убрать вариант ${i + 1}`, onClick: () => setPoll({ ...poll, options: poll.options.filter((_, j) => j !== i) }) }, "\u00D7")))),
            h("div", { class: "row tight" },
                poll.options.length < 10 && h("button", { type: "button", class: "link", onClick: () => setPoll({ ...poll, options: [...poll.options, ""] }) }, "\u0414\u043E\u0431\u0430\u0432\u0438\u0442\u044C \u0432\u0430\u0440\u0438\u0430\u043D\u0442"),
                h("label", null,
                    h("input", { type: "checkbox", checked: poll.multiple, onChange: (e) => setPoll({ ...poll, multiple: e.target.checked }) }),
                    " \u041D\u0435\u0441\u043A\u043E\u043B\u044C\u043A\u043E \u043E\u0442\u0432\u0435\u0442\u043E\u0432"),
                h("label", null,
                    h("input", { type: "checkbox", checked: poll.anonymous, onChange: (e) => setPoll({ ...poll, anonymous: e.target.checked }) }),
                    " \u0410\u043D\u043E\u043D\u0438\u043C\u043D\u044B\u0439"),
                h("button", { type: "button", class: "link", onClick: () => setPoll(null) }, "\u0423\u0431\u0440\u0430\u0442\u044C \u043E\u043F\u0440\u043E\u0441")))),
        audience === "list" && !communityId && (h("fieldset", { class: "audience-pick" },
            h("legend", null, "\u041A\u043E\u043C\u0443 \u043F\u043E\u043A\u0430\u0437\u0430\u0442\u044C"),
            !friends.data ? h(Loading, null) : friends.data.items.length === 0 ? h("p", { class: "muted" }, "\u0414\u0440\u0443\u0437\u0435\u0439 \u043F\u043E\u043A\u0430 \u043D\u0435\u0442.") :
                friends.data.items.map((f) => (h("label", { key: f.id },
                    h("input", { type: "checkbox", checked: picked.includes(f.id), onChange: (e) => setPicked(e.target.checked ? [...picked, f.id] : picked.filter((x) => x !== f.id)) }),
                    " ",
                    fullName(f)))))),
        when !== null && (h("div", { class: "row tight" },
            h("label", { for: "publish-at" }, "\u041E\u043F\u0443\u0431\u043B\u0438\u043A\u043E\u0432\u0430\u0442\u044C"),
            h("input", { id: "publish-at", type: "datetime-local", value: when, min: localInput(new Date()), onInput: (e) => setWhen(e.target.value) }),
            h("button", { type: "button", class: "link", onClick: () => setWhen(null) }, "\u0421\u0435\u0439\u0447\u0430\u0441"))),
        h(ErrorLine, { error: error }),
        note && h("p", { class: "note", role: "status" }, note),
        h("div", { class: "composer-bar" },
            h("div", { class: "composer-tools" },
                h("input", { ref: file, type: "file", accept: "image/jpeg,image/png,image/gif", multiple: true, hidden: true, onChange: (e) => addPhotos(e.target.files) }),
                h("input", { ref: docFile, type: "file", multiple: true, hidden: true, onChange: (e) => addDocs(e.target.files) }),
                h("button", { type: "button", class: "ghost", onClick: () => file.current?.click(), disabled: photos.length >= 10 }, "\u0424\u043E\u0442\u043E"),
                h("button", { type: "button", class: "ghost", onClick: () => docFile.current?.click(), disabled: docs.length >= 10 }, "\u0414\u043E\u043A\u0443\u043C\u0435\u043D\u0442"),
                h("button", { type: "button", class: "ghost", "aria-pressed": !!poll, onClick: () => setPoll(poll ? null : { question: "", options: ["", ""], multiple: false, anonymous: false }) }, "\u041E\u043F\u0440\u043E\u0441"),
                h("button", { type: "button", class: "ghost", "aria-pressed": when !== null, onClick: () => setWhen(when === null ? localInput(new Date(Date.now() + 3600_000)) : null) }, "\u041E\u0442\u043B\u043E\u0436\u0438\u0442\u044C"),
                !communityId && (h("select", { "aria-label": "\u041A\u0442\u043E \u0443\u0432\u0438\u0434\u0438\u0442 \u0437\u0430\u043F\u0438\u0441\u044C", value: audience, onChange: (e) => setAudience(e.target.value) }, Object.keys(audienceLabel).map((a) => h("option", { key: a, value: a }, audienceLabel[a]))))),
            h("button", { type: "submit", class: "primary", disabled: busy || empty || (audience === "list" && picked.length === 0 && !communityId) }, busy ? "Публикуем…" : when ? "Запланировать" : "Опубликовать"))));
}
export function DocList({ docs }) {
    const [error, setError] = useState(null);
    return (h(Fragment, null,
        h("ul", { class: "doc-list" }, docs.map((d) => (h("li", { key: d.id, class: "doc" },
            h("span", { class: "doc-ext", "aria-hidden": "true" }, d.ext),
            h("button", { class: "link doc-name", onClick: () => saveFile(d.url, `${d.title}.${d.ext}`).catch(setError) },
                d.title,
                ".",
                d.ext),
            h("span", { class: "muted" }, fileSize(d.size)))))),
        h(ErrorLine, { error: error })));
}
function PollView({ postId, initial }) {
    const [poll, setPoll] = useState(initial);
    const [chosen, setChosen] = useState([]);
    const [error, setError] = useState(null);
    const voted = poll.my_votes.length > 0;
    const canVote = !poll.closed && !voted;
    async function vote(ids) {
        setError(null);
        try {
            setPoll(await post(`/posts/${postId}/poll/votes`, { option_ids: ids }));
            setChosen([]);
        }
        catch (e) {
            setError(e);
        }
    }
    async function retract() {
        setError(null);
        try {
            setPoll(await del(`/posts/${postId}/poll/votes`));
        }
        catch (e) {
            setError(e);
        }
    }
    const total = poll.voters_count;
    return (h("fieldset", { class: "poll" },
        h("legend", null, poll.question),
        h("p", { class: "muted" },
            poll.anonymous ? "Анонимный опрос" : "Публичный опрос",
            poll.multiple && " · несколько ответов",
            poll.closed ? " · завершён" : poll.closes_at ? ` · до ${new Date(poll.closes_at).toLocaleString("ru", { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" })}` : ""),
        h("ul", { class: "poll-options" }, poll.options.map((o) => {
            const pct = o.votes !== null && total > 0 ? Math.round((o.votes / total) * 100) : 0;
            const mine = poll.my_votes.includes(o.id);
            return (h("li", { key: o.id }, canVote ? (poll.multiple ? (h("label", { class: "poll-choice" },
                h("input", { type: "checkbox", checked: chosen.includes(o.id), onChange: (e) => setChosen(e.target.checked ? [...chosen, o.id] : chosen.filter((x) => x !== o.id)) }),
                " ",
                o.text)) : h("button", { class: "poll-choice", onClick: () => vote([o.id]) }, o.text)) : (h("div", { class: mine ? "poll-result mine" : "poll-result" },
                h("span", { class: "poll-bar", style: { width: `${pct}%` }, "aria-hidden": "true" }),
                h("span", { class: "poll-text" },
                    o.text,
                    mine && h("span", { class: "sr-only" }, " (\u0432\u0430\u0448 \u0432\u044B\u0431\u043E\u0440)"),
                    mine && " ✓"),
                o.votes !== null && h("span", { class: "poll-pct" },
                    pct,
                    "%")))));
        })),
        h("div", { class: "row tight" },
            h("span", { class: "muted" }, total === 0 ? "Пока никто не голосовал" : `Проголосовали: ${total}`),
            canVote && poll.multiple && h("button", { class: "primary", disabled: chosen.length === 0, onClick: () => vote(chosen) }, "\u0413\u043E\u043B\u043E\u0441\u043E\u0432\u0430\u0442\u044C"),
            voted && !poll.closed && h("button", { class: "link", onClick: retract }, "\u041E\u0442\u043C\u0435\u043D\u0438\u0442\u044C \u0433\u043E\u043B\u043E\u0441")),
        h(ErrorLine, { error: error })));
}
export function PostView({ initial, onDelete }) {
    const [p, setP] = useState(initial);
    const [open, setOpen] = useState(false);
    const [picking, setPicking] = useState(false);
    const mine = me()?.id === p.author.id;
    const [error, setError] = useState(null);
    // любое действие с записью: ошибка показывается под записью, а состояние
    // меняется только после ответа сервера — интерфейс не врёт о несохранённом
    const act = async (fn) => {
        setError(null);
        try {
            await fn();
        }
        catch (e) {
            setError(e);
        }
    };
    const react = (r) => act(async () => {
        setPicking(false);
        const res = r === null || r === p.my_reaction
            ? await api("DELETE", `/posts/${p.id}/reactions`)
            : await post(`/posts/${p.id}/reactions`, { reaction: r });
        setP(await get(`/posts/${p.id}`).catch(() => ({ ...p, likes_count: res.likes_count })));
    });
    const remove = () => act(async () => {
        if (!confirm("Удалить запись?"))
            return;
        await del(`/posts/${p.id}`);
        onDelete?.();
    });
    const bookmark = () => act(async () => {
        if (p.bookmarked)
            await del(`/bookmarks/post/${p.id}`);
        else
            await put(`/bookmarks/post/${p.id}`);
        setP({ ...p, bookmarked: !p.bookmarked });
    });
    const changeAudience = (a, select) => act(async () => {
        select.value = p.audience; // пока сервер не подтвердил, показываем сохранённое значение
        if (a === "list") {
            alert("Выбрать отдельных друзей можно при создании записи.");
            return;
        }
        setP(await patch(`/posts/${p.id}`, { audience: a }));
    });
    const top = Object.entries(p.reactions).filter(([, n]) => n > 0).sort((a, b) => b[1] - a[1]).slice(0, 3);
    const mineR = reactions.find(([r]) => r === p.my_reaction);
    return (h("article", { class: "post", "aria-labelledby": `post-${p.id}-author` },
        h("header", { class: "post-head" },
            h(Avatar, { user: p.author }),
            h("div", null,
                h("div", { id: `post-${p.id}-author` }, p.community ? h("a", { class: "user-link", href: `/c/${p.community.slug}` }, p.community.name) : h(UserLink, { user: p.author })),
                h("a", { class: "muted", href: `/p/${p.id}` },
                    h(Time, { iso: p.created_at })),
                !p.community && p.audience !== "everyone" && h("span", { class: "muted" },
                    " \u00B7 \u0432\u0438\u0434\u044F\u0442: ",
                    audienceLabel[p.audience].toLowerCase())),
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
        p.documents.length > 0 && h(DocList, { docs: p.documents }),
        p.poll && h(PollView, { postId: p.id, initial: p.poll }),
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
                p.comments_count > 0 ? ` ${p.comments_count}` : ""),
            h("button", { class: p.bookmarked ? "chip active" : "chip", "aria-pressed": p.bookmarked, onClick: bookmark }, p.bookmarked ? "В закладках" : "В закладки"),
            mine && !p.community && (h("select", { class: "chip-select", "aria-label": "\u041A\u0442\u043E \u0432\u0438\u0434\u0438\u0442 \u0437\u0430\u043F\u0438\u0441\u044C", value: p.audience, onChange: (e) => { const el = e.target; changeAudience(el.value, el); } }, Object.keys(audienceLabel).filter((a) => a !== "list" || p.audience === "list").map((a) => h("option", { key: a, value: a }, audienceLabel[a]))))),
        h(ErrorLine, { error: error }),
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
