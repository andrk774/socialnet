import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { del, get, post } from "./api.js";
import { Empty, ErrorLine, Loading, duration, useLoad } from "./ui.js";
import { MediaUpload } from "./uploads.js";
import { Icon } from "./shellparts.js";
import { MusicDownloads, OfflineMusic } from "./offlinemusic.js";
import { MusicImport } from "./musicimport.js";
import { VKMusicImport } from "./vkimport.js";
export function MusicPage({ userId, active = true, onPlayerChange }) {
    const [tab, setTab] = useState("home");
    const [tracks, setTracks] = useState([]);
    const [cursor, setCursor] = useState(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    const playlists = useLoad(() => get(`/users/${userId}/playlists`), [userId]);
    const [selected, setSelected] = useState(null);
    const [query, setQuery] = useState("");
    const [search, setSearch] = useState("");
    const results = useLoad(() => search.length >= 2 ? get(`/search?type=tracks&q=${encodeURIComponent(search)}`) : Promise.resolve(null), [search]);
    const uploadDialog = useRef(null);
    const createDialog = useRef(null);
    const mixDialog = useRef(null);
    const addDialog = useRef(null);
    const [title, setTitle] = useState("");
    const [saving, setSaving] = useState(false);
    const [dialogError, setDialogError] = useState(null);
    const [addTrack, setAddTrack] = useState(null);
    const [queue, setQueue] = useState([]);
    const [position, setPosition] = useState(0);
    const [source, setSource] = useState("mine");
    const [order, setOrder] = useState("shuffle");
    const [draftSource, setDraftSource] = useState("mine");
    const [draftOrder, setDraftOrder] = useState("shuffle");
    const audio = useRef(null);
    const playing = queue[position];
    const [paused, setPaused] = useState(true);
    const [elapsed, setElapsed] = useState(0);
    const [length, setLength] = useState(0);
    const [volume, setVolume] = useState(1);
    useEffect(() => { setElapsed(0); setLength(playing?.duration_sec ?? 0); }, [playing?.id]);
    const [repeat, setRepeat] = useState(false);
    const [shuffle, setShuffle] = useState(false);
    const [recent, setRecent] = useState(() => { try {
        const data = JSON.parse(localStorage.getItem(`sn-listening-${userId}`) ?? "[]");
        return Array.isArray(data) ? data.filter(t => typeof t?.id === "string" && typeof t?.title === "string" && typeof t?.url === "string").slice(0, 24) : [];
    }
    catch {
        return [];
    } });
    function heard(t) { setRecent(old => { const next = [t, ...old.filter(x => x.id !== t.id)].slice(0, 24); try {
        localStorage.setItem(`sn-listening-${userId}`, JSON.stringify(next));
    }
    catch { } return next; }); }
    function nextTrack() { if (shuffle && queue.length > 1) {
        setPosition((position + 1 + Math.floor(Math.random() * (queue.length - 1))) % queue.length);
    }
    else if (position + 1 < queue.length)
        setPosition(position + 1); }
    async function removeCurrent() { if (!playing)
        return; await act(async () => { await del(`/tracks/${playing.id}/my-music`); await load(); }); }
    useEffect(() => { if (playing)
        void audio.current?.play().catch(() => { }); }, [queue, position]);
    useEffect(() => { const element = audio.current; return () => element?.pause(); }, [playing?.id]);
    async function load(after) {
        setBusy(true);
        setError(null);
        try {
            const page = await get(`/users/${userId}/music?limit=50${after ? `&before=${after}` : ""}`);
            setTracks((old) => after ? [...old, ...page.items] : page.items);
            setCursor(page.next_cursor);
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    }
    useEffect(() => { void load(); }, [userId]);
    useEffect(() => { const timer = setTimeout(() => setSearch(query.trim()), 300); return () => clearTimeout(timer); }, [query]);
    const act = async (fn) => { setError(null); try {
        await fn();
    }
    catch (e) {
        setError(e);
    } };
    const play = (items, index = 0) => { setQueue(items); setPosition(index); };
    const toggleTrack = (items, index) => {
        const el = audio.current;
        if (playing?.id !== items[index].id || !el) {
            setPaused(true);
            play(items, index);
            return;
        }
        if (el.paused)
            void el.play().catch(() => setError(new Error("Нажмите воспроизведение в плеере.")));
        else
            el.pause();
    };
    useEffect(() => { onPlayerChange?.(playing ? { title: playing.title, artist: playing.artist, paused, previousEnabled: position > 0, nextEnabled: shuffle ? queue.length > 1 : position + 1 < queue.length, previous: () => setPosition(p => Math.max(0, p - 1)), toggle: () => toggleTrack(queue, position), next: nextTrack } : null); }, [queue, position, paused, shuffle, onPlayerChange]);
    useEffect(() => () => onPlayerChange?.(null), [onPlayerChange]);
    useEffect(() => { if (!active) {
        for (const ref of [uploadDialog, createDialog, mixDialog, addDialog])
            ref.current?.close();
    } }, [active]);
    async function mix() {
        setBusy(true);
        setError(null);
        try {
            let items;
            if (source === "mine") {
                items = [];
                let before = null;
                do {
                    const page = await get(`/users/${userId}/music?limit=50${before ? `&before=${before}` : ""}`);
                    items.push(...page.items);
                    before = page.next_cursor;
                } while (before !== null);
            }
            else
                items = (await get(`/playlists/${source}`)).tracks ?? [];
            if (order === "shuffle")
                for (let i = items.length - 1; i > 0; i--) {
                    const j = Math.floor(Math.random() * (i + 1));
                    [items[i], items[j]] = [items[j], items[i]];
                }
            if (!items.length)
                throw new Error("В выбранной коллекции пока нет треков. Добавьте музыку и попробуйте снова.");
            play(items);
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    }
    async function create(e) {
        e.preventDefault();
        if (saving)
            return;
        setSaving(true);
        setDialogError(null);
        try {
            const pl = await post("/playlists", { title });
            playlists.reload();
            setSelected(pl);
            setTab("playlists");
            setQuery("");
            createDialog.current?.close();
            setTitle("");
        }
        catch (err) {
            setDialogError(err);
        }
        finally {
            setSaving(false);
        }
    }
    const openPlaylist = (id) => act(async () => { setSelected(await get(`/playlists/${id}`)); setTab("playlists"); setQuery(""); });
    const available = selected && tab === "playlists" ? selected.tracks ?? [] : tracks;
    const visible = search.length >= 2 ? results.data?.items ?? [] : available;
    const rows = (items) => h("ol", { class: "tracks music-tracks" }, items.map((t, i) => h("li", { key: t.id },
        h("button", { class: playing?.id === t.id ? "track-btn on" : "track-btn", "aria-label": `${playing?.id === t.id && !paused ? "Пауза" : "Слушать"}: ${t.artist} — ${t.title}`, onClick: () => toggleTrack(items, i) }, playing?.id === t.id && !paused ? "Ⅱ" : "▶"),
        h("span", { class: "music-track-title" },
            h("strong", null, t.title),
            h("small", null, t.artist),
            h("span", { class: "sr-only" },
                t.artist,
                " \u2014 ",
                t.title)),
        h("span", { class: "muted" }, duration(t.duration_sec)),
        h("div", { class: "music-track-actions" },
            h("button", { class: "icon-btn", title: "\u0414\u043E\u0431\u0430\u0432\u0438\u0442\u044C \u0432 \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442", "aria-label": `Добавить в плейлист: ${t.title}`, onClick: () => { setAddTrack(t); setDialogError(null); addDialog.current?.showModal(); } },
                h(Icon, { name: "plus" })),
            search.length >= 2 && !t.in_my_music && !tracks.some((x) => x.id === t.id) && h("button", { class: "link", onClick: () => act(async () => { await post(`/tracks/${t.id}/my-music`); await load(); results.reload(); }) }, "\u0421\u0435\u0431\u0435"),
            selected && tab === "playlists" && search.length < 2 && h("button", { class: "icon-btn", "aria-label": `Убрать из плейлиста: ${t.title}`, onClick: () => act(async () => { await del(`/playlists/${selected.id}/tracks/${t.id}`); setSelected(await get(`/playlists/${selected.id}`)); playlists.reload(); }) }, "\u00D7")),
        h(MusicDownloads, { track: t, userId: userId }))));
    const cards = h("div", { class: "music-playlists" }, playlists.data?.items.map((pl, i) => h("button", { key: pl.id, class: `playlist-card tone-${i % 4}`, onClick: () => openPlaylist(pl.id) },
        h("span", { class: "playlist-art" }, "\u266B"),
        h("strong", null, pl.title),
        h("small", null,
            "\u0422\u0440\u0435\u043A\u043E\u0432: ",
            pl.track_count))));
    const dialogClose = (ref) => h("button", { type: "button", class: "music-dialog-close", "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C \u043E\u043A\u043D\u043E", onClick: () => ref.current?.close() }, "\u00D7");
    return h("section", { class: "music-page" },
        h("h1", { class: "sr-only" }, "\u041C\u0443\u0437\u044B\u043A\u0430"),
        playing && h("div", { class: "music-player-dock" },
            h("div", { class: "player music-player" },
                h("div", { class: "music-now" },
                    h("button", { class: "icon-btn", "aria-label": "\u041F\u0440\u0435\u0434\u044B\u0434\u0443\u0449\u0438\u0439 \u0442\u0440\u0435\u043A", disabled: position === 0, onClick: () => setPosition(position - 1) }, "\u2039"),
                    h("span", null,
                        h("strong", null, playing.title),
                        h("small", null,
                            playing.artist,
                            " \u00B7 ",
                            position + 1,
                            " / ",
                            queue.length)),
                    h("button", { class: "icon-btn", "aria-label": "\u0421\u043B\u0435\u0434\u0443\u044E\u0449\u0438\u0439 \u0442\u0440\u0435\u043A", disabled: !shuffle && position + 1 >= queue.length, onClick: nextTrack }, "\u203A")),
                h("audio", { ref: audio, key: playing.id, autoPlay: true, loop: repeat, src: playing.url, onDurationChange: (e) => { if (Number.isFinite(e.currentTarget.duration) && e.currentTarget.duration > 0)
                        setLength(e.currentTarget.duration); }, onTimeUpdate: (e) => setElapsed(e.currentTarget.currentTime), onLoadedMetadata: (e) => { setLength(Number.isFinite(e.currentTarget.duration) && e.currentTarget.duration > 0 ? e.currentTarget.duration : playing.duration_sec); e.currentTarget.volume = volume; setElapsed(0); }, onPlay: () => { setPaused(false); heard(playing); }, onPause: () => setPaused(true), onEnded: () => { setPaused(true); nextTrack(); }, onError: () => setError(new Error("Не удалось воспроизвести трек. Попробуйте следующий.")) })),
            h("div", { class: "music-playback-controls" },
                h("button", { class: "primary playback-toggle", "aria-label": paused ? "Воспроизвести" : "Пауза", onClick: () => toggleTrack(queue, position) }, paused ? "▶" : "Ⅱ"),
                h("button", { class: "icon-btn", title: "\u0412\u043F\u0435\u0440\u0435\u043C\u0435\u0448\u043A\u0443", "aria-label": "\u0412\u043F\u0435\u0440\u0435\u043C\u0435\u0448\u043A\u0443", "aria-pressed": shuffle, onClick: () => setShuffle(!shuffle) }, "\u21C4"),
                h("button", { class: "icon-btn", title: "\u041F\u043E\u0432\u0442\u043E\u0440 \u0442\u0440\u0435\u043A\u0430", "aria-label": "\u041F\u043E\u0432\u0442\u043E\u0440 \u0442\u0440\u0435\u043A\u0430", "aria-pressed": repeat, onClick: () => setRepeat(!repeat) }, "\u21BB"),
                h("span", { class: "playback-time" },
                    duration(elapsed),
                    " / ",
                    duration(length || playing.duration_sec)),
                h("input", { type: "range", min: 0, max: length || 1, step: 0.1, value: Math.min(elapsed, length || 1), disabled: !length, "aria-label": "\u041F\u043E\u0437\u0438\u0446\u0438\u044F \u0432\u043E\u0441\u043F\u0440\u043E\u0438\u0437\u0432\u0435\u0434\u0435\u043D\u0438\u044F", onInput: (e) => { const value = Number(e.currentTarget.value); if (audio.current)
                        audio.current.currentTime = value; setElapsed(value); } }),
                h("label", { class: "playback-volume" },
                    "\u0413\u0440\u043E\u043C\u043A\u043E\u0441\u0442\u044C",
                    h("input", { type: "range", min: 0, max: 1, step: 0.05, value: volume, "aria-label": "\u0413\u0440\u043E\u043C\u043A\u043E\u0441\u0442\u044C", onInput: (e) => { const value = Number(e.currentTarget.value); setVolume(value); if (audio.current)
                            audio.current.volume = value; } })),
                tracks.some(t => t.id === playing.id) && h("button", { class: "icon-btn", title: "\u0423\u0434\u0430\u043B\u0438\u0442\u044C \u0438\u0437 \u043C\u043E\u0435\u0439 \u043C\u0443\u0437\u044B\u043A\u0438", "aria-label": "\u0423\u0434\u0430\u043B\u0438\u0442\u044C \u0438\u0437 \u043C\u043E\u0435\u0439 \u043C\u0443\u0437\u044B\u043A\u0438", onClick: removeCurrent }, "\u2713"))),
        h("div", { class: "music-panel music-top" },
            h(MusicImport, { done: () => { void load(); playlists.reload(); } }),
            h(VKMusicImport, { done: () => { void load(); playlists.reload(); } }),
            h("div", { class: "music-navigation" },
                h("nav", { "aria-label": "\u0420\u0430\u0437\u0434\u0435\u043B\u044B \u043C\u0443\u0437\u044B\u043A\u0438" }, [["offline", "\u041e\u0444\u043b\u0430\u0439\u043d"], ["home", "Главная"], ["mine", "Моя музыка"], ["playlists", "Плейлисты"]].map(([id, name]) => h("button", { class: tab === id ? "music-tab active" : "music-tab", "aria-pressed": tab === id, onClick: () => { if (id === "offline")
                        audio.current?.pause(); setTab(id); setSelected(null); setQuery(""); } }, name))),
                h("div", { class: "music-tools" },
                    h("button", { class: "icon-btn", title: "\u0417\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044C \u0430\u0443\u0434\u0438\u043E\u0437\u0430\u043F\u0438\u0441\u044C", "aria-label": "\u0417\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044C \u0430\u0443\u0434\u0438\u043E\u0437\u0430\u043F\u0438\u0441\u044C", onClick: () => uploadDialog.current?.showModal() },
                        h(Icon, { name: "upload" })),
                    h("button", { class: "icon-btn", title: "\u0421\u043E\u0437\u0434\u0430\u0442\u044C \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442", "aria-label": "\u0421\u043E\u0437\u0434\u0430\u0442\u044C \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442", onClick: () => { setDialogError(null); createDialog.current?.showModal(); } },
                        h(Icon, { name: "plus" })))),
            h("label", { class: "music-search" },
                h(Icon, { name: "search" }),
                h("input", { type: "search", placeholder: "\u041F\u043E\u0438\u0441\u043A \u043C\u0443\u0437\u044B\u043A\u0438", "aria-label": "\u041F\u043E\u0438\u0441\u043A \u043C\u0443\u0437\u044B\u043A\u0438", value: query, onInput: (e) => setQuery(e.currentTarget.value) })),
            tab === "home" && !query && h("div", { class: "music-mix" },
                h("button", { class: "mix-play", "aria-label": "\u0421\u043B\u0443\u0448\u0430\u0442\u044C \u043C\u043E\u0439 \u043C\u0438\u043A\u0441", disabled: busy, onClick: mix }, "\u25B6"),
                h("h2", null, "\u0421\u043B\u0443\u0448\u0430\u0442\u044C \u043C\u043E\u0439 \u043C\u0438\u043A\u0441"),
                h("p", null, "\u0412\u0430\u0448\u0430 \u043C\u0443\u0437\u044B\u043A\u0430 \u0432 \u043B\u044E\u0431\u0438\u043C\u043E\u043C \u043F\u043E\u0440\u044F\u0434\u043A\u0435"),
                h("button", { class: "mix-settings", onClick: () => { setDraftSource(source); setDraftOrder(order); mixDialog.current?.showModal(); } }, "\u2637 \u041D\u0430\u0441\u0442\u0440\u043E\u0438\u0442\u044C"))),
        tab !== "offline" && h(ErrorLine, { error: error ?? results.error ?? playlists.error }),
        tab === "mine" && !query && recent.length > 0 && h("div", { class: "music-panel music-history" },
            h("div", { class: "music-section-head" },
                h("h2", null, "\u041D\u0435\u0434\u0430\u0432\u043D\u043E \u043F\u0440\u043E\u0441\u043B\u0443\u0448\u0430\u043D\u043D\u044B\u0435"),
                h("button", { class: "link", onClick: () => { setRecent([]); try {
                        localStorage.removeItem(`sn-listening-${userId}`);
                    }
                    catch { } } }, "\u041E\u0447\u0438\u0441\u0442\u0438\u0442\u044C \u0438\u0441\u0442\u043E\u0440\u0438\u044E")),
            rows(recent)),
        tab === "offline" ? h(OfflineMusic, { userId: userId }) : search.length >= 2 ? h("div", { class: "music-panel" },
            h("h2", null, "\u0420\u0435\u0437\u0443\u043B\u044C\u0442\u0430\u0442\u044B \u043F\u043E\u0438\u0441\u043A\u0430"),
            !results.data ? h(Loading, null) : visible.length ? rows(visible) : h(Empty, null, "\u041D\u0438\u0447\u0435\u0433\u043E \u043D\u0435 \u043D\u0430\u0439\u0434\u0435\u043D\u043E.")) : h(Fragment, null,
            tab !== "playlists" && h("div", { class: "music-panel" },
                h("div", { class: "music-section-head" },
                    h("h2", null, "\u041C\u043E\u044F \u043C\u0443\u0437\u044B\u043A\u0430"),
                    tab === "home" && h("button", { class: "link", onClick: () => setTab("mine") }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u0432\u0441\u0435")),
                busy && !tracks.length ? h(Loading, null) : tracks.length ? rows(tab === "home" ? tracks.slice(0, 6) : tracks) : h(Empty, null, "\u0414\u043E\u0431\u0430\u0432\u044C\u0442\u0435 \u043F\u0435\u0440\u0432\u044B\u0435 \u0442\u0440\u0435\u043A\u0438 \u0447\u0435\u0440\u0435\u0437 \u043A\u043D\u043E\u043F\u043A\u0443 \u0437\u0430\u0433\u0440\u0443\u0437\u043A\u0438 \u0438\u043B\u0438 \u043F\u043E\u0438\u0441\u043A."),
                tab === "mine" && cursor !== null && h("button", { class: "ghost", disabled: busy, onClick: () => load(cursor) }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u0435\u0449\u0451")),
            (tab === "home" || tab === "playlists") && h("div", { class: "music-panel" },
                h("div", { class: "music-section-head" },
                    h("h2", null, selected && tab === "playlists" ? selected.title : "Мои плейлисты"),
                    selected && h("button", { class: "link", onClick: () => setSelected(null) }, "\u0412\u0441\u0435 \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442\u044B")),
                selected && tab === "playlists" ? (available.length ? rows(available) : h(Empty, null, "\u041F\u043B\u0435\u0439\u043B\u0438\u0441\u0442 \u043F\u0443\u0441\u0442. \u041D\u0430\u0436\u043C\u0438\u0442\u0435 \u00AB+\u00BB \u0440\u044F\u0434\u043E\u043C \u0441 \u0442\u0440\u0435\u043A\u043E\u043C, \u0447\u0442\u043E\u0431\u044B \u0434\u043E\u0431\u0430\u0432\u0438\u0442\u044C \u0435\u0433\u043E.")) : playlists.data?.items.length ? cards : h(Empty, null, "\u0421\u043E\u0437\u0434\u0430\u0439\u0442\u0435 \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442 \u043A\u043D\u043E\u043F\u043A\u043E\u0439 \u00AB+\u00BB \u0432\u0432\u0435\u0440\u0445\u0443 \u0441\u0442\u0440\u0430\u043D\u0438\u0446\u044B."))),
        h("dialog", { ref: uploadDialog, class: "music-dialog music-upload-dialog", "aria-label": "\u0417\u0430\u0433\u0440\u0443\u0437\u043A\u0430 \u043C\u0443\u0437\u044B\u043A\u0438" },
            dialogClose(uploadDialog),
            h(MediaUpload, { kind: "music", onUploaded: () => { void load(); setTab("mine"); setQuery(""); } }),
            h("footer", { class: "upload-dialog-footer" },
                h("button", { class: "primary", onClick: () => uploadDialog.current?.close() }, "\u0417\u0430\u043A\u0440\u044B\u0442\u044C"))),
        h("dialog", { ref: createDialog, class: "music-dialog", "aria-label": "\u0421\u043E\u0437\u0434\u0430\u043D\u0438\u0435 \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442\u0430" },
            dialogClose(createDialog),
            h("form", { onSubmit: create },
                h("h2", null, "\u0421\u043E\u0437\u0434\u0430\u0442\u044C \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442"),
                h("label", null,
                    "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435 \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442\u0430",
                    h("input", { required: true, maxLength: 100, value: title, onInput: (e) => setTitle(e.currentTarget.value) })),
                h(ErrorLine, { error: dialogError }),
                h("button", { class: "primary", disabled: saving || !title.trim() }, "\u0421\u043E\u0437\u0434\u0430\u0442\u044C"))),
        h("dialog", { ref: addDialog, class: "music-dialog", "aria-label": "\u0414\u043E\u0431\u0430\u0432\u043B\u0435\u043D\u0438\u0435 \u0432 \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442" },
            dialogClose(addDialog),
            h("h2", null, "\u0414\u043E\u0431\u0430\u0432\u0438\u0442\u044C \u0432 \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442"),
            h("p", null, addTrack?.title),
            h(ErrorLine, { error: dialogError }),
            playlists.data?.items.length ? playlists.data.items.map((pl) => h("button", { class: "playlist-choice", disabled: saving, onClick: async () => { if (!addTrack || saving)
                    return; setSaving(true); setDialogError(null); try {
                    await post(`/playlists/${pl.id}/tracks`, { track_id: addTrack.id });
                    playlists.reload();
                    if (selected?.id === pl.id)
                        setSelected(await get(`/playlists/${pl.id}`));
                    addDialog.current?.close();
                }
                catch (e) {
                    setDialogError(e);
                }
                finally {
                    setSaving(false);
                } } }, pl.title)) : h(Empty, null, "\u0421\u043D\u0430\u0447\u0430\u043B\u0430 \u0441\u043E\u0437\u0434\u0430\u0439\u0442\u0435 \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442 \u043A\u043D\u043E\u043F\u043A\u043E\u0439 \u00AB+\u00BB \u0432\u0432\u0435\u0440\u0445\u0443 \u0441\u0442\u0440\u0430\u043D\u0438\u0446\u044B.")),
        h("dialog", { ref: mixDialog, class: "music-dialog", "aria-label": "\u041D\u0430\u0441\u0442\u0440\u043E\u0438\u0442\u044C \u043C\u0438\u043A\u0441" },
            dialogClose(mixDialog),
            h("h2", null, "\u041D\u0430\u0441\u0442\u0440\u043E\u0438\u0442\u044C \u043C\u043E\u0439 \u043C\u0438\u043A\u0441"),
            h("fieldset", null,
                h("legend", null, "\u041A\u043E\u043B\u043B\u0435\u043A\u0446\u0438\u044F"),
                h("label", null,
                    h("input", { type: "radio", name: "mix-source", checked: draftSource === "mine", onChange: () => setDraftSource("mine") }),
                    " \u041C\u043E\u044F \u043C\u0443\u0437\u044B\u043A\u0430"),
                playlists.data?.items.map((pl) => h("label", { key: pl.id },
                    h("input", { type: "radio", name: "mix-source", checked: draftSource === String(pl.id), onChange: () => setDraftSource(String(pl.id)) }),
                    " ",
                    pl.title))),
            h("fieldset", null,
                h("legend", null, "\u041F\u043E\u0440\u044F\u0434\u043E\u043A \u0432\u043E\u0441\u043F\u0440\u043E\u0438\u0437\u0432\u0435\u0434\u0435\u043D\u0438\u044F"),
                h("button", { class: draftOrder === "shuffle" ? "chip active" : "chip", "aria-pressed": draftOrder === "shuffle", onClick: () => setDraftOrder("shuffle") }, "\u0412\u043F\u0435\u0440\u0435\u043C\u0435\u0448\u043A\u0443"),
                " ",
                h("button", { class: draftOrder === "list" ? "chip active" : "chip", "aria-pressed": draftOrder === "list", onClick: () => setDraftOrder("list") }, "\u041F\u043E \u0441\u043F\u0438\u0441\u043A\u0443")),
            h("footer", null,
                h("button", { class: "ghost", onClick: () => { setDraftSource("mine"); setDraftOrder("shuffle"); } }, "\u0421\u0431\u0440\u043E\u0441\u0438\u0442\u044C"),
                h("button", { class: "primary", onClick: () => { setSource(draftSource); setOrder(draftOrder); mixDialog.current?.close(); } }, "\u041F\u0440\u0438\u043C\u0435\u043D\u0438\u0442\u044C"))));
}
