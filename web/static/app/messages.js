import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { fetchBlob, fullName, get, post } from "./api.js";
import { on, sendTyping } from "./realtime.js";
import { navigate } from "./router.js";
import { me } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, Time, duration, useLoad } from "./ui.js";
const title = (c) => (c.kind === "group" ? c.title || "Беседа" : c.peer ? fullName(c.peer) : "Диалог");
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
    return m.attachments.map((a) => ({ photo: "Фото", track: "Трек", voice: "Голосовое" })[a.kind]).join(", ");
}
// /im/u/:id — открыть (или создать) личный диалог с пользователем
export function DirectPage({ userId }) {
    const [error, setError] = useState(null);
    const [text, setText] = useState("");
    const peer = useLoad(() => get(`/users/${userId}`), [userId]);
    async function send(e) {
        e.preventDefault();
        try {
            const m = await post(`/users/${userId}/messages`, { body: text });
            navigate(`/im/${m.conversation_id}`, true);
        }
        catch (err) {
            setError(err);
        }
    }
    return (h("section", { class: "chat" },
        h("h1", null, peer.data ? fullName(peer.data) : "Новый диалог"),
        h(Empty, null, "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0439 \u0435\u0449\u0451 \u043D\u0435\u0442."),
        h("form", { class: "chat-form", onSubmit: send },
            h("input", { value: text, onInput: (e) => setText(e.target.value), placeholder: "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435", "aria-label": "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435", maxLength: 4000 }),
            h("button", { class: "primary", disabled: !text.trim() }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C")),
        h(ErrorLine, { error: error })));
}
export function ChatPage({ id }) {
    const [msgs, setMsgs] = useState(null);
    const [conv, setConv] = useState(null);
    const [text, setText] = useState("");
    const [error, setError] = useState(null);
    const [typing, setTyping] = useState(false);
    const end = useRef(null);
    const lastTyping = useRef(0);
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
        setError(null);
        try {
            const m = await post(`/conversations/${id}/messages`, { body: text });
            setText("");
            setMsgs((xs) => (xs && !xs.some((x) => x.id === m.id) ? [...xs, m] : xs));
        }
        catch (err) {
            setError(err);
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
        h(ErrorLine, { error: error }),
        h("div", { class: "chat-log", role: "log", "aria-live": "polite" },
            msgs === null ? h(Loading, null) : msgs.map((m) => (m.kind === "service" ? h("p", { key: m.id, class: "msg-service" }, m.body) : (h("div", { key: m.id, class: m.sender_id === self ? "msg mine" : "msg" },
                m.encrypted ? h("em", null, "\u0417\u0430\u0448\u0438\u0444\u0440\u043E\u0432\u0430\u043D\u043D\u043E\u0435 \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435") : m.body && h("p", null, m.body),
                m.attachments.map((a) => (a.kind === "photo" ? h("img", { key: a.id, src: a.thumb_url ?? a.url, alt: "" }) :
                    a.kind === "track" ? h("span", { key: a.id, class: "track" },
                        a.artist,
                        " \u2014 ",
                        a.title,
                        " ",
                        h("audio", { controls: true, preload: "none", src: a.url })) :
                        h(VoiceNote, { key: a.id, url: a.url, seconds: a.duration_sec ?? 0 }))),
                h(Time, { iso: m.created_at }))))),
            typing && h("p", { class: "muted typing" }, "\u043F\u0435\u0447\u0430\u0442\u0430\u0435\u0442\u2026"),
            h("div", { ref: end })),
        h("form", { class: "chat-form", onSubmit: send },
            h("input", { value: text, onInput: input, placeholder: "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435", "aria-label": "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435", maxLength: 4000 }),
            h("button", { class: "primary", disabled: !text.trim() }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C"))));
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
