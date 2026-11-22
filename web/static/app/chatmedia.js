import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { fetchBlob, get, post, upload } from "./api.js";
import { ErrorLine } from "./ui.js";
export function PrivateVideo({ url }) {
    const [src, setSrc] = useState("");
    const [error, setError] = useState(null);
    const [busy, setBusy] = useState(false);
    const object = useRef("");
    const alive = useRef(true);
    useEffect(() => () => { alive.current = false; URL.revokeObjectURL(object.current); }, []);
    async function load() { if (busy)
        return; setBusy(true); try {
        const blob = await fetchBlob(url);
        if (alive.current) {
            object.current = URL.createObjectURL(blob);
            setSrc(object.current);
        }
    }
    catch (e) {
        if (alive.current)
            setError(e);
    }
    finally {
        if (alive.current)
            setBusy(false);
    } }
    return h("div", { class: "chat-video" },
        h(ErrorLine, { error: error }),
        src ? h("video", { controls: true, playsInline: true, preload: "metadata", src: src }) : h("button", { disabled: busy, onClick: load }, busy ? "Загрузка…" : "▶ Видеосообщение"));
}
export function VideoRecorder({ done, close }) {
    const [state, setState] = useState("idle");
    const [seconds, setSeconds] = useState(0);
    const [blob, setBlob] = useState(null);
    const [src, setSrc] = useState("");
    const [error, setError] = useState(null);
    const camera = useRef(null);
    const rec = useRef(null);
    const alive = useRef(true);
    const timer = useRef();
    const stream = useRef(null);
    useEffect(() => () => { alive.current = false; clearInterval(timer.current); if (rec.current) {
        rec.current.onstop = null;
        if (rec.current.state !== "inactive")
            rec.current.stop();
    } stream.current?.getTracks().forEach(t => t.stop()); }, []);
    useEffect(() => { if (!blob) {
        setSrc("");
        return;
    } const u = URL.createObjectURL(blob); setSrc(u); return () => URL.revokeObjectURL(u); }, [blob]);
    async function start() {
        setError(null);
        setBlob(null);
        setState("permission");
        try {
            const s = await navigator.mediaDevices.getUserMedia({ audio: true, video: { width: { ideal: 480 }, height: { ideal: 480 }, facingMode: "user" } });
            if (!alive.current) {
                s.getTracks().forEach(t => t.stop());
                return;
            }
            stream.current = s;
            const mime = ["video/webm;codecs=vp8,opus", "video/mp4", "video/webm"].find(x => MediaRecorder.isTypeSupported(x));
            const r = new MediaRecorder(s, mime ? { mimeType: mime, videoBitsPerSecond: 800000 } : undefined);
            rec.current = r;
            let size = 0;
            const chunks = [];
            r.ondataavailable = e => { chunks.push(e.data); size += e.data.size; if (size > 45 * 1024 * 1024 && r.state !== "inactive")
                r.stop(); };
            r.onstop = () => { clearInterval(timer.current); s.getTracks().forEach(t => t.stop()); if (alive.current) {
                setBlob(new Blob(chunks, { type: r.mimeType }));
                setState("preview");
            } };
            r.start(500);
            setSeconds(0);
            setState("recording");
            if (camera.current) {
                camera.current.srcObject = s;
                void camera.current.play().catch(() => { });
            }
            const start = Date.now();
            timer.current = setInterval(() => { const n = Math.floor((Date.now() - start) / 1000); setSeconds(n); if (n >= 59 && r.state !== "inactive")
                r.stop(); }, 250);
        }
        catch (e) {
            stream.current?.getTracks().forEach(t => t.stop());
            setError(e);
            setState("idle");
        }
    }
    async function save() { if (!blob || state === "uploading")
        return; setState("uploading"); setError(null); try {
        const v = await upload("/video-messages", new File([blob], blob.type.includes("mp4") ? "message.mp4" : "message.webm", { type: blob.type }));
        if (alive.current)
            done(v);
    }
    catch (e) {
        if (alive.current) {
            setError(e);
            setState("preview");
        }
    } }
    return h("section", { class: "music-panel chat-video", "aria-label": "\u0417\u0430\u043F\u0438\u0441\u044C \u0432\u0438\u0434\u0435\u043E\u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F" },
        h("h2", null, "\u0412\u0438\u0434\u0435\u043E\u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435"),
        h("p", null, "\u0414\u043E 60 \u0441\u0435\u043A\u0443\u043D\u0434 \u0438 50 \u041C\u0411. \u041F\u0435\u0440\u0435\u0434 \u043E\u0442\u043F\u0440\u0430\u0432\u043A\u043E\u0439 \u0437\u0430\u043F\u0438\u0441\u044C \u043C\u043E\u0436\u043D\u043E \u043F\u043E\u0441\u043C\u043E\u0442\u0440\u0435\u0442\u044C."),
        h(ErrorLine, { error: error }),
        h("video", { ref: camera, hidden: state !== "recording", muted: true, playsInline: true }),
        src && h("video", { src: src, controls: true, playsInline: true }),
        state === "recording" ? h("button", { onClick: () => rec.current?.stop() },
            "\u25A0 \u0417\u0430\u0432\u0435\u0440\u0448\u0438\u0442\u044C \u00B7 ",
            seconds,
            " \u0441") : h("button", { disabled: state === "permission" || state === "uploading", onClick: start }, blob ? "Записать заново" : "Включить камеру"),
        h("label", null,
            "\u0412\u044B\u0431\u0440\u0430\u0442\u044C \u0432\u0438\u0434\u0435\u043E ",
            h("input", { type: "file", accept: "video/mp4,video/webm,video/quicktime", disabled: state === "recording" || state === "permission" || state === "uploading", onChange: e => { const f = e.currentTarget.files?.[0]; if (f) {
                    if (f.size > 50 * 1024 * 1024) {
                        setError(new Error("Максимум 50 МБ"));
                        return;
                    }
                    setBlob(f);
                    setState("preview");
                } } })),
        blob && state !== "recording" && h("button", { disabled: state === "uploading" || state === "permission", onClick: save }, state === "uploading" ? "Обработка…" : "Прикрепить к сообщению"),
        h("button", { onClick: close }, "\u041E\u0442\u043C\u0435\u043D\u0430"));
}
export function ChatGifts({ recipient, close }) {
    const [items, setItems] = useState([]);
    const [gift, setGift] = useState(null);
    const [text, setText] = useState("");
    const [busy, setBusy] = useState(false);
    const [sent, setSent] = useState(false);
    const [error, setError] = useState(null);
    const key = useRef(crypto.randomUUID());
    const locked = useRef(false);
    const payload = useRef(null);
    useEffect(() => { void get("/gifts/catalog").then(r => setItems(r.items || []), setError); }, []);
    async function send() { if (!gift || locked.current)
        return; locked.current = true; setBusy(true); setError(null); payload.current ??= { gift_id: gift.id, message: text, anonymous: false, idempotency_key: key.current }; try {
        await post(`/users/${recipient}/gifts`, payload.current);
        setSent(true);
    }
    catch (e) {
        setError(e);
    }
    finally {
        locked.current = false;
        setBusy(false);
    } }
    return h("section", { class: "music-panel", "aria-label": "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C \u043F\u043E\u0434\u0430\u0440\u043E\u043A" },
        h("h2", null, "\u041F\u043E\u0434\u0430\u0440\u043E\u043A \u0441\u043E\u0431\u0435\u0441\u0435\u0434\u043D\u0438\u043A\u0443"),
        h(ErrorLine, { error: error }),
        sent ? h("p", { role: "status" }, "\u041F\u043E\u0434\u0430\u0440\u043E\u043A \u043E\u0442\u043F\u0440\u0430\u0432\u043B\u0435\u043D. \u041E\u043D \u043F\u043E\u044F\u0432\u0438\u0442\u0441\u044F \u0432 \u043F\u043E\u0434\u0430\u0440\u043A\u0430\u0445 \u043F\u0440\u043E\u0444\u0438\u043B\u044F \u043F\u043E\u043B\u0443\u0447\u0430\u0442\u0435\u043B\u044F; \u043F\u043E\u043B\u0443\u0447\u0430\u0442\u0435\u043B\u044C \u043F\u043E\u043B\u0443\u0447\u0438\u0442 \u0443\u0432\u0435\u0434\u043E\u043C\u043B\u0435\u043D\u0438\u0435.") : h("div", null,
            items.map(g => h("button", { disabled: busy || !!gift, onClick: () => setGift(g) },
                "\uD83C\uDF81 ",
                g.name,
                " \u00B7 ",
                g.price,
                " \u043C\u043E\u043D\u0435\u0442")),
            gift && h("div", null,
                h("p", null,
                    gift.name,
                    ". \u0421 \u0431\u0430\u043B\u0430\u043D\u0441\u0430 \u0431\u0443\u0434\u0435\u0442 \u0441\u043F\u0438\u0441\u0430\u043D\u043E ",
                    gift.price,
                    " \u043C\u043E\u043D\u0435\u0442."),
                h("textarea", { "aria-label": "\u041F\u043E\u0437\u0434\u0440\u0430\u0432\u043B\u0435\u043D\u0438\u0435", maxLength: 200, value: text, disabled: busy || !!payload.current, onInput: e => setText(e.currentTarget.value) }),
                h("button", { disabled: busy, onClick: send }, busy ? "Отправка…" : `Подтвердить за ${gift.price} монет`))),
        h("button", { disabled: busy, onClick: close }, "\u0417\u0430\u043A\u0440\u044B\u0442\u044C"));
}
export const chatServices = [{ name: "Музыка", path: "/music" }, { name: "Видео", path: "/videos" }, { name: "Фотографии", path: "/photos" }, { name: "Люди рядом", path: "/nearby" }];
export function ServiceCard({ body }) {
    const service = chatServices.find(s => body.endsWith(`Сервис socialnet: ${s.name}\n${s.path}`));
    const prefix = service ? body.slice(0, -`Сервис socialnet: ${service.name}\n${service.path}`.length).trim() : "";
    return service ? h("div", null,
        prefix && h("p", null, prefix),
        h("a", { class: "chat-service-card", href: service.path },
            "\u25A6 ",
            service.name,
            " \u2192")) : h("p", null, body);
}
