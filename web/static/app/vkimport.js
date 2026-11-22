import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { ApiError, get, post, upload } from "./api.js";
import { me } from "./session.js";
import { ErrorLine } from "./ui.js";
const uuid = /^[a-f0-9-]{36}$/i;
function bridge(op, key, signal) {
    return new Promise((resolve, reject) => {
        const id = crypto.randomUUID();
        const chunks = [];
        let size = 0, index = 0;
        const finish = (error, value) => { clearTimeout(timer); window.removeEventListener("message", receive); signal?.removeEventListener("abort", cancel); if (error) {
            window.postMessage({ channel: "socialnet-vk:request", id, op: "cancel" }, location.origin);
            reject(error);
        }
        else
            resolve(value); };
        const cancel = () => finish(new Error("Импорт остановлен"));
        const receive = (e) => {
            const m = e.data;
            if (e.source !== window || e.origin !== location.origin || m?.channel !== "socialnet-vk:response" || m.id !== id)
                return;
            try {
                if (m.type === "error")
                    finish(new Error(typeof m.error === "string" ? m.error : "Ошибка расширения"));
                else if (op === "catalogue" && m.type === "catalogue")
                    finish(undefined, m.items);
                else if (op === "audio" && m.type === "chunk") {
                    if (m.index !== index++ || typeof m.data !== "string" || m.data.length > 90000)
                        throw Error("Повреждённый ответ расширения");
                    const decoded = atob(m.data);
                    size += decoded.length;
                    if (size > 50 * 1024 * 1024)
                        throw Error("Максимум 50 МБ на трек");
                    chunks.push(Uint8Array.from(decoded, c => c.charCodeAt(0)));
                }
                else if (op === "audio" && m.type === "done") {
                    if (size !== m.bytes || !size || !["mp3", "m4a", "ogg", "flac", "wav"].includes(m.ext))
                        throw Error("Аудиофайл получен не полностью");
                    finish(undefined, new File(chunks, `vk_${key}.${m.ext}`, { type: m.mime }));
                }
            }
            catch (e) {
                finish(e instanceof Error ? e : new Error("Неверный ответ расширения"));
            }
        };
        const timer = setTimeout(() => finish(new Error(op === "catalogue" ? "Расширение не отвечает. Установите его, разрешите перенос в эту вкладку и повторите." : "Истекло время загрузки трека. Обновите список в VK.")), op === "catalogue" ? 8000 : 125000);
        window.addEventListener("message", receive);
        signal?.addEventListener("abort", cancel, { once: true });
        if (signal?.aborted) {
            cancel();
            return;
        }
        window.postMessage({ channel: "socialnet-vk:request", id, op, key }, location.origin);
    });
}
export function VKMusicImport({ done }) {
    const [items, setItems] = useState([]);
    const [selected, setSelected] = useState([]);
    const [busy, setBusy] = useState(false);
    const [reading, setReading] = useState(false);
    const [error, setError] = useState(null);
    const [results, setResults] = useState([]);
    const [status, setStatus] = useState("");
    const control = useRef(null);
    const running = useRef(false);
    const mounted = useRef(true);
    useEffect(() => () => { mounted.current = false; control.current?.abort(); }, []);
    async function load() {
        if (reading || running.current)
            return;
        setReading(true);
        setError(null);
        try {
            const raw = await bridge("catalogue");
            if (!Array.isArray(raw) || raw.length > 5000)
                throw Error("Неверный список расширения");
            const parsed = raw.map((t) => {
                if (typeof t.key !== "string" || !/^-?\d+_\d+$/.test(t.key) || typeof t.title !== "string" || !t.title || t.title.length > 200 || typeof t.artist !== "string" || t.artist.length > 200 || !Array.isArray(t.playlists) || t.playlists.length > 20 || t.playlists.some((p) => typeof p !== "string" || !p || p.length > 100))
                    throw Error("Некорректные данные трека");
                return { key: t.key, title: t.title, artist: t.artist, playlists: t.playlists, available: t.available === true, reason: typeof t.reason === "string" ? t.reason : "Нет доступного файла" };
            });
            if (mounted.current) {
                setItems(parsed);
                setSelected([]);
                setResults([]);
                setStatus(parsed.length ? "Выберите записи для переноса." : "Список пуст. Сначала добавьте записи из VK через значок расширения.");
            }
        }
        catch (e) {
            if (mounted.current)
                setError(e);
        }
        finally {
            if (mounted.current)
                setReading(false);
        }
    }
    function exportList() { const data = items.map(t => ({ file: `vk_${t.key}.mp3`, title: t.title, artist: t.artist, playlists: t.playlists, vk_id: t.key })); const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: "application/json" })); const a = document.createElement("a"); a.href = url; a.download = "vk-music-library.json"; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000); }
    async function run() {
        const user = me()?.id;
        if (running.current || !user || !selected.length)
            return;
        running.current = true;
        setBusy(true);
        setError(null);
        setResults([]);
        control.current = new AbortController();
        const lines = [];
        let success = 0;
        const storageKey = `sn-vk-import-${user}`;
        try {
            const saved = JSON.parse(localStorage.getItem(storageKey) || '{"tracks":{},"playlists":{}}');
            const progress = { tracks: {}, playlists: {} };
            for (const [key, value] of Object.entries(saved.tracks || {}))
                if (typeof value === "string" && uuid.test(value))
                    progress.tracks[key] = value;
            for (const [key, value] of Object.entries(saved.playlists || {}))
                if (typeof value === "number" && Number.isSafeInteger(value) && value > 0)
                    progress.playlists[key] = value;
            const checkpoint = () => localStorage.setItem(storageKey, JSON.stringify(progress));
            checkpoint();
            const check = () => { if (control.current?.signal.aborted || me()?.id !== user)
                throw Error("Импорт остановлен"); };
            for (const t of items.filter(t => selected.includes(t.key) && t.available)) {
                if (control.current.signal.aborted || me()?.id !== user)
                    break;
                try {
                    setStatus(`Перенос: ${t.artist} — ${t.title}`);
                    let tid = progress.tracks[t.key];
                    if (tid) {
                        try {
                            await get(`/tracks/${tid}`);
                        }
                        catch (e) {
                            if (e instanceof ApiError && e.status === 404) {
                                delete progress.tracks[t.key];
                                tid = "";
                            }
                            else
                                throw e;
                        }
                    }
                    if (!tid) {
                        const file = await bridge("audio", t.key, control.current.signal);
                        check();
                        const uploaded = await upload("/tracks", file, { title: t.title, artist: t.artist });
                        tid = uploaded.id;
                        progress.tracks[t.key] = tid;
                        checkpoint();
                    }
                    for (const name of t.playlists) {
                        check();
                        let pid = progress.playlists[name];
                        if (pid) {
                            try {
                                await get(`/playlists/${pid}`);
                            }
                            catch (e) {
                                if (e instanceof ApiError && e.status === 404) {
                                    delete progress.playlists[name];
                                    pid = 0;
                                }
                                else
                                    throw e;
                            }
                        }
                        if (!pid) {
                            pid = (await post("/playlists", { title: name })).id;
                            progress.playlists[name] = pid;
                            checkpoint();
                        }
                        check();
                        await post(`/playlists/${pid}/tracks`, { track_id: tid });
                    }
                    success++;
                    lines.push(`${t.artist} — ${t.title}: перенесён`);
                }
                catch (e) {
                    lines.push(`${t.title}: ${e instanceof Error ? e.message : "ошибка"}`);
                }
                if (mounted.current)
                    setResults([...lines]);
            }
            if (mounted.current)
                setStatus(`${control.current.signal.aborted ? "Остановлено" : "Готово"}. Перенесено: ${success}. Проверьте отчёт ниже.`);
            if (me()?.id === user)
                done();
        }
        catch (e) {
            if (mounted.current)
                setError(e);
        }
        finally {
            running.current = false;
            if (mounted.current)
                setBusy(false);
        }
    }
    return h("details", { class: "music-panel vk-import" },
        h("summary", null, "\u0418\u043C\u043F\u043E\u0440\u0442 \u0438\u0437 \u043E\u0442\u043A\u0440\u044B\u0442\u043E\u0433\u043E VK"),
        h("p", null, "\u041F\u0435\u0440\u0435\u043D\u043E\u0441 \u0447\u0435\u0440\u0435\u0437 \u0440\u0430\u0441\u0448\u0438\u0440\u0435\u043D\u0438\u0435 \u0432 \u0432\u0430\u0448\u0435\u043C \u0431\u0440\u0430\u0443\u0437\u0435\u0440\u0435. \u041F\u0430\u0440\u043E\u043B\u044C \u0438 cookies VK \u0432 socialnet \u043D\u0435 \u043F\u0435\u0440\u0435\u0434\u0430\u044E\u0442\u0441\u044F."),
        h("ol", null,
            h("li", null,
                h("a", { href: "/static/downloads/socialnet-vk-import.zip", download: true }, "\u0421\u043A\u0430\u0447\u0430\u0442\u044C \u0440\u0430\u0441\u0448\u0438\u0440\u0435\u043D\u0438\u0435"),
                ", \u0440\u0430\u0441\u043F\u0430\u043A\u043E\u0432\u0430\u0442\u044C. \u0412 Chrome: chrome://extensions \u2192 \u0440\u0435\u0436\u0438\u043C \u0440\u0430\u0437\u0440\u0430\u0431\u043E\u0442\u0447\u0438\u043A\u0430 \u2192 \u00AB\u0417\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044C \u0440\u0430\u0441\u043F\u0430\u043A\u043E\u0432\u0430\u043D\u043D\u043E\u0435 \u0440\u0430\u0441\u0448\u0438\u0440\u0435\u043D\u0438\u0435\u00BB."),
            h("li", null, "\u041E\u0442\u043A\u0440\u044B\u0442\u044C \u0441\u0432\u043E\u044E \u043C\u0443\u0437\u044B\u043A\u0443 \u0432 VK, \u043F\u0440\u043E\u043A\u0440\u0443\u0442\u0438\u0442\u044C \u0441\u043F\u0438\u0441\u043E\u043A \u0438 \u043D\u0430\u0436\u0430\u0442\u044C \u0437\u043D\u0430\u0447\u043E\u043A \u0440\u0430\u0441\u0448\u0438\u0440\u0435\u043D\u0438\u044F \u2192 \u00AB\u0414\u043E\u0431\u0430\u0432\u0438\u0442\u044C \u0437\u0430\u043F\u0438\u0441\u0438\u00BB. \u0414\u043B\u044F \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442\u043E\u0432 \u043F\u043E\u0432\u0442\u043E\u0440\u0438\u0442\u044C \u043D\u0430 \u0438\u0445 \u0441\u0442\u0440\u0430\u043D\u0438\u0446\u0430\u0445, \u0443\u043A\u0430\u0437\u0430\u0432 \u043D\u0430\u0437\u0432\u0430\u043D\u0438\u0435."),
            h("li", null, "\u0412\u0435\u0440\u043D\u0443\u0442\u044C\u0441\u044F \u0432 \u044D\u0442\u0443 \u0432\u043A\u043B\u0430\u0434\u043A\u0443, \u0432 \u0440\u0430\u0441\u0448\u0438\u0440\u0435\u043D\u0438\u0438 \u043D\u0430\u0436\u0430\u0442\u044C \u00AB\u0420\u0430\u0437\u0440\u0435\u0448\u0438\u0442\u044C \u043F\u0435\u0440\u0435\u043D\u043E\u0441\u00BB, \u0437\u0430\u0442\u0435\u043C \u043F\u043E\u043B\u0443\u0447\u0438\u0442\u044C \u0441\u043F\u0438\u0441\u043E\u043A \u043D\u0438\u0436\u0435.")),
        h("p", { class: "muted" }, "\u041F\u043E\u0434\u0434\u0435\u0440\u0436\u0438\u0432\u0430\u044E\u0442\u0441\u044F localhost:8080 \u0438 127.0.0.1:8080. \u0421\u043E\u0431\u0438\u0440\u0430\u044E\u0442\u0441\u044F \u0442\u043E\u043B\u044C\u043A\u043E \u0437\u0430\u0433\u0440\u0443\u0436\u0435\u043D\u043D\u044B\u0435 \u043D\u0430 \u0441\u0442\u0440\u0430\u043D\u0438\u0446\u0435 \u0437\u0430\u043F\u0438\u0441\u0438. \u0410\u0443\u0434\u0438\u043E\u0444\u0430\u0439\u043B\u044B HLS, \u0437\u0430\u0449\u0438\u0449\u0451\u043D\u043D\u044B\u0435 \u0438 \u043D\u0435\u0434\u043E\u0441\u0442\u0443\u043F\u043D\u044B\u0435 \u0441\u0441\u044B\u043B\u043A\u0438 \u043F\u043E\u043A\u0430 \u043D\u0435 \u043F\u0435\u0440\u0435\u043D\u043E\u0441\u044F\u0442\u0441\u044F. \u041D\u0430\u0437\u0432\u0430\u043D\u0438\u044F \u0442\u0430\u043A\u0438\u0445 \u0442\u0440\u0435\u043A\u043E\u0432 \u043C\u043E\u0436\u043D\u043E \u0441\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C \u0432 JSON."),
        h("button", { disabled: busy || reading, onClick: load }, reading ? "Получаем список…" : "Получить список из VK"),
        h(ErrorLine, { error: error }),
        h("p", { role: "status" }, status),
        !!items.length && h("div", null,
            h("p", null,
                "\u0412\u0441\u0435\u0433\u043E: ",
                items.length,
                ". \u0414\u043E\u0441\u0442\u0443\u043F\u0435\u043D \u043F\u0440\u044F\u043C\u043E\u0439 \u0444\u0430\u0439\u043B: ",
                items.filter(t => t.available).length,
                ". \u0412\u044B\u0431\u0440\u0430\u043D\u043E: ",
                selected.length,
                "."),
            h("button", { disabled: busy, onClick: () => setSelected(items.filter(t => t.available).map(t => t.key)) }, "\u0412\u044B\u0431\u0440\u0430\u0442\u044C \u0434\u043E\u0441\u0442\u0443\u043F\u043D\u044B\u0435"),
            h("button", { disabled: busy, onClick: () => setSelected([]) }, "\u0421\u043D\u044F\u0442\u044C \u0432\u044B\u0431\u043E\u0440"),
            h("button", { onClick: exportList }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C \u043D\u0430\u0437\u0432\u0430\u043D\u0438\u044F \u0438 \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442\u044B \u0432 JSON"),
            h("div", { class: "vk-import-list" }, items.map(t => h("label", { key: t.key },
                h("input", { type: "checkbox", disabled: busy || !t.available, checked: selected.includes(t.key), onChange: e => setSelected(old => e.currentTarget.checked ? [...old, t.key] : old.filter(k => k !== t.key)) }),
                h("span", null,
                    h("strong", null,
                        t.artist,
                        " \u2014 ",
                        t.title),
                    h("small", null,
                        t.playlists.join(" · "),
                        !t.available && ` · ${t.reason}`))))),
            h("p", { class: "muted" }, "\u0414\u043E 50 \u041C\u0411 \u043D\u0430 \u0442\u0440\u0435\u043A. \u041F\u0440\u043E\u0433\u0440\u0435\u0441\u0441 \u0441\u043E\u0445\u0440\u0430\u043D\u044F\u0435\u0442\u0441\u044F \u0432 \u044D\u0442\u043E\u043C \u0431\u0440\u0430\u0443\u0437\u0435\u0440\u0435 \u0434\u043B\u044F \u0442\u0435\u043A\u0443\u0449\u0435\u0433\u043E \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430. \u041F\u0440\u0438 \u043F\u043E\u0442\u0435\u0440\u0435 \u043E\u0442\u0432\u0435\u0442\u0430 \u0441\u0435\u0440\u0432\u0435\u0440\u0430 \u043F\u043E\u0432\u0442\u043E\u0440 \u043C\u043E\u0436\u0435\u0442 \u0441\u043E\u0437\u0434\u0430\u0442\u044C \u0434\u0443\u0431\u043B\u0438\u043A\u0430\u0442 \u2014 \u043F\u0440\u043E\u0432\u0435\u0440\u044C\u0442\u0435 \u0431\u0438\u0431\u043B\u0438\u043E\u0442\u0435\u043A\u0443. \u041E\u0441\u0442\u0430\u043D\u043E\u0432\u043A\u0430 \u0437\u0430\u0432\u0435\u0440\u0448\u0430\u0435\u0442 \u0443\u0436\u0435 \u043D\u0430\u0447\u0430\u0442\u0443\u044E \u0437\u0430\u0433\u0440\u0443\u0437\u043A\u0443 \u0432 socialnet."),
            h("button", { disabled: busy || !selected.length, onClick: run }, busy ? "Перенос…" : "Импортировать выбранные"),
            busy && h("button", { onClick: () => { control.current?.abort(); setStatus("Останавливаем перенос…"); } }, "\u041E\u0441\u0442\u0430\u043D\u043E\u0432\u0438\u0442\u044C")),
        h("ul", null, results.map((line, i) => h("li", { key: i }, line))));
}
