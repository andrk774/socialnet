import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { get, put, post, upload } from "./api.js";
import { setMe } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, Time, duration, useLoad } from "./ui.js";
import { Video } from "./posts.js";
export function AvatarUpload({ user, onSaved }) {
    const [file, setFile] = useState(null);
    const [preview, setPreview] = useState("");
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    const input = useRef(null);
    const menu = useRef(null);
    const viewer = useRef(null);
    useEffect(() => {
        const outside = (e) => { if (menu.current && !menu.current.contains(e.target))
            menu.current.open = false; };
        const escape = (e) => { if (e.key === "Escape" && menu.current?.open) {
            menu.current.open = false;
            menu.current.querySelector("summary")?.focus();
        } };
        document.addEventListener("pointerdown", outside);
        document.addEventListener("keydown", escape);
        return () => { document.removeEventListener("pointerdown", outside); document.removeEventListener("keydown", escape); };
    }, []);
    async function remove() {
        if (busy || !confirm("Удалить фото профиля?"))
            return;
        setBusy(true);
        setError(null);
        try {
            setMe(await put("/users/me/avatar", { media_id: null }));
            setFile(null);
            if (input.current)
                input.current.value = "";
            onSaved();
            if (menu.current)
                menu.current.open = false;
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    }
    useEffect(() => {
        if (!file) {
            setPreview("");
            return;
        }
        const url = URL.createObjectURL(file);
        setPreview(url);
        return () => URL.revokeObjectURL(url);
    }, [file]);
    async function save() {
        if (!file || busy)
            return;
        setBusy(true);
        setError(null);
        try {
            const photo = await upload("/media", file);
            setMe(await put("/users/me/avatar", { media_id: photo.id }));
            setFile(null);
            if (input.current)
                input.current.value = "";
            onSaved();
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    }
    return h("div", { class: "avatar-upload" },
        h("details", { ref: menu, class: "profile-avatar-menu" },
            h("summary", { "aria-label": "\u041C\u0435\u043D\u044E \u0444\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0438 \u043F\u0440\u043E\u0444\u0438\u043B\u044F" },
                h(Avatar, { user: user, size: 144 }),
                h("span", { class: "avatar-plus", "aria-hidden": "true" }, "+")),
            h("div", { class: "profile-avatar-options" },
                user.avatar_url && h("button", { type: "button", onClick: () => { if (menu.current)
                        menu.current.open = false; viewer.current?.showModal(); } }, "\u041E\u0442\u043A\u0440\u044B\u0442\u044C \u0444\u043E\u0442\u043E"),
                h("button", { type: "button", disabled: busy, onClick: () => { if (menu.current)
                        menu.current.open = false; input.current?.click(); } }, "\u0418\u0437\u043C\u0435\u043D\u0438\u0442\u044C \u0444\u043E\u0442\u043E"),
                user.avatar_url && h("button", { type: "button", class: "avatar-delete", disabled: busy, onClick: remove }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C \u0444\u043E\u0442\u043E"))),
        h("input", { ref: input, type: "file", class: "sr-only", "aria-label": "\u041D\u043E\u0432\u043E\u0435 \u0444\u043E\u0442\u043E \u043F\u0440\u043E\u0444\u0438\u043B\u044F", accept: "image/jpeg,image/png,image/gif", disabled: busy, onChange: (e) => {
                const el = e.currentTarget;
                const selected = el.files?.[0];
                setError(null);
                if (selected && selected.size > 10 * 1024 * 1024) {
                    setError(new Error("Фото должно быть не больше 10 МБ"));
                    el.value = "";
                    setFile(null);
                    return;
                }
                setFile(selected ?? null);
            } }),
        h("dialog", { ref: viewer, class: "avatar-photo-dialog", "aria-label": "\u0424\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u044F \u043F\u0440\u043E\u0444\u0438\u043B\u044F", onClick: (e) => { if (e.target === e.currentTarget)
                viewer.current?.close(); } },
            h("button", { class: "photo-close", "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C \u0444\u043E\u0442\u043E", onClick: () => viewer.current?.close() }, "\u00D7"),
            user.avatar_url && h("img", { src: user.avatar_url.replace(/\/thumb$/, "/full"), alt: "\u0424\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u044F \u043F\u0440\u043E\u0444\u0438\u043B\u044F" })),
        file && h("small", { class: "muted" }, "JPG, PNG, GIF \u00B7 \u0434\u043E 10 \u041C\u0411"),
        file && h("div", { class: "avatar-upload-preview" },
            preview && h("img", { src: preview, alt: "\u041D\u043E\u0432\u044B\u0439 \u0430\u0432\u0430\u0442\u0430\u0440" }),
            h("button", { class: "primary", disabled: busy, onClick: save }, busy ? "Сохраняем…" : "Сохранить аватар"),
            h("button", { class: "ghost", disabled: busy, onClick: () => { setFile(null); if (input.current)
                    input.current.value = ""; } }, "\u041E\u0442\u043C\u0435\u043D\u0430")),
        h(ErrorLine, { error: error }));
}
export function MediaUpload({ kind, onUploaded }) {
    const [file, setFile] = useState(null);
    const [title, setTitle] = useState("");
    const [artist, setArtist] = useState("");
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    const [done, setDone] = useState(false);
    const input = useRef(null);
    const music = kind === "music";
    async function send(e) {
        e.preventDefault();
        if (!file || busy)
            return;
        setBusy(true);
        setError(null);
        setDone(false);
        try {
            await upload(music ? "/tracks" : "/videos", file, music ? { title, artist } : { title });
            setFile(null);
            setTitle("");
            setArtist("");
            if (input.current)
                input.current.value = "";
            setDone(true);
            onUploaded();
        }
        catch (err) {
            setError(err);
        }
        finally {
            setBusy(false);
        }
    }
    return h("form", { class: "media-upload", onSubmit: send, "aria-label": music ? "Загрузка музыки" : "Загрузка видео" },
        h("h2", null, music ? "Выберите аудиозапись на вашем компьютере" : "Загрузить видео"),
        music ? h("div", { class: "upload-restrictions" },
            h("p", { class: "muted" }, "\u041E\u0433\u0440\u0430\u043D\u0438\u0447\u0435\u043D\u0438\u044F"),
            h("ul", null,
                h("li", null, "\u0410\u0443\u0434\u0438\u043E\u0444\u0430\u0439\u043B \u2014 \u0434\u043E 50 \u041C\u0411, \u0434\u043B\u0438\u0442\u0435\u043B\u044C\u043D\u043E\u0441\u0442\u044C\u044E \u043E\u0442 1 \u0441\u0435\u043A\u0443\u043D\u0434\u044B \u0434\u043E 20 \u043C\u0438\u043D\u0443\u0442. \u0424\u043E\u0440\u043C\u0430\u0442\u044B: MP3, OGG, FLAC, WAV, M4A."),
                h("li", null, "\u0417\u0430\u0433\u0440\u0443\u0436\u0430\u0439\u0442\u0435 \u043C\u0443\u0437\u044B\u043A\u0443, \u043A\u043E\u0442\u043E\u0440\u0443\u044E \u0432\u044B \u0432\u043F\u0440\u0430\u0432\u0435 \u0440\u0430\u0437\u043C\u0435\u0449\u0430\u0442\u044C."))) : h("p", { class: "muted" }, "MP4, MOV, MKV, WEBM \u00B7 \u0434\u043E 1 \u0413\u0411 \u0438 30 \u043C\u0438\u043D\u0443\u0442"),
        h("label", { class: music ? "sr-only" : "" },
            "\u0424\u0430\u0439\u043B",
            h("input", { ref: input, type: "file", required: true, disabled: busy, accept: music ? ".mp3,.ogg,.flac,.wav,.m4a" : ".mp4,.mov,.mkv,.webm", onChange: (e) => {
                    const el = e.currentTarget;
                    const selected = el.files?.[0];
                    setError(null);
                    setDone(false);
                    if (selected && selected.size > (music ? 50 * 1024 * 1024 : 1024 * 1024 * 1024)) {
                        setError(new Error(music ? "Файл больше 50 МБ" : "Файл больше 1 ГБ"));
                        el.value = "";
                        setFile(null);
                        return;
                    }
                    setFile(selected ?? null);
                    setTitle(selected?.name.replace(/\.[^.]+$/, "") ?? "");
                } })),
        music && h("div", { class: "upload-file-choice" },
            h("button", { type: "button", class: "primary", disabled: busy, onClick: () => input.current?.click() }, file ? "Выбрать другой файл" : "Выбрать файл"),
            file && h("p", { class: "muted" }, file.name)),
        file && h(Fragment, null,
            h("label", null,
                "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435",
                h("input", { value: title, maxLength: 200, disabled: busy, onInput: (e) => setTitle(e.currentTarget.value) })),
            music && h("label", null,
                "\u0418\u0441\u043F\u043E\u043B\u043D\u0438\u0442\u0435\u043B\u044C",
                h("input", { value: artist, maxLength: 200, disabled: busy, onInput: (e) => setArtist(e.currentTarget.value) })),
            h("button", { class: "primary", disabled: busy }, busy ? "Загрузка и обработка…" : "Загрузить")),
        busy && h("p", { role: "status" }, "\u041D\u0435 \u0437\u0430\u043A\u0440\u044B\u0432\u0430\u0439\u0442\u0435 \u0441\u0442\u0440\u0430\u043D\u0438\u0446\u0443 \u0434\u043E \u043E\u043A\u043E\u043D\u0447\u0430\u043D\u0438\u044F \u0437\u0430\u0433\u0440\u0443\u0437\u043A\u0438."),
        done && h("p", { role: "status" }, music ? "Трек добавлен в вашу музыку." : "Видео загружено. После обработки здесь появится плеер."),
        h(ErrorLine, { error: error }));
}
export function VideosPage({ userId }) {
    const list = useLoad(() => get(`/users/${userId}/videos?limit=50`), [userId]);
    const [query, setQuery] = useState("");
    const [filter, setFilter] = useState("all");
    const [selected, setSelected] = useState(null);
    const [viewError, setViewError] = useState(null);
    const viewer = useRef(null);
    const uploader = useRef(null);
    useEffect(() => { if (selected)
        viewer.current?.showModal(); }, [selected]);
    useEffect(() => {
        if (!list.data?.items.some((v) => v.status === "processing"))
            return;
        const timer = window.setTimeout(list.reload, 4000);
        return () => clearTimeout(timer);
    }, [list.data]);
    const videos = list.data?.items.filter((v) => (filter === "all" || v.status === filter) && v.title.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())) ?? [];
    return h("section", { class: "video-library" },
        h("header", { class: "video-library-header" },
            h("div", null,
                h("h1", null, "\u0412\u0438\u0434\u0435\u043E"),
                h("p", { class: "muted" }, "\u0412\u0430\u0448\u0430 \u0432\u0438\u0434\u0435\u043E\u0442\u0435\u043A\u0430")),
            h("button", { class: "primary", onClick: () => uploader.current?.showModal() }, "\u0417\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044C \u0432\u0438\u0434\u0435\u043E")),
        h("label", { class: "video-search" },
            h("span", { class: "sr-only" }, "\u041F\u043E\u0438\u0441\u043A \u0432\u0438\u0434\u0435\u043E"),
            h("input", { type: "search", placeholder: "\u041F\u043E\u0438\u0441\u043A \u043F\u043E \u043D\u0430\u0437\u0432\u0430\u043D\u0438\u044E", value: query, onInput: (e) => setQuery(e.currentTarget.value) })),
        h("nav", { class: "video-filters", "aria-label": "\u0424\u0438\u043B\u044C\u0442\u0440\u044B \u0432\u0438\u0434\u0435\u043E" }, [["all", "Все"], ["ready", "Готовые"], ["processing", "Обрабатываются"], ["failed", "С ошибкой"]].map(([id, label]) => h("button", { class: filter === id ? "chip active" : "chip", "aria-pressed": filter === id, onClick: () => setFilter(id) }, label))),
        h(ErrorLine, { error: list.error }),
        !list.data ? h(Loading, null) : !videos.length ? h(Empty, null, list.data.items.length ? "Нет видео по выбранным условиям." : "Загрузите своё первое видео.") : h("div", { class: "video-grid" }, videos.map((v) => h("article", { class: "video-card", key: v.id },
            h("button", { class: "video-thumbnail", disabled: v.status !== "ready", "aria-label": `Смотреть: ${v.title}`, onClick: () => { setViewError(null); setSelected(v); } },
                v.poster_url ? h("img", { src: v.poster_url, alt: "", loading: "lazy" }) : h("span", { class: "video-placeholder", "aria-hidden": "true" }, "\u25B7"),
                v.status === "ready" && h(Fragment, null,
                    h("span", { class: "video-play", "aria-hidden": "true" }, "\u25B6"),
                    v.duration_sec !== undefined && h("span", { class: "video-duration" }, duration(v.duration_sec))),
                v.status !== "ready" && h("span", { class: "video-state" }, v.status === "failed" ? "Ошибка обработки" : "Обрабатывается…")),
            h("h2", null, v.title),
            h("p", { class: "muted" },
                v.views ?? 0,
                " \u043F\u0440\u043E\u0441\u043C\u043E\u0442\u0440\u043E\u0432"),
            h("p", { class: "muted" },
                h(Time, { iso: v.created_at })),
            v.status === "failed" && h("p", { class: "error" }, "\u041F\u043E\u043F\u0440\u043E\u0431\u0443\u0439\u0442\u0435 \u0437\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044C \u0434\u0440\u0443\u0433\u043E\u0439 \u0444\u0430\u0439\u043B.")))),
        h("dialog", { ref: uploader, class: "music-dialog", "aria-label": "\u0417\u0430\u0433\u0440\u0443\u0437\u043A\u0430 \u0432\u0438\u0434\u0435\u043E" },
            h("button", { class: "music-dialog-close", "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C \u0437\u0430\u0433\u0440\u0443\u0437\u043A\u0443 \u0432\u0438\u0434\u0435\u043E", onClick: () => uploader.current?.close() }, "\u00D7"),
            h(MediaUpload, { kind: "video", onUploaded: () => { list.reload(); setFilter("all"); setQuery(""); } })),
        h("dialog", { ref: viewer, class: "video-viewer", "aria-label": "\u041F\u0440\u043E\u0441\u043C\u043E\u0442\u0440 \u0432\u0438\u0434\u0435\u043E", onClose: () => setSelected(null), onClick: (e) => { if (e.target === e.currentTarget)
                viewer.current?.close(); } },
            h("button", { class: "music-dialog-close", "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C \u0432\u0438\u0434\u0435\u043E", onClick: () => viewer.current?.close() }, "\u00D7"),
            selected && h(Fragment, null,
                h(Video, { onPlay: () => { void post(`/videos/${selected.id}/view`).then(x => { setSelected(old => old?.id === selected.id ? { ...old, views: x.views } : old); list.reload(); }).catch(setViewError); }, key: selected.id, src: selected.hls_url ?? "", mp4: selected.mp4_url, poster: selected.poster_url ?? "", title: selected.title }),
                h(ErrorLine, { error: viewError }),
                h("p", { class: "muted" },
                    selected.views ?? 0,
                    " \u043F\u0440\u043E\u0441\u043C\u043E\u0442\u0440\u043E\u0432"))));
}
