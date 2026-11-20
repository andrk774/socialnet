import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { get, patch, post, upload } from "./api.js";
import { Empty, ErrorLine, Loading, useLoad } from "./ui.js";
import { Icon } from "./shellparts.js";
import { useMe } from "./session.js";
import { LibraryViewer } from "./libraryviewer.js";
export function PhotosPage() {
    const me = useMe();
    const settingsKey = `sn-photo-albums-${me?.id}`;
    const [hiddenAlbums, setHiddenAlbums] = useState(() => { try {
        const v = JSON.parse(localStorage.getItem(settingsKey) ?? "[]");
        return Array.isArray(v) ? v.filter(x => Number.isSafeInteger(x)) : [];
    }
    catch {
        return [];
    } });
    const [draftHidden, setDraftHidden] = useState([]);
    const settings = useRef(null);
    const menu = useRef(null);
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
    const photos = useLoad(async () => {
        const all = [];
        let cursor = null;
        do {
            const page = await get(`/photos${cursor ? `?after=${cursor}` : ""}`);
            all.push(...page.items);
            cursor = page.next_cursor;
        } while (cursor);
        return all.sort((a, b) => b.created_at.localeCompare(a.created_at));
    }, []);
    const albums = useLoad(() => get("/photo-albums"), []);
    const [tab, setTab] = useState(new URLSearchParams(location.search).get("tab") === "albums" ? "albums" : "photos");
    const [album, setAlbum] = useState(null);
    const [selecting, setSelecting] = useState(false);
    const [selected, setSelected] = useState([]);
    const [current, setCurrent] = useState(null);
    const [error, setError] = useState(null);
    const [busy, setBusy] = useState(false);
    const [progress, setProgress] = useState("");
    const [title, setTitle] = useState("");
    const input = useRef(null);
    const creator = useRef(null);
    const visible = (photos.data ?? []).filter(p => p.archived === (tab === "archive") && (album === null || p.album_id === album) && (tab !== "photos" || album !== null || !hiddenAlbums.includes(p.album_id ?? 0)));
    const choices = [{ id: 0, title: "Без альбома", count: (photos.data ?? []).filter(p => !p.album_id && !p.archived).length, cover_url: (photos.data ?? []).find(p => !p.album_id && !p.archived)?.thumb_url ?? null }, ...(albums.data?.items ?? [])];
    function saveSettings() { try {
        localStorage.setItem(settingsKey, JSON.stringify(draftHidden));
        setHiddenAlbums(draftHidden);
        setSelected([]);
        settings.current?.close();
    }
    catch {
        setError(new Error("Не удалось сохранить настройки браузера."));
    } }
    const photo = visible.find(p => p.id === current);
    const reload = () => { photos.reload(); albums.reload(); };
    function switchTab(t) { setTab(t); setAlbum(null); setSelected([]); setSelecting(false); }
    function step(n) { const at = visible.findIndex(p => p.id === current); if (visible.length)
        setCurrent(visible[(at + n + visible.length) % visible.length].id); }
    async function organize(action) {
        if (!selected.length || busy)
            return;
        setBusy(true);
        setError(null);
        try {
            await patch("/photos", { ids: selected, ...action });
            setSelected([]);
            setSelecting(false);
            reload();
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    }
    async function files(files) {
        if (busy)
            return;
        setBusy(true);
        setError(null);
        const failures = [];
        for (let i = 0; i < files.length; i++) {
            const file = files[i];
            setProgress(`Загрузка ${i + 1} из ${files.length}`);
            try {
                if (file.size > 10 * 1024 * 1024 || !/\.(jpe?g|png|gif)$/i.test(file.name))
                    throw new Error("JPEG, PNG или GIF, не более 10 МБ");
                const p = await upload("/media", file);
                if (album !== null)
                    await patch("/photos", { ids: [p.id], album_id: album });
            }
            catch (e) {
                failures.push(`${file.name}: ${e instanceof Error ? e.message : String(e)}`);
            }
        }
        setBusy(false);
        setProgress(failures.length ? `Не удалось загрузить: ${failures.length}` : `Загружено фотографий: ${files.length}`);
        if (failures.length)
            setError(new Error(failures.join("; ")));
        if (input.current)
            input.current.value = "";
        reload();
    }
    async function create(e) {
        e.preventDefault();
        if (busy)
            return;
        setBusy(true);
        setError(null);
        try {
            const a = await post("/photo-albums", { title });
            setTitle("");
            creator.current?.close();
            setTab("photos");
            setAlbum(a.id);
            setSelected([]);
            reload();
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    }
    const years = [...new Set(visible.map(p => new Date(p.created_at).getFullYear()))];
    return h("section", { class: "photo-library" },
        h("header", { class: "library-panel" },
            h("h1", null, "\u041C\u043E\u0438 \u0444\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0438"),
            h("div", { class: "library-toolbar" },
                h("nav", { class: "library-tabs", "aria-label": "\u0424\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0438" },
                    h("button", { "aria-pressed": tab === "photos", onClick: () => switchTab("photos") }, "\u0424\u043E\u0442\u043E"),
                    h("button", { "aria-pressed": tab === "albums", onClick: () => switchTab("albums") }, "\u0410\u043B\u044C\u0431\u043E\u043C\u044B"),
                    tab === "archive" && h("button", { "aria-pressed": "true" }, "\u0410\u0440\u0445\u0438\u0432")),
                h("div", { class: "library-actions photo-actions" },
                    h("button", { class: "primary", disabled: busy, onClick: () => input.current?.click() },
                        h(Icon, { name: "photo" }),
                        "\u0417\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044C \u0444\u043E\u0442\u043E"),
                    h("details", { class: "photo-menu", ref: menu },
                        h("summary", { "aria-label": "\u0414\u0435\u0439\u0441\u0442\u0432\u0438\u044F \u0441 \u0444\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u044F\u043C\u0438" }, "\u2022\u2022\u2022"),
                        h("div", { class: "photo-menu-items" },
                            h("button", { onClick: () => { switchTab("archive"); if (menu.current)
                                    menu.current.open = false; } }, "\u0410\u0440\u0445\u0438\u0432"),
                            h("button", { onClick: () => { creator.current?.showModal(); if (menu.current)
                                    menu.current.open = false; } }, "\u0421\u043E\u0437\u0434\u0430\u0442\u044C \u0430\u043B\u044C\u0431\u043E\u043C"),
                            h("button", { disabled: tab === "albums", "aria-pressed": selecting, onClick: () => { setSelecting(!selecting); setSelected([]); if (menu.current)
                                    menu.current.open = false; } }, selecting ? "Отменить выбор" : "Выбрать несколько"))),
                    h("button", { class: "photo-settings-button", "aria-label": "\u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u0432\u043A\u043B\u0430\u0434\u043A\u0438 \u0424\u043E\u0442\u043E", title: "\u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u0432\u043A\u043B\u0430\u0434\u043A\u0438 \u0424\u043E\u0442\u043E", onClick: () => { setDraftHidden([...hiddenAlbums]); setError(null); settings.current?.showModal(); } },
                        h(Icon, { name: "settings" }))),
                h("input", { ref: input, type: "file", hidden: true, multiple: true, accept: "image/jpeg,image/png,image/gif", onChange: e => void files(Array.from(e.target.files ?? [])) })),
            h("p", { class: "muted" }, "JPEG, PNG \u0438\u043B\u0438 GIF \u0434\u043E 10 \u041C\u0411. \u041C\u043E\u0436\u043D\u043E \u0432\u044B\u0431\u0440\u0430\u0442\u044C \u043D\u0435\u0441\u043A\u043E\u043B\u044C\u043A\u043E \u0444\u0430\u0439\u043B\u043E\u0432."),
            h("p", { role: "status" }, progress),
            h(ErrorLine, { error: error || photos.error || albums.error })),
        selecting && h("div", { class: "library-panel library-toolbar" },
            h("strong", null,
                "\u0412\u044B\u0431\u0440\u0430\u043D\u043E: ",
                selected.length,
                " / 200"),
            h("button", { disabled: busy || !selected.length, onClick: () => organize({ archived: tab !== "archive" }) }, tab === "archive" ? "Восстановить" : "В архив"),
            h("label", null,
                "\u0412 \u0430\u043B\u044C\u0431\u043E\u043C ",
                h("select", { disabled: busy || !selected.length, value: "", onChange: e => { const value = e.target.value; if (value !== "")
                        void organize({ album_id: +value }); } },
                    h("option", { value: "" }, "\u0412\u044B\u0431\u0435\u0440\u0438\u0442\u0435 \u0430\u043B\u044C\u0431\u043E\u043C"),
                    h("option", { value: "0" }, "\u0411\u0435\u0437 \u0430\u043B\u044C\u0431\u043E\u043C\u0430"),
                    albums.data?.items.map(a => h("option", { value: a.id }, a.title))))),
        h("div", { class: "library-panel" },
            album !== null && h("div", { class: "library-toolbar" },
                h("button", { onClick: () => switchTab("albums") }, "\u2190 \u0410\u043B\u044C\u0431\u043E\u043C\u044B"),
                h("h2", null, albums.data?.items.find(a => a.id === album)?.title)),
            tab === "archive" && h("p", { class: "muted" }, "\u0410\u0440\u0445\u0438\u0432 \u0441\u043A\u0440\u044B\u0432\u0430\u0435\u0442 \u0444\u043E\u0442\u043E \u0442\u043E\u043B\u044C\u043A\u043E \u0438\u0437 \u044D\u0442\u043E\u0439 \u0433\u0430\u043B\u0435\u0440\u0435\u0438. \u0412 \u043E\u043F\u0443\u0431\u043B\u0438\u043A\u043E\u0432\u0430\u043D\u043D\u044B\u0445 \u0437\u0430\u043F\u0438\u0441\u044F\u0445 \u0438 \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F\u0445 \u043E\u043D\u043E \u043E\u0441\u0442\u0430\u0451\u0442\u0441\u044F."),
            !photos.data && !photos.error ? h(Loading, null) : tab === "albums" ? h("div", { class: "album-grid" },
                albums.data?.items.map(a => h("button", { class: "album-tile", onClick: () => { setAlbum(a.id); setTab("photos"); } },
                    a.cover_url ? h("img", { src: a.cover_url, alt: "", loading: "lazy" }) : h("span", { class: "album-placeholder" },
                        h(Icon, { name: "photo" })),
                    h("strong", null, a.title),
                    h("span", { class: "muted" },
                        "\u0424\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0439: ",
                        a.count))),
                albums.data?.items.length === 0 && h(Empty, null, "\u0421\u043E\u0437\u0434\u0430\u0439\u0442\u0435 \u043F\u0435\u0440\u0432\u044B\u0439 \u0430\u043B\u044C\u0431\u043E\u043C, \u0447\u0442\u043E\u0431\u044B \u0441\u043E\u0431\u0440\u0430\u0442\u044C \u0444\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0438 \u0432\u043C\u0435\u0441\u0442\u0435.")) : visible.length === 0 ? h(Empty, null, tab === "archive" ? "В архиве пока нет фотографий." : hiddenAlbums.length && album === null ? "Нет фото из выбранных альбомов. Измените настройки вкладки." : "Загрузите фотографии — они появятся здесь.") : years.map(year => h("section", { class: "photo-year" },
                h("h2", null, year),
                h("div", { class: "photo-grid" }, visible.filter(p => new Date(p.created_at).getFullYear() === year).map((p, i) => h("button", { key: p.id, class: `photo-tile ${selected.includes(p.id) ? "selected" : ""}`, "aria-label": `${selecting ? "Выбрать" : "Открыть"} фото ${i + 1}, ${new Date(p.created_at).toLocaleDateString()}`, "aria-pressed": selecting ? selected.includes(p.id) : undefined, onClick: () => { if (selecting)
                        setSelected(selected.includes(p.id) ? selected.filter(id => id !== p.id) : selected.length < 200 ? [...selected, p.id] : selected);
                    else
                        setCurrent(p.id); } },
                    h("img", { src: p.thumb_url, alt: `Фотография ${i + 1}`, loading: "lazy" }),
                    selecting && h("span", { class: "photo-check" }, selected.includes(p.id) ? "✓" : "○"))))))),
        h("dialog", { ref: creator, class: "music-dialog", "aria-label": "\u0421\u043E\u0437\u0434\u0430\u0442\u044C \u0430\u043B\u044C\u0431\u043E\u043C" },
            h("form", { class: "stack", onSubmit: create },
                h("h2", null, "\u041D\u043E\u0432\u044B\u0439 \u0430\u043B\u044C\u0431\u043E\u043C"),
                h("label", null,
                    "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435 \u0430\u043B\u044C\u0431\u043E\u043C\u0430",
                    h("input", { required: true, maxLength: 100, value: title, onInput: e => setTitle(e.target.value) })),
                h(ErrorLine, { error: error }),
                h("div", { class: "library-actions" },
                    h("button", { class: "primary", disabled: busy }, "\u0421\u043E\u0437\u0434\u0430\u0442\u044C"),
                    h("button", { type: "button", onClick: () => creator.current?.close() }, "\u041E\u0442\u043C\u0435\u043D\u0430")))),
        h("dialog", { ref: settings, class: "music-dialog photo-settings-dialog", "aria-label": "\u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u0432\u043A\u043B\u0430\u0434\u043A\u0438 \u0424\u043E\u0442\u043E" },
            h("button", { class: "music-dialog-close", "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C \u043D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u0444\u043E\u0442\u043E", onClick: () => settings.current?.close() }, "\u00D7"),
            h("header", null,
                h("h2", null, "\u0412\u043A\u043B\u0430\u0434\u043A\u0430 \u00AB\u0424\u043E\u0442\u043E\u00BB"),
                h("p", { class: "muted" }, "\u0412\u044B\u0431\u0435\u0440\u0438\u0442\u0435 \u0430\u043B\u044C\u0431\u043E\u043C\u044B, \u0441\u043D\u0438\u043C\u043A\u0438 \u0438\u0437 \u043A\u043E\u0442\u043E\u0440\u044B\u0445 \u0445\u043E\u0442\u0438\u0442\u0435 \u043F\u043E\u043A\u0430\u0437\u044B\u0432\u0430\u0442\u044C \u0432\u043E \u0432\u043A\u043B\u0430\u0434\u043A\u0435 \u00AB\u0424\u043E\u0442\u043E\u00BB")),
            h("div", { class: "photo-album-choices" }, choices.map(a => h("label", { key: a.id, class: "photo-album-choice" },
                a.cover_url ? h("img", { src: a.cover_url, alt: "" }) : h("span", { class: "photo-album-empty" },
                    h(Icon, { name: "photo" })),
                h("span", null,
                    h("strong", null, a.title),
                    h("small", null,
                        a.count,
                        " \u0444\u043E\u0442\u043E")),
                h("input", { type: "checkbox", checked: !draftHidden.includes(a.id), onChange: e => setDraftHidden(e.currentTarget.checked ? draftHidden.filter(id => id !== a.id) : [...draftHidden, a.id]) })))),
            h(ErrorLine, { error: error }),
            h("footer", null,
                h("button", { class: "ghost", onClick: () => settings.current?.close() }, "\u041E\u0442\u043C\u0435\u043D\u0430"),
                h("button", { class: "primary", onClick: saveSettings }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C")),
            h("p", { class: "muted photo-settings-note" }, "\u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0430 \u0441\u043E\u0445\u0440\u0430\u043D\u044F\u0435\u0442\u0441\u044F \u0432 \u044D\u0442\u043E\u043C \u0431\u0440\u0430\u0443\u0437\u0435\u0440\u0435. \u0412\u0438\u0434\u0438\u043C\u043E\u0441\u0442\u044C \u043E\u043F\u0443\u0431\u043B\u0438\u043A\u043E\u0432\u0430\u043D\u043D\u044B\u0445 \u0444\u043E\u0442\u043E \u043D\u0435 \u043C\u0435\u043D\u044F\u0435\u0442\u0441\u044F.")),
        photo && h(LibraryViewer, { photo: photo, index: visible.findIndex(p => p.id === current), total: visible.length, album: albums.data?.items.find(a => a.id === photo.album_id)?.title ?? "Мои фотографии", onStep: step, onClose: () => setCurrent(null), onChanged: reload }));
}
