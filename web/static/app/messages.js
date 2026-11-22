import { VideoRecorder, PrivateVideo, ChatGifts, chatServices, ServiceCard } from "./chatmedia.js";
import { AIText } from "./ai.js";
import { CustomStickers } from "./customstickers.js";
import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { fetchBlob, fullName, get, post, put, del, upload } from "./api.js";
import { Icon } from "./shellparts.js";
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
    useEffect(() => on("message_updated", () => list.reload()), []);
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
    return m.attachments.map((a) => ({ video: "Видеосообщение", photo: "Фото", track: "Трек", voice: "Голосовое", document: "Документ" })[a.kind]).join(", ");
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
    const [editing, setEditing] = useState(null);
    const [reply, setReply] = useState(null);
    const [menu, setMenu] = useState(null);
    const [selected, setSelected] = useState([]);
    const [stars, setStars] = useState([]);
    const [notice, setNotice] = useState("");
    const [submitting, setSubmitting] = useState(false);
    const [forward, setForward] = useState(null);
    const [targets, setTargets] = useState([]);
    const [search, setSearch] = useState("");
    const [showSearch, setShowSearch] = useState(false);
    const draft = useRef("");
    const [attachMenu, setAttachMenu] = useState(false);
    const [photo, setPhoto] = useState(null);
    const [track, setTrack] = useState(null);
    const [video, setVideo] = useState(null);
    const [videoPicker, setVideoPicker] = useState(false);
    const [giftPicker, setGiftPicker] = useState(false);
    const [servicePicker, setServicePicker] = useState(false);
    const [voice, setVoice] = useState(null);
    const [musicChoices, setMusicChoices] = useState([]);
    const [emoji, setEmoji] = useState(false);
    const [suggestions, setSuggestions] = useState([]);
    const photoInput = useRef(null);
    const recorder = useRef(null);
    const recordingTimer = useRef();
    const [recording, setRecording] = useState(false);
    useEffect(() => { void get("/stickers").then(r => setSuggestions(r.items), () => { }); return () => { clearTimeout(recordingTimer.current); const rec = recorder.current; if (rec) {
        rec.onstop = null;
        if (rec.state !== "inactive")
            rec.stop();
        rec.stream.getTracks().forEach(t => t.stop());
    } }; }, []);
    async function recordVoice() { try {
        if (recording) {
            recorder.current?.stop();
            return;
        }
        const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
        let rec;
        try {
            rec = new MediaRecorder(stream);
        }
        catch (e) {
            stream.getTracks().forEach(t => t.stop());
            throw e;
        }
        recorder.current = rec;
        const chunks = [];
        rec.ondataavailable = e => { if (e.data.size)
            chunks.push(e.data); };
        const timer = setTimeout(() => { if (rec.state !== "inactive")
            rec.stop(); }, 290000);
        recordingTimer.current = timer;
        rec.onstop = async () => { clearTimeout(timer); stream.getTracks().forEach(t => t.stop()); setRecording(false); try {
            setVoice(await upload("/voice", new File(chunks, "voice.webm", { type: rec.mimeType })));
        }
        catch (e) {
            setError(e);
        } };
        rec.start();
        setRecording(true);
    }
    catch (e) {
        setError(e);
    } }
    async function chooseMusic() { setAttachMenu(false); try {
        setMusicChoices((await get(`/users/${me()?.id}/music?limit=50`)).items);
    }
    catch (e) {
        setError(e);
    } }
    function attachLocation() { setAttachMenu(false); navigator.geolocation.getCurrentPosition(p => setText(old => `${old}\nhttps://www.openstreetmap.org/?mlat=${p.coords.latitude.toFixed(5)}&mlon=${p.coords.longitude.toFixed(5)}`), e => setError(new Error(e.message)), { timeout: 15000 }); }
    useEffect(() => { setEditing(null); setReply(null); setMenu(null); setSelected([]); setVideo(null); setVideoPicker(false); setGiftPicker(false); setServicePicker(false); try {
        const stored = JSON.parse(localStorage.getItem(`sn-message-stars-${me()?.id}`) || "[]");
        setStars(Array.isArray(stored) ? stored.filter(x => Number.isSafeInteger(x)) : []);
    }
    catch {
        setStars([]);
    } }, [id]);
    useEffect(() => { const close = (e) => { if (!e.target?.closest?.(".message-actions"))
        setMenu(null); }; const esc = (e) => { if (e.key === "Escape") {
        setMenu(null);
        setForward(null);
    } }; document.addEventListener("pointerdown", close); document.addEventListener("keydown", esc); return () => { document.removeEventListener("pointerdown", close); document.removeEventListener("keydown", esc); }; }, []);
    function updateMessage(m) { setMsgs(xs => xs?.map(x => x.id === m.id ? m : x) ?? null); setPinned(x => x?.id === m.id ? m : x); }
    useEffect(() => on("message_updated", (m) => { if (m.conversation_id === id)
        updateMessage(m); }), [id]);
    function edit(m) { draft.current = text; setText(m.body || ""); setEditing(m); setReply(null); setMenu(null); }
    function cancelEdit() { setEditing(null); setText(draft.current); }
    async function remove(m) { setMenu(null); if (!confirm("Удалить сообщение у всех участников?"))
        return; try {
        updateMessage(await del(`/conversations/${id}/messages/${m.id}`));
        if (editing?.id === m.id)
            cancelEdit();
    }
    catch (e) {
        setError(e);
    } }
    function star(mid) { const next = stars.includes(mid) ? stars.filter(x => x !== mid) : [...stars, mid]; try {
        localStorage.setItem(`sn-message-stars-${me()?.id}`, JSON.stringify(next));
        setStars(next);
        setMenu(null);
        setNotice("Отметка сохранена на этом устройстве");
    }
    catch (e) {
        setError(e);
    } }
    async function copy(value) { try {
        await navigator.clipboard.writeText(value);
        setNotice("Текст скопирован");
        setMenu(null);
    }
    catch (e) {
        setError(e);
    } }
    async function chooseForward(m) { setMenu(null); try {
        setTargets((await get("/conversations")).items);
        setForward(m);
    }
    catch (e) {
        setError(e);
    } }
    async function forwardTo(target) { if (!forward || submitting)
        return; setSubmitting(true); try {
        await post(`/conversations/${target}/messages`, { body: `Пересланное сообщение:\n${forward.body}` });
        setForward(null);
        setNotice("Текст переслан");
    }
    catch (e) {
        setError(e);
    }
    finally {
        setSubmitting(false);
    } }
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
        if (sending.current || recording || (!text.trim() && !doc && !sticker && !photo && !track && !voice && !video))
            return;
        sending.current = true;
        setSubmitting(true);
        setError(null);
        try {
            if (editing) {
                const m = await put(`/conversations/${id}/messages/${editing.id}`, { body: text });
                updateMessage(m);
                cancelEdit();
                return;
            }
            const body = reply ? `> ${(reply.body || "Вложение").slice(0, 500)}\n\n${text}` : text;
            if (body.length > 4000)
                throw new Error("Сообщение вместе с цитатой превышает 4000 символов.");
            const m = await post(`/conversations/${id}/messages`, { body, attachments: [...(doc ? [{ kind: "document", id: doc.id }] : []), ...(sticker ? [{ kind: "photo", id: sticker.id }] : []), ...(photo ? [{ kind: "photo", id: photo.id }] : []), ...(track ? [{ kind: "track", id: track.id }] : []), ...(voice ? [{ kind: "voice", id: voice.id }] : []), ...(video ? [{ kind: "video", id: video.id }] : [])] });
            setPhoto(null);
            setTrack(null);
            setVoice(null);
            setVideo(null);
            setReply(null);
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
            setSubmitting(false);
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
        h("header", { class: "chat-top" },
            h("a", { href: "/im", "aria-label": "\u0412\u0441\u0435 \u0434\u0438\u0430\u043B\u043E\u0433\u0438" },
                h(Icon, { name: "close" })),
            conv?.peer && h(Avatar, { user: conv.peer }),
            h("h1", null, conv ? title(conv) : "Диалог"),
            conv?.peer && h("button", { class: "icon-btn", "aria-label": "\u041F\u043E\u0437\u0432\u043E\u043D\u0438\u0442\u044C", onClick: () => startCall(conv.peer) },
                h(Icon, { name: "phone" })),
            h("button", { class: "icon-btn", "aria-label": "\u041F\u043E\u0438\u0441\u043A \u043F\u043E \u0437\u0430\u0433\u0440\u0443\u0436\u0435\u043D\u043D\u044B\u043C \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F\u043C", onClick: () => setShowSearch(!showSearch) },
                h(Icon, { name: "search" }))),
        showSearch && h("input", { type: "search", "aria-label": "\u041F\u043E\u0438\u0441\u043A \u043F\u043E \u0437\u0430\u0433\u0440\u0443\u0436\u0435\u043D\u043D\u044B\u043C \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F\u043C", placeholder: "\u041F\u043E\u0438\u0441\u043A \u043F\u043E \u0437\u0430\u0433\u0440\u0443\u0436\u0435\u043D\u043D\u044B\u043C \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F\u043C", value: search, onInput: e => setSearch(e.currentTarget.value) }),
        notice && h("p", { role: "status" },
            notice,
            " ",
            h("button", { class: "link", onClick: () => setNotice("") }, "\u0417\u0430\u043A\u0440\u044B\u0442\u044C")),
        selected.length > 0 && h("div", { class: "message-selection" },
            "\u0412\u044B\u0431\u0440\u0430\u043D\u043E: ",
            selected.length,
            h("button", { onClick: () => copy((msgs || []).filter(m => selected.includes(m.id)).map(m => m.body || "").join("\n")) }, "\u041A\u043E\u043F\u0438\u0440\u043E\u0432\u0430\u0442\u044C \u0442\u0435\u043A\u0441\u0442"),
            h("button", { onClick: () => setSelected([]) }, "\u041E\u0442\u043C\u0435\u043D\u0438\u0442\u044C \u0432\u044B\u0431\u043E\u0440")),
        forward && h("div", { class: "music-panel", role: "region", "aria-label": "\u041F\u0435\u0440\u0435\u0441\u043B\u0430\u0442\u044C \u0442\u0435\u043A\u0441\u0442" },
            h("h2", null, "\u041F\u0435\u0440\u0435\u0441\u043B\u0430\u0442\u044C \u0442\u0435\u043A\u0441\u0442 \u0432 \u0434\u0438\u0430\u043B\u043E\u0433"),
            h("p", null, "\u0412\u043B\u043E\u0436\u0435\u043D\u0438\u044F \u043D\u0435 \u043F\u0435\u0440\u0435\u0441\u044B\u043B\u0430\u044E\u0442\u0441\u044F."),
            targets.map(c => h("button", { disabled: submitting, onClick: () => forwardTo(c.id) }, title(c))),
            h("button", { onClick: () => setForward(null) }, "\u041E\u0442\u043C\u0435\u043D\u0430")),
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
            msgs === null ? h(Loading, null) : msgs.filter(m => !showSearch || !search || m.body?.toLocaleLowerCase().includes(search.toLocaleLowerCase())).map((m, index) => (m.kind === "service" ? h("p", { key: m.id, class: "msg-service" }, m.body) : (h("div", { id: `message-${m.id}`, key: m.id, class: m.sender_id === self ? "msg mine" : "msg" },
                (index === 0 || new Date(msgs[index - 1]?.created_at).toDateString() !== new Date(m.created_at).toDateString()) && h("div", { class: "chat-day" }, new Date(m.created_at).toLocaleDateString("ru-RU", { day: "numeric", month: "long" })),
                h("div", { class: "message-author" },
                    m.sender_id === self ? fullName(me()) : conv?.peer?.id === m.sender_id ? fullName(conv.peer) : `Участник #${m.sender_id}`,
                    stars.includes(m.id) && " ★"),
                (m.sender_id === self ? me() : conv?.peer?.id === m.sender_id ? conv.peer : null) && h("div", { class: "message-avatar" },
                    h(Avatar, { user: (m.sender_id === self ? me() : conv.peer) })),
                selected.includes(m.id) && h("span", { class: "badge" }, "\u0412\u044B\u0431\u0440\u0430\u043D\u043E"),
                m.encrypted ? h("em", null, "\u0417\u0430\u0448\u0438\u0444\u0440\u043E\u0432\u0430\u043D\u043D\u043E\u0435 \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435") : m.body && h(ServiceCard, { body: m.body }),
                m.attachments.map((a) => (a.kind === "photo" ? h("img", { key: a.id, src: a.thumb_url ?? a.url, alt: "" }) :
                    a.kind === "video" ? h(PrivateVideo, { key: a.id, url: a.url }) :
                        a.kind === "track" ? h("span", { key: a.id, class: "track" },
                            a.artist,
                            " \u2014 ",
                            a.title,
                            " ",
                            h("audio", { controls: true, preload: "none", src: a.url })) :
                            a.kind === "document" ? h(DocList, { key: a.id, docs: [{ id: a.id, title: a.title ?? "Документ", ext: a.ext ?? "", size: a.size ?? 0, url: a.url }] }) :
                                h(VoiceNote, { key: a.id, url: a.url, seconds: a.duration_sec ?? 0 }))),
                h("span", { class: "message-time", title: new Date(m.created_at).toLocaleString("ru-RU") },
                    new Date(m.created_at).toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" }),
                    m.edited_at && !m.deleted_at && " · изменено"),
                !m.deleted_at && !m.encrypted && h("div", { class: "message-actions" },
                    h("button", { class: "icon-btn", "aria-label": "\u0414\u0435\u0439\u0441\u0442\u0432\u0438\u044F \u0441 \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435\u043C", "aria-expanded": menu === m.id, onClick: () => setMenu(menu === m.id ? null : m.id) }, "\u2022\u2022\u2022"),
                    menu === m.id && h("div", { class: "message-menu" },
                        h("button", { onClick: () => { setReply(m); setMenu(null); if (editing)
                                cancelEdit(); } }, "\u21A9 \u041E\u0442\u0432\u0435\u0442\u0438\u0442\u044C"),
                        m.body && h("button", { onClick: () => chooseForward(m) }, "\u2197 \u041F\u0435\u0440\u0435\u0441\u043B\u0430\u0442\u044C \u0442\u0435\u043A\u0441\u0442"),
                        h("button", { onClick: () => star(m.id) },
                            "\u2606 ",
                            stars.includes(m.id) ? "Убрать из важного" : "Отметить как важное"),
                        m.body && h("button", { onClick: () => copy(m.body) }, "\u041A\u043E\u043F\u0438\u0440\u043E\u0432\u0430\u0442\u044C \u0442\u0435\u043A\u0441\u0442"),
                        m.sender_id === self && h("button", { onClick: () => edit(m) },
                            h(Icon, { name: "edit" }),
                            "\u0420\u0435\u0434\u0430\u043A\u0442\u0438\u0440\u043E\u0432\u0430\u0442\u044C"),
                        m.sender_id === self && h("button", { class: "danger", onClick: () => remove(m) },
                            h(Icon, { name: "trash" }),
                            "\u0423\u0434\u0430\u043B\u0438\u0442\u044C"),
                        h("button", { onClick: () => { setSelected(xs => xs.includes(m.id) ? xs.filter(x => x !== m.id) : [...xs, m.id]); setMenu(null); } }, "\u0412\u044B\u0431\u0440\u0430\u0442\u044C"),
                        h("button", { onClick: () => { void pin(m.id); setMenu(null); } }, "\u0417\u0430\u043A\u0440\u0435\u043F\u0438\u0442\u044C"))))))),
            typing && h("p", { class: "muted typing" }, "\u043F\u0435\u0447\u0430\u0442\u0430\u0435\u0442\u2026"),
            h("div", { ref: end })),
        videoPicker && h(VideoRecorder, { key: id, close: () => setVideoPicker(false), done: v => { setVideo(v); setVideoPicker(false); } }),
        giftPicker && conv?.peer && h(ChatGifts, { key: id, recipient: conv.peer.id, close: () => setGiftPicker(false) }),
        h(AIText, { text: text, apply: setText }),
        showStickers && h(CustomStickers, { select: x => { setSticker(x); setShowStickers(false); } }),
        !editing && text.trim().length > 1 && suggestions.some(x => x.title.toLocaleLowerCase().includes(text.trim().toLocaleLowerCase())) && h("div", { class: "chat-sticker-suggestions" }, suggestions.filter(x => x.title.toLocaleLowerCase().includes(text.trim().toLocaleLowerCase())).slice(0, 5).map(x => h("button", { title: x.title, onClick: () => { setSticker(x); setText(""); } },
            h("img", { src: x.url, alt: x.title })))),
        photo && h("p", null,
            h("img", { src: photo.url, alt: "\u041F\u0440\u0438\u043A\u0440\u0435\u043F\u043B\u0451\u043D\u043D\u043E\u0435 \u0444\u043E\u0442\u043E", style: "max-height:100px" }),
            h("button", { onClick: () => setPhoto(null) }, "\u0423\u0431\u0440\u0430\u0442\u044C \u0444\u043E\u0442\u043E")),
        track && h("p", null,
            track.title,
            h("button", { onClick: () => setTrack(null) }, "\u0423\u0431\u0440\u0430\u0442\u044C \u043C\u0443\u0437\u044B\u043A\u0443")),
        video && h("div", null,
            h(PrivateVideo, { key: video.id, url: video.url }),
            h("button", { onClick: () => setVideo(null) }, "\u0423\u0431\u0440\u0430\u0442\u044C \u0432\u0438\u0434\u0435\u043E")),
        servicePicker && h("div", { class: "music-panel", "aria-label": "\u0421\u0435\u0440\u0432\u0438\u0441\u044B socialnet" },
            h("h2", null, "\u041F\u043E\u0434\u0435\u043B\u0438\u0442\u044C\u0441\u044F \u0441\u0435\u0440\u0432\u0438\u0441\u043E\u043C"),
            chatServices.map(s => h("button", { onClick: () => { setText(t => `${t ? t + "\n" : ""}Сервис socialnet: ${s.name}\n${s.path}`); setServicePicker(false); } }, s.name)),
            h("button", { onClick: () => setServicePicker(false) }, "\u0417\u0430\u043A\u0440\u044B\u0442\u044C")),
        voice && h("p", null,
            "\u0413\u043E\u043B\u043E\u0441\u043E\u0432\u043E\u0435 \u00B7 ",
            duration(voice.duration_sec),
            h("button", { onClick: () => setVoice(null) }, "\u0423\u0431\u0440\u0430\u0442\u044C")),
        musicChoices.length > 0 && h("div", { class: "music-panel" },
            musicChoices.map(t => h("button", { onClick: () => { setTrack(t); setMusicChoices([]); } }, t.title)),
            h("button", { onClick: () => setMusicChoices([]) }, "\u0417\u0430\u043A\u0440\u044B\u0442\u044C")),
        emoji && h("div", { class: "chat-emoji", "aria-label": "\u042D\u043C\u043E\u0434\u0437\u0438" }, ["😀", "😂", "😊", "😍", "😮", "😢", "😡", "👍", "❤️", "🔥", "🎉", "🙏"].map(x => h("button", { onClick: () => { setText(t => t + x); setEmoji(false); } }, x))),
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
            (editing || reply) && h("div", { class: "message-context" },
                h("strong", null, editing ? "Редактируемое сообщение" : "Ответ с цитатой"),
                h("p", null, (editing || reply)?.body || "Вложение"),
                h("button", { type: "button", "aria-label": "\u041E\u0442\u043C\u0435\u043D\u0438\u0442\u044C", onClick: () => editing ? cancelEdit() : setReply(null) },
                    h(Icon, { name: "close" }))),
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
            h("input", { ref: photoInput, type: "file", accept: "image/*", hidden: true, onChange: async (e) => { const f = e.currentTarget.files?.[0]; e.currentTarget.value = ""; if (f)
                    try {
                        setPhoto(await upload("/media", f));
                    }
                    catch (e) {
                        setError(e);
                    } } }),
            !editing && h("div", { class: "chat-attachment" },
                h("button", { type: "button", class: "icon-btn", "aria-label": "\u041F\u0440\u0438\u043A\u0440\u0435\u043F\u0438\u0442\u044C", "aria-expanded": attachMenu, onClick: () => setAttachMenu(!attachMenu) },
                    h(Icon, { name: "plus" })),
                attachMenu && h("div", { class: "message-menu" },
                    h("button", { type: "button", onClick: () => { photoInput.current?.click(); setAttachMenu(false); } },
                        h(Icon, { name: "photo" }),
                        "\u0424\u043E\u0442\u043E"),
                    h("button", { type: "button", onClick: chooseMusic },
                        h(Icon, { name: "music" }),
                        "\u041C\u0443\u0437\u044B\u043A\u0430"),
                    h("button", { type: "button", onClick: () => { docInput.current?.click(); setAttachMenu(false); } },
                        h(Icon, { name: "file" }),
                        "\u0424\u0430\u0439\u043B"),
                    h("button", { type: "button", onClick: () => { setServicePicker(true); setAttachMenu(false); } }, "\u25A6 \u0421\u0435\u0440\u0432\u0438\u0441"),
                    conv?.peer && h("button", { type: "button", onClick: () => { setGiftPicker(true); setAttachMenu(false); } }, "\uD83C\uDF81 \u041F\u043E\u0434\u0430\u0440\u043E\u043A"),
                    h("button", { type: "button", onClick: attachLocation }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C \u043C\u043E\u0451 \u043C\u0435\u0441\u0442\u043E\u043F\u043E\u043B\u043E\u0436\u0435\u043D\u0438\u0435"))),
            h("button", { type: "button", class: "icon-btn", "aria-label": "\u0421\u0442\u0438\u043A\u0435\u0440\u044B", title: "\u0421\u0442\u0438\u043A\u0435\u0440\u044B", onClick: () => setShowStickers(!showStickers) }, "\u25A7"),
            h(MessageField, { value: text, onInput: input }),
            h("button", { type: "button", class: "icon-btn", "aria-label": "\u042D\u043C\u043E\u0434\u0437\u0438", onClick: () => setEmoji(!emoji) }, "\u263A"),
            !editing && h("button", { type: "button", class: "icon-btn", disabled: recording, "aria-label": "\u0417\u0430\u043F\u0438\u0441\u0430\u0442\u044C \u0432\u0438\u0434\u0435\u043E\u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435", onClick: () => setVideoPicker(true) }, "\u25B7"),
            !editing && h("button", { type: "button", class: "icon-btn", disabled: videoPicker, "aria-label": recording ? "Закончить запись" : "Записать голосовое сообщение", onClick: recordVoice }, recording ? "■" : "🎙"),
            h("button", { class: "primary chat-send", "aria-label": editing ? "Сохранить" : "Отправить", title: editing ? "Сохранить" : "Отправить", disabled: submitting || recording || (editing ? !text.trim() : !text.trim() && !doc && !sticker && !photo && !track && !voice && !video) }, editing ? "✓" : "➤"))));
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
