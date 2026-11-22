import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { fetchBlob, fileSize } from "./api.js";
import { ErrorLine } from "./ui.js";
const prefix = "sn-offline-video-";
const key = (id) => `/offline/video/${encodeURIComponent(id)}`;
export function VideoDownloads({ video, userId }) {
    const [busy, setBusy] = useState(false), [saved, setSaved] = useState(false), [error, setError] = useState(null);
    useEffect(() => { let alive = true; setSaved(false); if (typeof caches !== "undefined")
        void caches.open(prefix + userId).then(c => c.match(key(video.id))).then(r => { if (alive)
            setSaved(!!r); }).catch(setError); return () => { alive = false; }; }, [video.id, userId]);
    async function run(offline) {
        setBusy(true);
        setError(null);
        try {
            const cache = typeof caches !== "undefined" ? await caches.open(prefix + userId) : null;
            if (offline && !cache)
                throw new Error("Для офлайн-сохранения нужен HTTPS или localhost.");
            if (offline && saved) {
                await cache.delete(key(video.id));
                setSaved(false);
            }
            else {
                const hit = await cache?.match(key(video.id));
                if (!hit && !video.mp4_url)
                    throw new Error("MP4 ещё не готов. Дождитесь обработки видео.");
                const blob = hit ? await hit.blob() : await fetchBlob(video.mp4_url);
                if (!blob.size)
                    throw new Error("Видео пустое.");
                if (offline) {
                    await cache.put(key(video.id), new Response(blob, { headers: { "Content-Type": "video/mp4", "X-Video": encodeURIComponent(JSON.stringify(video)) } }));
                    setSaved(true);
                }
                else {
                    const url = URL.createObjectURL(blob);
                    const a = document.createElement("a");
                    a.href = url;
                    a.download = video.title.replace(/[\\/:*?"<>|]/g, "_") + ".mp4";
                    a.click();
                    setTimeout(() => URL.revokeObjectURL(url), 60000);
                }
            }
            window.dispatchEvent(new Event("offline-video-change"));
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    }
    return h("div", { class: "music-download-actions" },
        h("button", { disabled: busy, onClick: () => run(false) }, "\u0421\u043A\u0430\u0447\u0430\u0442\u044C MP4"),
        h("button", { disabled: busy, onClick: () => run(true) }, busy ? "Подождите…" : saved ? "Удалить офлайн" : "Сохранить офлайн"),
        h(ErrorLine, { error: error }));
}
export function OfflineVideos() {
    const [items, setItems] = useState([]), [playing, setPlaying] = useState(null), [error, setError] = useState(null);
    useEffect(() => { let alive = true; async function load() { try {
        const result = [];
        if (typeof caches !== "undefined")
            for (const name of await caches.keys()) {
                if (!name.startsWith(prefix))
                    continue;
                const c = await caches.open(name);
                for (const req of await c.keys()) {
                    const r = await c.match(req);
                    if (r)
                        result.push({ clip: JSON.parse(decodeURIComponent(r.headers.get("X-Video") || "")), user: Number(name.slice(prefix.length)), size: (await r.blob()).size });
                }
            }
        if (alive)
            setItems(result);
    }
    catch (e) {
        if (alive)
            setError(e);
    } } void load(); window.addEventListener("offline-video-change", load); return () => { alive = false; window.removeEventListener("offline-video-change", load); }; }, []);
    useEffect(() => () => { if (playing)
        URL.revokeObjectURL(playing.url); }, [playing]);
    async function play(clip, user) { try {
        const r = await (await caches.open(prefix + user)).match(key(clip.id));
        if (!r)
            throw new Error("Сохранённая копия удалена.");
        setPlaying({ url: URL.createObjectURL(await r.blob()), title: clip.title });
    }
    catch (e) {
        setError(e);
    } }
    return h("section", null,
        h("h2", null,
            "\u0412\u0438\u0434\u0435\u043E \u043E\u0444\u043B\u0430\u0439\u043D \u00B7 ",
            items.length),
        h("p", null, "\u041A\u043E\u043F\u0438\u0438 \u0445\u0440\u0430\u043D\u044F\u0442\u0441\u044F \u043D\u0430 \u044D\u0442\u043E\u043C \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0435 \u0438 \u0443\u0434\u0430\u043B\u044F\u044E\u0442\u0441\u044F \u043F\u0440\u0438 \u0432\u044B\u0445\u043E\u0434\u0435 \u0438\u0437 \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430 \u0438\u043B\u0438 \u043E\u0447\u0438\u0441\u0442\u043A\u0435 \u0431\u0440\u0430\u0443\u0437\u0435\u0440\u0430. \u0421\u043A\u0430\u0447\u0430\u043D\u043D\u044B\u0439 MP4 \u043C\u043E\u0436\u043D\u043E \u043E\u0442\u043A\u0440\u044B\u0442\u044C \u043E\u0442\u0434\u0435\u043B\u044C\u043D\u043E."),
        h(ErrorLine, { error: error }),
        playing && h("div", null,
            h("h3", null, playing.title),
            h("video", { key: playing.url, controls: true, autoPlay: true, src: playing.url, style: "width:100%;max-height:65vh" })),
        items.map(x => h("article", { key: x.user + ":" + x.clip.id },
            h("button", { onClick: () => play(x.clip, x.user) },
                "\u25B6 ",
                x.clip.title),
            " ",
            h("small", null, fileSize(x.size)),
            h(VideoDownloads, { video: x.clip, userId: x.user }))),
        !items.length && h("p", null, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u0435 \u0432\u0438\u0434\u0435\u043E \u0432 \u0440\u0430\u0437\u0434\u0435\u043B\u0435 \u00AB\u0412\u0438\u0434\u0435\u043E\u00BB \u043F\u0440\u0438 \u043F\u043E\u0434\u043A\u043B\u044E\u0447\u0435\u043D\u0438\u0438 \u043A \u0441\u0435\u0442\u0438."));
}
