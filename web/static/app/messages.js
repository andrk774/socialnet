import { AIText } from "./ai.js";
import { CustomStickers } from "./customstickers.js";
import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { fetchBlob, fullName, get, post, put, upload } from "./api.js";
import { DocList } from "./posts.js";
import { startCall } from "./calls.js";
import { on, sendTyping } from "./realtime.js";
import { navigate } from "./router.js";
import { me } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, Time, duration, useLoad } from "./ui.js";
const title = (c) => (c.kind === "group" ? c.title || "Беседа" : c.peer ? fullName(c.peer) : "Диалог");
function MessageField({ value, onInput }) {
    const field = useRef(null);
    useEffect(() => {
        const el = field.current;
        if (el) {
            el.style.height = "auto";
            el.style.height = `${Math.min(el.scrollHeight, 144)}px`;
        }
    }, [value]);
    return h("textarea", { ref: field, rows: 1, value: value, onInput: onInput, placeholder: "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435", "aria-label": "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435", title: "Enter \u2014 \u043E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C, Shift+Enter \u2014 \u043D\u043E\u0432\u0430\u044F \u0441\u0442\u0440\u043E\u043A\u0430", maxLength: 4000, onKeyDown: (e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
                e.preventDefault();
                e.currentTarget.form?.requestSubmit();
            }
        } });
}
export function ConversationsPage() {
    const list = useLoad(() => get("/conversations"), []);
    useEffect(() => on("message", () => list.reload()), []);
    if (list.error)
        return h(ErrorLine, { error: list.error });
    if (!list.data)
        return h(Loading, null);
    return (h("section", null,
        h("h1", null, "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F"),
        list.data.items.length === 0 ? h(Empty, null, "\u0414\u0438\u0430\u043B\u043E\u0433\u043E\u0432 \u043F\u043E\u043A\u0430 \u043D\u0435\u0442. \u041D\u0430\u043F\u0438\u0448\u0438\u0442\u0435 \u043A\u043E\u043C\u0443-\u043D\u0438\u0431\u0443\u0434\u044C \u0438\u0437 \u0434\u0440\u0443\u0437\u0435\u0439.") : (h("ul", { class: "conv-list" }, list.data.items.map((c) => (h("li", { key: c.id },
            h("a", { href: `/im/${c.id}`, class: c.unread ? "conv unread" : "conv" },
                c.peer ? h(Avatar, { user: c.peer }) : h("span", { class: "avatar avatar-letter group", "aria-hidden": "true" }, c.members_count),
                h("span", { class: "conv-main" },
                    h("span", { class: "conv-title" }, title(c)),
                    h("span", { class: "muted conv-last" }, preview(c.last_message))),
                h("span", { class: "conv-side" },
                    c.last_message && h(Time, { iso: c.last_message.created_at }),
                    c.unread > 0 && h("span", { class: "badge", "aria-label": `непрочитанных: ${c.unread}` }, c.unread))))))))));
}
function preview(m) {
    if (!m)
        return "";
    if (m.encrypted)
        return "Зашифрованное сообщение";
    if (m.body)
        return m.body;
    return m.attachments.map((a) => ({ photo: "Фото", track: "Трек", voice: "Голосовое", document: "Документ" })[a.kind]).join(", ");
}
// /im/u/:id — открыть (или создать) личный диалог с пользователем
export function DirectPage({ userId }) {
    const sending = useRef(false);
    const [error, setError] = useState(null);
    const [text, setText] = useState("");
    const peer = useLoad(() => get(`/users/${userId}`), [userId]);
    async function send(e) {
        e.preventDefault();
        if (sending.current || !text.trim())
            return;
        sending.current = true;
        try {
            const m = await post(`/users/${userId}/messages`, { body: text });
            navigate(`/im/${m.conversation_id}`, true);
        }
        catch (err) {
            setError(err);
        }
        finally {
            sending.current = false;
        }
    }
    return (h("section", { class: "chat" },
        h("h1", null, peer.data ? fullName(peer.data) : "Новый диалог"),
        h(Empty, null, "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0439 \u0435\u0449\u0451 \u043D\u0435\u0442."),
        h("form", { class: "chat-form", onSubmit: send },
            h(MessageField, { value: text, onInput: (e) => setText(e.target.value) }),
            h("button", { class: "primary", disabled: !text.trim() }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C")),
        h(ErrorLine, { error: error })));
}
export function ChatPage({ id }) {
    const sending = useRef(false);
    const [msgs, setMsgs] = useState(null);
    const [conv, setConv] = useState(null);
    const [text, setText] = useState("");
    const [error, setError] = useState(null);
    const [typing, setTyping] = useState(false);
    const end = useRef(null);
    const lastTyping = useRef(0);
    const docInput = useRef(null);
    const [doc, setDoc] = useState(null);
    const [sticker, setSticker] = useState(null);
    const [showStickers, setShowStickers] = useState(false);
    const [pinned, setPinned] = useState(null);
    async function loadPin() { const r = await get(`/conversations/${id}/pin`); setPinned(r.items[0] ?? null); }
    async function pin(message_id) { try {
        await put(`/conversations/${id}/pin`, { message_id });
        await loadPin();
    }
    catch (e) {
        setError(e);
    } }
    useEffect(() => { setPinned(null); void loadPin().catch(setError); return on("conversation_pin", (d) => { if (d.conversation_id === id)
        void loadPin().catch(setError); }); }, [id]);
    useEffect(() => {
        setMsgs(null);
        get(`/conversations/${id}/messages?limit=50`).then((r) => setMsgs(r.items.slice().reverse()), setError);
        get("/conversations").then((r) => setConv(r.items.find((c) => c.id === id) ?? null), () => { });
        let t;
        const offMsg = on("message", (m) => {
            if (m.conversation_id !== id)
                return;
            setMsgs((xs) => (xs && !xs.some((x) => x.id === m.id) ? [...xs, m] : xs));
            if (m.sender_id !== me()?.id) {
                setTyping(false);
                clearTimeout(t);
            } // дописал — уже не печатает
        });
        const offTyping = on("typing", (d) => {
            if (d.conversation_id !== id)
                return;
            setTyping(true);
            clearTimeout(t);
            t = window.setTimeout(() => setTyping(false), 4000);
        });
        return () => { offMsg(); offTyping(); clearTimeout(t); };
    }, [id]);
    useEffect(() => {
        end.current?.scrollIntoView({ block: "end" });
        const last = msgs?.[msgs.length - 1];
        if (last && last.sender_id !== me()?.id)
            post(`/conversations/${id}/read`, { up_to: last.id }).catch(() => { });
    }, [msgs?.length]);
    async function send(e) {
        e.preventDefault();
        if (sending.current || (!text.trim() && !doc && !sticker))
            return;
        sending.current = true;
        setError(null);
        try {
            const m = await post(`/conversations/${id}/messages`, { body: text, attachments: [...(doc ? [{ kind: "document", id: doc.id }] : []), ...(sticker ? [{ kind: "photo", id: sticker.id }] : [])] });
            setText("");
            setDoc(null);
            setSticker(null);
            setMsgs((xs) => (xs && !xs.some((x) => x.id === m.id) ? [...xs, m] : xs));
        }
        catch (err) {
            setError(err);
        }
        finally {
            sending.current = false;
        }
    }
    function input(e) {
        setText(e.target.value);
        if (Date.now() - lastTyping.current > 3000) {
            lastTyping.current = Date.now();
            sendTyping(id);
        }
    }
    const self = me()?.id;
    return (h("section", { class: "chat" },
        h("h1", null,
            h("a", { href: "/im", class: "back", "aria-label": "\u0412\u0441\u0435 \u0434\u0438\u0430\u043B\u043E\u0433\u0438" }, "\u2039"),
            " ",
            conv ? title(conv) : "Диалог"),
        conv?.peer && h("div", { class: "chat-call-actions" },
            h("button", { class: "ghost", onClick: () => startCall(conv.peer) }, "\u041F\u043E\u0437\u0432\u043E\u043D\u0438\u0442\u044C"),
            h("button", { class: "ghost", onClick: () => startCall(conv.peer, true) }, "\u0412\u0438\u0434\u0435\u043E\u0437\u0432\u043E\u043D\u043E\u043A")),
        conv?.event_id && h("p", { class: "muted" },
            "\u0427\u0430\u0442 \u043F\u043E\u0445\u043E\u0434\u0430: \u0432 \u043D\u0451\u043C \u0432\u0441\u0435, \u043A\u0442\u043E \u0438\u0434\u0451\u0442. ",
            h("a", { href: `/events/${conv.event_id}` }, "\u0421\u0442\u0440\u0430\u043D\u0438\u0446\u0430 \u043F\u043E\u0445\u043E\u0434\u0430")),
        h(ErrorLine, { error: error }),
        pinned && h("aside", { class: "music-panel" },
            h("strong", null, "\u0417\u0430\u043A\u0440\u0435\u043F\u043B\u0451\u043D\u043D\u043E\u0435 \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435"),
            h("p", null, preview(pinned)),
            pinned.attachments.filter(a => a.kind === "photo").map(a => h("img", { src: a.thumb_url ?? a.url, alt: "", style: "max-height:100px" })),
            h("button", { class: "link", onClick: () => document.getElementById(`message-${pinned.id}`)?.scrollIntoView({ block: "center" }) }, "\u041F\u0435\u0440\u0435\u0439\u0442\u0438"),
            h("button", { class: "link", onClick: () => pin(0) }, "\u041E\u0442\u043A\u0440\u0435\u043F\u0438\u0442\u044C")),
        h("div", { class: "chat-log", role: "log", "aria-live": "polite" },
            msgs === null ? h(Loading, null) : msgs.map((m) => (m.kind === "service" ? h("p", { key: m.id, class: "msg-service" }, m.body) : (h("div", { id: `message-${m.id}`, key: m.id, class: m.sender_id === self ? "msg mine" : "msg" },
                m.encrypted ? h("em", null, "\u0417\u0430\u0448\u0438\u0444\u0440\u043E\u0432\u0430\u043D\u043D\u043E\u0435 \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435") : m.body && h("p", null, m.body),
                m.attachments.map((a) => (a.kind === "photo" ? h("img", { key: a.id, src: a.thumb_url ?? a.url, alt: "" }) :
                    a.kind === "track" ? h("span", { key: a.id, class: "track" },
                        a.artist,
                        " \u2014 ",
                        a.title,
                        " ",
                        h("audio", { controls: true, preload: "none", src: a.url })) :
                        a.kind === "document" ? h(DocList, { key: a.id, docs: [{ id: a.id, title: a.title ?? "Документ", ext: a.ext ?? "", size: a.size ?? 0, url: a.url }] }) :
                            h(VoiceNote, { key: a.id, url: a.url, seconds: a.duration_sec ?? 0 }))),
                h(Time, { iso: m.created_at }),
                h("button", { type: "button", class: "link", onClick: () => pin(m.id) }, "\u0417\u0430\u043A\u0440\u0435\u043F\u0438\u0442\u044C"))))),
            typing && h("p", { class: "muted typing" }, "\u043F\u0435\u0447\u0430\u0442\u0430\u0435\u0442\u2026"),
            h("div", { ref: end })),
        h(AIText, { text: text, apply: setText }),
        showStickers && h(CustomStickers, { select: x => { setSticker(x); setShowStickers(false); } }),
        sticker && h("p", null,
            h("img", { src: sticker.url, alt: sticker.title, style: "max-height:120px" }),
            h("button", { class: "link", onClick: () => setSticker(null) }, "\u0423\u0431\u0440\u0430\u0442\u044C")),
        doc && h("p", { class: "muted" },
            "\u0414\u043E\u043A\u0443\u043C\u0435\u043D\u0442: ",
            doc.title,
            ".",
            doc.ext,
            " ",
            h("button", { class: "link", onClick: () => setDoc(null) }, "\u0443\u0431\u0440\u0430\u0442\u044C")),
        h("form", { class: "chat-form", onSubmit: send },
            h("input", { ref: docInput, type: "file", hidden: true, onChange: async (e) => {
                    const f = e.target.files?.[0];
                    if (f) {
                        setError(null);
                        try {
                            setDoc(await upload("/documents", f));
                        }
                        catch (err) {
                            setError(err);
                        }
                    }
                } }),
            h("button", { type: "button", class: "icon-btn", "aria-label": "\u041F\u0440\u0438\u043A\u0440\u0435\u043F\u0438\u0442\u044C \u0434\u043E\u043A\u0443\u043C\u0435\u043D\u0442", onClick: () => docInput.current?.click() }, "\uD83D\uDCCE"),
            h("button", { type: "button", class: "ghost", onClick: () => setShowStickers(!showStickers) }, "\u0421\u0442\u0438\u043A\u0435\u0440\u044B"),
            h(MessageField, { value: text, onInput: input }),
            h("button", { class: "primary", disabled: !text.trim() && !doc && !sticker }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C"))));
}
// Голосовые закрыты токеном — их нельзя дать <audio> по ссылке, грузим через fetch.
function VoiceNote({ url, seconds }) {
    const [src, setSrc] = useState(null);
    async function load() {
        const blob = await fetchBlob(url).catch(() => null);
        if (blob)
            setSrc(URL.createObjectURL(blob));
    }
    return src ? h("audio", { controls: true, autoPlay: true, src: src }) : h("button", { class: "chip", onClick: load },
        "\u25B6 \u0413\u043E\u043B\u043E\u0441\u043E\u0432\u043E\u0435 ",
        duration(seconds));
}
