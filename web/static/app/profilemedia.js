import { h, Fragment } from '../vendor/preact/index.js';
import { useState, useRef } from '../vendor/preact/hooks.js';
import { get, post } from './api.js';
import { ErrorLine, Loading, useLoad, duration } from './ui.js';
import { Icon } from './shellparts.js';
import { Video } from './posts.js';
import { LibraryViewer } from './libraryviewer.js';
const tabs = [['music', 'music', 'Музыка'], ['photos', 'photo', 'Фото'], ['videos', 'video', 'Видео'], ['albums', 'photo', 'Альбомы'], ['clips', 'video', 'Клипы'], ['articles', 'file', 'Статьи']];
function ProfileTrack({ item }) {
    const audio = useRef(null);
    const [playing, setPlaying] = useState(false), [elapsed, setElapsed] = useState(0), [error, setError] = useState(null);
    const [length, setLength] = useState(item.duration_sec ?? 0);
    async function toggle() { const el = audio.current; if (!el)
        return; if (!el.paused) {
        el.pause();
        return;
    } document.querySelectorAll('audio').forEach(other => { if (other !== el)
        other.pause(); }); try {
        await el.play();
        setError(null);
    }
    catch (e) {
        setError(e);
    } }
    return h("div", { class: "profile-track" },
        h("button", { class: "profile-track-play", "aria-label": `${playing ? 'Пауза' : 'Слушать'}: ${item.title}`, onClick: toggle },
            h("svg", { width: "20", height: "20", viewBox: "0 0 24 24", fill: "currentColor", "aria-hidden": "true" },
                h("path", { d: playing ? 'M6 4h4v16H6zM14 4h4v16h-4z' : 'M7 4v16l14-8z' }))),
        h("div", { class: "profile-track-info" },
            h("strong", null, item.title),
            h("span", { class: "muted" }, item.artist || 'Неизвестный исполнитель')),
        h("span", { class: "profile-track-duration" }, duration(length)),
        h("audio", { ref: audio, preload: "metadata", src: item.url, onPlay: () => setPlaying(true), onPause: () => setPlaying(false), onEnded: () => setPlaying(false), onTimeUpdate: e => setElapsed(e.currentTarget.currentTime), onLoadedMetadata: e => { if (Number.isFinite(e.currentTarget.duration))
                setLength(e.currentTarget.duration); }, onError: () => setError(new Error('Не удалось загрузить аудио')) }),
        (playing || elapsed > 0) && h("div", { class: "profile-track-progress" },
            h("span", null, duration(elapsed)),
            h("input", { "aria-label": "\u041F\u043E\u0437\u0438\u0446\u0438\u044F \u0442\u0440\u0435\u043A\u0430", type: "range", min: "0", max: length || 1, step: "1", value: elapsed, onInput: e => { if (audio.current)
                    audio.current.currentTime = Number(e.currentTarget.value); } })),
        h(ErrorLine, { error: error }));
}
function ProfileVideo({ item }) {
    const dialog = useRef(null);
    const [opened, setOpened] = useState(false);
    const [error, setError] = useState(null);
    return h(Fragment, null,
        h("button", { class: "profile-video-card", onClick: () => { setOpened(true); dialog.current?.showModal(); } },
            h("span", { class: "profile-video-thumb" },
                item.poster_url ? h("img", { loading: "lazy", src: item.poster_url, alt: "" }) : h(Icon, { name: "video" }),
                h("span", { class: "profile-video-play", "aria-hidden": "true" }, "\u25B6"),
                h("span", { class: "profile-video-duration" }, duration(item.duration_sec ?? 0))),
            h("strong", null, item.title || 'Видео')),
        h("dialog", { ref: dialog, class: "profile-video-dialog", onClose: () => setOpened(false) },
            h("header", null,
                h("strong", null, item.title || 'Видео'),
                h("button", { class: "ghost", "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C \u0432\u0438\u0434\u0435\u043E", onClick: () => dialog.current?.close() }, "\u00D7")),
            opened && h(Video, { src: item.hls_url ?? '', mp4: item.mp4_url, poster: item.poster_url ?? '', title: item.title, onPlay: () => { void post(`/videos/${item.id}/view`).catch(setError); } }),
            h(ErrorLine, { error: error })));
}
export function ProfileMedia({ id }) {
    const [tab, setTab] = useState('music');
    return h("section", { class: "profile-media" },
        h("div", { class: "profile-media-tabs", role: "tablist", "aria-label": "\u041C\u0430\u0442\u0435\u0440\u0438\u0430\u043B\u044B \u043F\u0440\u043E\u0444\u0438\u043B\u044F" }, tabs.map(([key, icon, label]) => h("button", { id: `profile-tab-${key}`, role: "tab", "aria-selected": tab === key, "aria-controls": "profile-media-content", class: tab === key ? 'active' : '', onClick: () => setTab(key) },
            h(Icon, { name: icon }),
            " ",
            label))),
        h("div", { id: "profile-media-content", role: "tabpanel", "aria-labelledby": `profile-tab-${tab}` },
            h(MediaContent, { key: `${id}-${tab}`, id: id, tab: tab })));
}
function MediaContent({ id, tab }) {
    const [current, setCurrent] = useState(null);
    const path = tab === 'music' ? `/users/${id}/music?limit=6` : tab === 'photos' ? '/photos?limit=6' : tab === 'albums' ? '/photo-albums' : tab === 'articles' ? '/articles/mine' : `/users/${id}/videos`;
    const load = useLoad(() => get(path), [path]);
    if (load.error)
        return h(ErrorLine, { error: load.error });
    if (!load.data)
        return h(Loading, null);
    const items = load.data.items.filter(x => tab === 'clips' ? x.status === 'ready' && !!x.width && !!x.height && x.height > x.width && (x.duration_sec ?? Infinity) <= 60 : tab === 'videos' ? x.status === 'ready' : true).slice(0, 6);
    const link = tab === 'music' ? '/music' : tab === 'photos' ? '/photos' : tab === 'albums' ? '/photos?tab=albums' : tab === 'articles' ? '/articles/new' : '/videos';
    return h("div", { class: "profile-media-body" },
        tab === 'clips' && h("p", { class: "muted" }, "\u0412\u0435\u0440\u0442\u0438\u043A\u0430\u043B\u044C\u043D\u044B\u0435 \u0432\u0438\u0434\u0435\u043E \u0434\u043B\u0438\u0442\u0435\u043B\u044C\u043D\u043E\u0441\u0442\u044C\u044E \u0434\u043E \u043C\u0438\u043D\u0443\u0442\u044B"),
        !items.length ? h("div", { class: "profile-media-empty" },
            h("p", { class: "muted" }, tab === 'articles' ? 'Вы ещё не добавили статью' : 'Здесь пока нет материалов'),
            h("a", { class: "button ghost", href: link }, tab === 'articles' ? 'Создать статью' : tab === 'albums' ? 'Создать альбом' : tab === 'music' ? 'Добавить музыку' : tab === 'photos' ? 'Загрузить фото' : 'Загрузить видео')) : h(Fragment, null,
            h("div", { class: `profile-media-items ${tab}` }, items.map(x => h("article", { key: x.id }, tab === 'music' ? h(ProfileTrack, { item: x }) : tab === 'photos' ? h("button", { class: "profile-photo-card", onClick: () => setCurrent(String(x.id)) },
                h("img", { src: x.thumb_url || x.url, alt: "\u0424\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u044F" })) : tab === 'albums' ? h("a", { href: "/photos?tab=albums" },
                x.cover_url && h("img", { src: x.cover_url, alt: "" }),
                h("strong", null, x.title),
                h("p", { class: "muted" },
                    x.count ?? 0,
                    " \u0444\u043E\u0442\u043E")) : tab === 'articles' ? h("a", { href: `/articles/${x.id}` },
                h("strong", null, x.title)) : h(ProfileVideo, { item: x })))),
            h("a", { class: "profile-media-all", href: tab === 'articles' ? '/articles' : link }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u0432\u0441\u0451")),
        tab === 'photos' && current && items.some(x => String(x.id) === current) && h(LibraryViewer, { photo: items.find(x => String(x.id) === current), index: items.findIndex(x => String(x.id) === current), total: items.length, album: "\u041C\u043E\u0438 \u0444\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0438", onStep: n => setCurrent(String(items[(items.findIndex(x => String(x.id) === current) + n + items.length) % items.length].id)), onClose: () => setCurrent(null), onChanged: load.reload }));
}
