import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { fetchBlob, fileSize } from "./api.js";
import { ErrorLine, duration } from "./ui.js";
const prefix = "sn-offline-music-";
const key = (id) => `/offline/music/${encodeURIComponent(id)}`;
function download(blob, t) { const url = URL.createObjectURL(blob); const a = document.createElement("a"); a.href = url; a.download = `${t.artist} - ${t.title}`.replace(/[\\/:*?"<>|]/g, "_") + ".mp3"; a.click(); setTimeout(() => URL.revokeObjectURL(url), 60000); }
export function MusicDownloads({ track: t, userId }) {
    const [saved, setSaved] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState(null);
    useEffect(() => { if (typeof caches !== "undefined")
        void caches.open(prefix + userId).then(c => c.match(key(t.id))).then(r => setSaved(!!r)).catch(() => { }); }, [t.id, userId]);
    async function run(save) {
        setBusy(true);
        setError(null);
        try {
            if (save && typeof caches === "undefined")
                throw new Error("Сохранение офлайн доступно по HTTPS или на localhost.");
            const cache = typeof caches !== "undefined" ? await caches.open(prefix + userId) : null;
            if (save && saved) {
                await cache.delete(key(t.id));
                setSaved(false);
            }
            else {
                const hit = await cache?.match(key(t.id));
                const blob = hit ? await hit.blob() : await fetchBlob(t.url);
                if (!blob.size)
                    throw new Error("Аудиофайл пуст.");
                if (save) {
                    await cache.put(key(t.id), new Response(blob, { headers: { "Content-Type": blob.type || "audio/mpeg", "X-Track": encodeURIComponent(JSON.stringify(t)) } }));
                    setSaved(true);
                }
                else
                    download(blob, t);
            }
            window.dispatchEvent(new Event("offline-music-change"));
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    }
    return h("div", { class: "music-download-actions" },
        h("button", { class: "link", disabled: busy, onClick: () => run(false) }, "\u0421\u043A\u0430\u0447\u0430\u0442\u044C \u0444\u0430\u0439\u043B"),
        h("button", { class: "link", disabled: busy, "aria-pressed": saved, onClick: () => run(true) }, busy ? "Загрузка…" : saved ? "Удалить офлайн" : "Сохранить офлайн"),
        h(ErrorLine, { error: error }));
}
export function OfflineMusic({ userId }) {
    const [items, setItems] = useState([]), [error, setError] = useState(null), [playing, setPlaying] = useState(null);
    useEffect(() => { let alive = true; async function load() { try {
        const result = [];
        if (typeof caches !== "undefined")
            for (const name of await caches.keys()) {
                if (!name.startsWith(prefix) || (userId !== undefined && name !== prefix + userId))
                    continue;
                const c = await caches.open(name);
                for (const req of await c.keys()) {
                    const r = await c.match(req);
                    if (r) {
                        const track = JSON.parse(decodeURIComponent(r.headers.get("X-Track") || ""));
                        result.push({ track, cache: name, size: (await r.blob()).size });
                    }
                }
            }
        if (alive)
            setItems(result);
    }
    catch (e) {
        if (alive)
            setError(e);
    } } void load(); window.addEventListener("offline-music-change", load); return () => { alive = false; window.removeEventListener("offline-music-change", load); }; }, [userId]);
    useEffect(() => () => { if (playing)
        URL.revokeObjectURL(playing.url); }, [playing]);
    async function play(s) { try {
        const r = await (await caches.open(s.cache)).match(key(s.track.id));
        if (!r)
            throw new Error("Копия удалена. Сохраните трек заново при подключении к сети.");
        setPlaying({ url: URL.createObjectURL(await r.blob()), title: s.track.title });
    }
    catch (e) {
        setError(e);
    } }
    return h("section", { class: "music-panel" },
        h("h2", null,
            "\u041C\u0443\u0437\u044B\u043A\u0430 \u043E\u0444\u043B\u0430\u0439\u043D \u00B7 ",
            items.length),
        h("p", { class: "muted" }, "\u0414\u043E\u0441\u0442\u0443\u043F\u043D\u0430 \u043D\u0430 \u044D\u0442\u043E\u043C \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0435. \u041A\u043E\u043F\u0438\u0438 \u0443\u0434\u0430\u043B\u044F\u044E\u0442\u0441\u044F \u043F\u0440\u0438 \u0432\u044B\u0445\u043E\u0434\u0435 \u0438\u0437 \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430 \u0438\u043B\u0438 \u043E\u0447\u0438\u0441\u0442\u043A\u0435 \u0434\u0430\u043D\u043D\u044B\u0445 \u0431\u0440\u0430\u0443\u0437\u0435\u0440\u0430. \u0411\u0440\u0430\u0443\u0437\u0435\u0440 \u0442\u0430\u043A\u0436\u0435 \u043C\u043E\u0436\u0435\u0442 \u043E\u0441\u0432\u043E\u0431\u043E\u0434\u0438\u0442\u044C \u043C\u0435\u0441\u0442\u043E \u0430\u0432\u0442\u043E\u043C\u0430\u0442\u0438\u0447\u0435\u0441\u043A\u0438."),
        h(ErrorLine, { error: error }),
        playing && h("div", null,
            h("strong", null, playing.title),
            h("audio", { key: playing.url, src: playing.url, controls: true, autoPlay: true, style: "width:100%" })),
        !items.length && h("p", null, "\u041D\u0430\u0436\u043C\u0438\u0442\u0435 \u00AB\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C \u043E\u0444\u043B\u0430\u0439\u043D\u00BB \u0440\u044F\u0434\u043E\u043C \u0441 \u0442\u0440\u0435\u043A\u043E\u043C \u043F\u0440\u0438 \u043F\u043E\u0434\u043A\u043B\u044E\u0447\u0435\u043D\u0438\u0438 \u043A \u0438\u043D\u0442\u0435\u0440\u043D\u0435\u0442\u0443."),
        h("ul", { class: "plain-list" }, items.map(s => h("li", { key: s.cache + s.track.id, class: "offline-music-row" },
            h("button", { class: "link", onClick: () => play(s) },
                "\u25B6 ",
                s.track.artist,
                " \u2014 ",
                s.track.title),
            h("small", { class: "muted" },
                duration(s.track.duration_sec),
                " \u00B7 ",
                fileSize(s.size)),
            h(MusicDownloads, { track: s.track, userId: Number(s.cache.slice(prefix.length)) })))));
}
