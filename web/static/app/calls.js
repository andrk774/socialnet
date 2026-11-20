import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { api, get, post, fullName } from "./api.js";
import { me } from "./session.js";
import { on } from "./realtime.js";
import { Avatar, ErrorLine, Loading, Time, useLoad } from "./ui.js";
const live = (c) => c.status === "ringing" || c.status === "active";
const labels = { ringing: "Вызов", active: "Принят", ended: "Завершён", declined: "Отклонён", missed: "Пропущен" };
export function startCall(peer, video = false) { window.dispatchEvent(new CustomEvent("socialnet-call", { detail: { peer, video } })); }
function StreamVideo({ stream, muted = false }) {
    const ref = useRef(null);
    useEffect(() => { if (ref.current) {
        ref.current.srcObject = stream;
        ref.current.play().catch(() => { });
    } }, [stream]);
    return h("video", { ref: ref, autoPlay: true, playsInline: true, muted: muted, controls: !muted, "aria-label": muted ? "Ваша камера" : "Собеседник" });
}
async function gather(pc) {
    if (pc.iceGatheringState === "complete")
        return;
    await new Promise((resolve, reject) => { const done = () => { clearTimeout(timer); pc.removeEventListener("icegatheringstatechange", change); }; const change = () => { if (pc.iceGatheringState === "complete") {
        done();
        resolve();
    } }; const timer = setTimeout(() => { done(); reject(new Error("Не удалось подготовить соединение. Проверьте сеть и TURN-сервер.")); }, 15000); pc.addEventListener("icegatheringstatechange", change); change(); });
}
export function CallManager() {
    const [call, setCall] = useState(null), [local, setLocal] = useState(null), [remote, setRemote] = useState(null);
    const [busy, setBusy] = useState(false), [error, setError] = useState(null), [connection, setConnection] = useState(""), [muted, setMuted] = useState(false), [camera, setCamera] = useState(true);
    const dialog = useRef(null);
    const visible = !!call || busy || !!error;
    useEffect(() => { if (visible && !dialog.current?.open)
        dialog.current?.showModal(); }, [visible]);
    const current = useRef(null), pc = useRef(null), stream = useRef(null), working = useRef(false), generation = useRef(0), connectTimer = useRef(), lastBeat = useRef(0);
    function update(c) { current.current = c; setCall(c); }
    function cleanup() { generation.current++; clearTimeout(connectTimer.current); pc.current?.close(); pc.current = null; stream.current?.getTracks().forEach(t => t.stop()); stream.current = null; setLocal(null); setRemote(null); setConnection(""); setMuted(false); setCamera(true); working.current = false; setBusy(false); }
    async function end(action = "end", keepError = false) {
        if (!keepError)
            setError(null);
        const c = current.current;
        cleanup();
        update(null);
        if (c)
            try {
                await post(`/calls/${c.id}/action`, { action });
            }
            catch (e) {
                setError(e);
            }
        window.dispatchEvent(new Event("calls-changed"));
    }
    async function prepare(video) {
        if (!navigator.mediaDevices?.getUserMedia || !window.RTCPeerConnection)
            throw new Error("Для звонков откройте socialnet по HTTPS или на localhost в современном браузере.");
        const gen = generation.current;
        const config = await get("/calls/config");
        const media = await navigator.mediaDevices.getUserMedia({ audio: true, video });
        if (gen !== generation.current) {
            media.getTracks().forEach(t => t.stop());
            throw new Error("Вызов отменён");
        }
        stream.current = media;
        setLocal(media);
        const rtc = new RTCPeerConnection({ iceServers: config.ice_servers });
        pc.current = rtc;
        media.getTracks().forEach(t => rtc.addTrack(t, media));
        rtc.ontrack = e => { if (pc.current === rtc)
            setRemote(e.streams[0] || new MediaStream([e.track])); };
        rtc.onconnectionstatechange = () => { if (pc.current !== rtc)
            return; setConnection(rtc.connectionState); if (rtc.connectionState === "connected")
            clearTimeout(connectTimer.current); if (rtc.connectionState === "failed") {
            setError(new Error("Соединение прервано. Проверьте сеть; для разных сетей может понадобиться TURN."));
            void end("end", true);
        } };
        return rtc;
    }
    function connectionDeadline() { clearTimeout(connectTimer.current); connectTimer.current = window.setTimeout(() => { if (pc.current?.connectionState !== "connected") {
        setError(new Error("Собеседник принял вызов, но медиасоединение не установлено."));
        void end("end", true);
    } }, 45000); }
    async function initiate(peer, video) {
        if (current.current || working.current) {
            setError(new Error("Сначала завершите текущий вызов."));
            return;
        }
        working.current = true;
        setBusy(true);
        setError(null);
        const gen = generation.current;
        let createdID;
        try {
            const rtc = await prepare(video);
            await rtc.setLocalDescription(await rtc.createOffer());
            await gather(rtc);
            if (gen !== generation.current)
                return;
            const created = await post(`/users/${peer.id}/calls`, { video, offer: rtc.localDescription.sdp });
            createdID = created.id;
            if (gen !== generation.current) {
                await post(`/calls/${created.id}/action`, { action: "end" });
                return;
            }
            update(await get(`/calls/${created.id}`));
        }
        catch (e) {
            if (createdID)
                void post(`/calls/${createdID}/action`, { action: "end" }).catch(() => { });
            if (gen === generation.current) {
                setError(e);
                cleanup();
            }
        }
        finally {
            if (gen === generation.current) {
                working.current = false;
                setBusy(false);
            }
        }
    }
    async function accept() {
        const c = current.current;
        if (!c || working.current)
            return;
        working.current = true;
        setBusy(true);
        setError(null);
        const gen = generation.current;
        try {
            const detail = await get(`/calls/${c.id}`);
            if (!detail.offer)
                throw new Error("Вызов уже завершён.");
            const rtc = await prepare(c.video);
            await rtc.setRemoteDescription({ type: "offer", sdp: detail.offer });
            await rtc.setLocalDescription(await rtc.createAnswer());
            await gather(rtc);
            if (gen !== generation.current)
                return;
            await post(`/calls/${c.id}/action`, { action: "answer", answer: rtc.localDescription.sdp });
            update({ ...c, status: "active" });
            connectionDeadline();
        }
        catch (e) {
            if (gen === generation.current) {
                setError(e);
                cleanup();
            }
        }
        finally {
            if (gen === generation.current) {
                working.current = false;
                setBusy(false);
            }
        }
    }
    useEffect(() => {
        let stopped = false, polling = false;
        const poll = async () => {
            if (stopped || polling)
                return;
            polling = true;
            try {
                const c = current.current;
                if (c) {
                    const fresh = await get(`/calls/${c.id}`);
                    if (stopped || current.current?.id !== c.id)
                        return;
                    if (!live(fresh)) {
                        cleanup();
                        update(null);
                        window.dispatchEvent(new Event("calls-changed"));
                        return;
                    }
                    if (fresh.status === "active" && !pc.current && !working.current) {
                        cleanup();
                        update(null);
                        return;
                    }
                    update(fresh);
                    if (fresh.status === "active" && pc.current) {
                        if (fresh.caller_id === me()?.id && fresh.answer && !pc.current.remoteDescription) {
                            await pc.current.setRemoteDescription({ type: "answer", sdp: fresh.answer });
                            connectionDeadline();
                        }
                        if (Date.now() - lastBeat.current > 10000) {
                            lastBeat.current = Date.now();
                            await post(`/calls/${fresh.id}/action`, { action: "heartbeat" });
                        }
                    }
                }
                else if (!working.current) {
                    const list = await get("/calls");
                    if (stopped || current.current || working.current)
                        return;
                    const incoming = list.items.find(x => x.status === "ringing" && x.callee_id === me()?.id);
                    if (incoming)
                        update(incoming);
                }
            }
            catch (e) {
                if (current.current)
                    setError(e);
            }
            finally {
                polling = false;
            }
        };
        const handler = (e) => { const d = e.detail; void initiate(d.peer, d.video); };
        window.addEventListener("socialnet-call", handler);
        const off = on("call", poll);
        const timer = setInterval(poll, 2000);
        void poll();
        return () => { stopped = true; clearInterval(timer); off(); window.removeEventListener("socialnet-call", handler); const c = current.current, owned = !!pc.current; cleanup(); if (c && owned)
            void post(`/calls/${c.id}/action`, { action: "end" }).catch(() => { }); };
    }, []);
    if (!call && !busy && !error)
        return null;
    const incoming = !!call && call.callee_id === me()?.id && call.status === "ringing";
    return h("dialog", { ref: dialog, class: "call-overlay", "aria-label": "\u0417\u0432\u043E\u043D\u043E\u043A", onCancel: e => { e.preventDefault(); void end(incoming ? "decline" : "end"); } },
        h("div", { class: "call-window" },
            h("h2", null, call ? fullName(call.peer) : "Подготовка звонка"),
            h("p", { role: "status" }, busy ? "Подготовка микрофона и соединения…" : incoming ? "Входящий " + (call?.video ? "видеозвонок" : "аудиозвонок") : connection === "connected" ? "Соединение установлено" : call?.status === "active" ? "Соединяем…" : call ? "Ожидание ответа…" : "Звонок завершён"),
            h(ErrorLine, { error: error }),
            h("div", { class: call?.video ? "call-streams" : "call-streams audio-call" },
                h(StreamVideo, { stream: remote }),
                local && h(StreamVideo, { stream: local, muted: true })),
            h("div", { class: "call-controls" },
                incoming && h("button", { class: "primary", disabled: busy, onClick: accept }, "\u041F\u0440\u0438\u043D\u044F\u0442\u044C"),
                local && h(Fragment, null,
                    h("button", { class: "ghost", "aria-pressed": muted, onClick: () => { stream.current?.getAudioTracks().forEach(t => t.enabled = muted); setMuted(!muted); } }, muted ? "Включить микрофон" : "Выключить микрофон"),
                    call?.video && h("button", { class: "ghost", "aria-pressed": !camera, onClick: () => { stream.current?.getVideoTracks().forEach(t => t.enabled = !camera); setCamera(!camera); } }, camera ? "Выключить камеру" : "Включить камеру")),
                h("button", { class: "call-end", onClick: () => void end(incoming ? "decline" : "end") }, incoming ? "Отклонить" : call || busy ? "Завершить" : "Закрыть"))));
}
export function CallsPage() {
    const history = useLoad(() => get("/calls"), []), config = useLoad(() => get("/calls/config"), []);
    const [filter, setFilter] = useState("all"), [q, setQ] = useState(""), [people, setPeople] = useState([]), [error, setError] = useState(null), [loading, setLoading] = useState(false);
    useEffect(() => { const fn = () => history.reload(); window.addEventListener("calls-changed", fn); const off = on("call", fn); return () => { window.removeEventListener("calls-changed", fn); off(); }; }, []);
    useEffect(() => { let active = true; setPeople([]); if (q.trim().length < 2)
        return; const timer = setTimeout(() => get(`/search?type=users&q=${encodeURIComponent(q.trim())}`).then(r => { if (active)
        setPeople(r.items.filter(u => u.id !== me()?.id)); }, e => { if (active)
        setError(e); }), 300); return () => { active = false; clearTimeout(timer); }; }, [q]);
    async function more() { if (!history.data?.next_cursor || loading)
        return; setLoading(true); try {
        const page = await get(`/calls?before=${history.data.next_cursor}`);
        history.set({ items: [...history.data.items, ...page.items], next_cursor: page.next_cursor });
    }
    catch (e) {
        setError(e);
    }
    finally {
        setLoading(false);
    } }
    return h("section", { class: "calls-page" },
        h("header", { class: "library-panel" },
            h("h1", null, "\u0417\u0432\u043E\u043D\u043A\u0438"),
            h("p", null, "\u041B\u0438\u0447\u043D\u044B\u0435 \u0430\u0443\u0434\u0438\u043E- \u0438 \u0432\u0438\u0434\u0435\u043E\u0437\u0432\u043E\u043D\u043A\u0438 \u0432 socialnet"),
            h("label", null,
                "\u041A\u043E\u043C\u0443 \u043F\u043E\u0437\u0432\u043E\u043D\u0438\u0442\u044C?",
                h("input", { type: "search", "aria-label": "\u041F\u043E\u0438\u0441\u043A \u0441\u043E\u0431\u0435\u0441\u0435\u0434\u043D\u0438\u043A\u0430", placeholder: "\u0418\u043C\u044F \u0438\u043B\u0438 username, \u043E\u0442 2 \u0441\u0438\u043C\u0432\u043E\u043B\u043E\u0432", maxLength: 64, value: q, onInput: e => setQ(e.currentTarget.value) })),
            people.map(u => h("div", { class: "call-history-row" },
                h(Avatar, { user: u }),
                h("a", { href: `/u/${u.username}` }, fullName(u)),
                h("button", { class: "ghost", onClick: () => startCall(u) }, "\u041F\u043E\u0437\u0432\u043E\u043D\u0438\u0442\u044C"),
                h("button", { class: "ghost", onClick: () => startCall(u, true) }, "\u0412\u0438\u0434\u0435\u043E"))),
            config.data && !config.data.relay_configured && h("p", { class: "muted" }, "TURN-\u0441\u0435\u0440\u0432\u0435\u0440 \u043D\u0435 \u043D\u0430\u0441\u0442\u0440\u043E\u0435\u043D. \u0421\u043E\u0435\u0434\u0438\u043D\u0435\u043D\u0438\u0435 \u043C\u0435\u0436\u0434\u0443 \u0440\u0430\u0437\u043D\u044B\u043C\u0438 \u0441\u0435\u0442\u044F\u043C\u0438 \u043C\u043E\u0436\u0435\u0442 \u0431\u044B\u0442\u044C \u043D\u0435\u0434\u043E\u0441\u0442\u0443\u043F\u043D\u043E.")),
        h("div", { class: "library-panel" },
            h("div", { class: "stats-toolbar" },
                h("h2", null, "\u0418\u0441\u0442\u043E\u0440\u0438\u044F \u0437\u0432\u043E\u043D\u043A\u043E\u0432"),
                h("select", { "aria-label": "\u0424\u0438\u043B\u044C\u0442\u0440 \u0437\u0432\u043E\u043D\u043A\u043E\u0432", value: filter, onChange: e => setFilter(e.currentTarget.value) },
                    h("option", { value: "all" }, "\u0412\u0441\u0435"),
                    h("option", { value: "active" }, "\u0410\u043A\u0442\u0438\u0432\u043D\u044B\u0435"),
                    h("option", { value: "missed" }, "\u041F\u0440\u043E\u043F\u0443\u0449\u0435\u043D\u043D\u044B\u0435")),
                h("button", { class: "ghost", onClick: async () => { if (!confirm("Очистить историю завершённых звонков только у вас?"))
                        return; try {
                        await api("DELETE", "/calls/history");
                        history.reload();
                    }
                    catch (e) {
                        setError(e);
                    } } }, "\u041E\u0447\u0438\u0441\u0442\u0438\u0442\u044C \u0438\u0441\u0442\u043E\u0440\u0438\u044E")),
            h(ErrorLine, { error: error || history.error }),
            !history.data ? h(Loading, null) : h(Fragment, null,
                history.data.items.filter(c => filter === "all" || (filter === "active" ? live(c) : c.status === "missed" && c.callee_id === me()?.id)).map(c => h("div", { class: "call-history-row", key: c.id },
                    h(Avatar, { user: c.peer }),
                    h("div", null,
                        h("a", { href: `/u/${c.peer.username}` }, fullName(c.peer)),
                        h("p", { class: c.status === "missed" ? "call-missed" : "muted" },
                            c.caller_id === me()?.id ? "↗" : "↙",
                            " ",
                            labels[c.status],
                            " \u00B7 ",
                            h(Time, { iso: c.created_at }))),
                    h("button", { class: "ghost", "aria-label": `Позвонить ${fullName(c.peer)}`, onClick: () => startCall(c.peer) }, "\u260E"),
                    h("button", { class: "ghost", "aria-label": `Видеозвонок ${fullName(c.peer)}`, onClick: () => startCall(c.peer, true) }, "\u0412\u0438\u0434\u0435\u043E"))),
                !history.data.items.length && h("p", { class: "empty" }, "\u0417\u0432\u043E\u043D\u043A\u043E\u0432 \u043F\u043E\u043A\u0430 \u043D\u0435\u0442. \u041D\u0430\u0439\u0434\u0438\u0442\u0435 \u0441\u043E\u0431\u0435\u0441\u0435\u0434\u043D\u0438\u043A\u0430 \u0438\u043B\u0438 \u043F\u043E\u0437\u0432\u043E\u043D\u0438\u0442\u0435 \u0438\u0437 \u0434\u0438\u0430\u043B\u043E\u0433\u0430."),
                history.data.next_cursor && h("button", { class: "ghost", disabled: loading, onClick: more }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u0435\u0449\u0451"))));
}
