import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { post, upload } from "./api.js";
import { ErrorLine } from "./ui.js";
export function MusicImport({ done }) {
    const [entries, setEntries] = useState([]), [files, setFiles] = useState([]), [busy, setBusy] = useState(false), [error, setError] = useState(null), [results, setResults] = useState([]);
    async function read(file) { if (!file)
        return; try {
        if (file.size > 2e6)
            throw new Error("Список больше 2 МБ.");
        const raw = JSON.parse(await file.text());
        if (!Array.isArray(raw) || raw.length > 1000)
            throw new Error("Ожидается массив до 1000 треков.");
        const parsed = raw.map((x) => { if (typeof x.file !== "string" || typeof x.title !== "string" || typeof x.artist !== "string" || !Array.isArray(x.playlists) || x.playlists.some((p) => typeof p !== "string" || p.length > 100) || !x.title.trim() || x.title.length > 200 || x.artist.length > 200 || x.playlists.length > 20)
            throw new Error("Неверный формат трека."); return x; });
        setEntries(parsed);
        setError(null);
        setResults([]);
    }
    catch (e) {
        setError(e);
    } }
    async function run() {
        if (busy)
            return;
        setBusy(true);
        setError(null);
        const lines = [];
        const playlists = new Map();
        try {
            for (const entry of entries) {
                const matches = files.filter(f => f.name === entry.file);
                if (matches.length !== 1) {
                    lines.push(`${entry.artist} — ${entry.title}: файл отсутствует или имя неоднозначно`);
                    setResults([...lines]);
                    continue;
                }
                try {
                    const track = await upload("/tracks", matches[0], { title: entry.title, artist: entry.artist });
                    for (const name of new Set(entry.playlists)) {
                        let id = playlists.get(name);
                        if (!id) {
                            const pl = await post("/playlists", { title: name });
                            id = pl.id;
                            playlists.set(name, id);
                        }
                        await post(`/playlists/${id}/tracks`, { track_id: track.id });
                    }
                    lines.push(`${entry.artist} — ${entry.title}: импортирован`);
                }
                catch (e) {
                    lines.push(`${entry.title}: ${e instanceof Error ? e.message : "ошибка импорта"}`);
                }
                setResults([...lines]);
            }
            done();
        }
        finally {
            setBusy(false);
        }
    }
    return h("details", { class: "music-panel" },
        h("summary", null, "\u0418\u043C\u043F\u043E\u0440\u0442 \u043C\u0443\u0437\u044B\u043A\u0438 \u0438 \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442\u043E\u0432 \u0438\u0437 \u0444\u0430\u0439\u043B\u043E\u0432"),
        h("p", null, "\u0412\u044B\u0431\u0435\u0440\u0438\u0442\u0435 \u0441\u0432\u043E\u0438 \u0430\u0443\u0434\u0438\u043E\u0444\u0430\u0439\u043B\u044B \u0438 JSON-\u0441\u043F\u0438\u0441\u043E\u043A \u0441 \u043D\u0430\u0437\u0432\u0430\u043D\u0438\u044F\u043C\u0438 \u0438 \u043F\u043B\u0435\u0439\u043B\u0438\u0441\u0442\u0430\u043C\u0438. \u041F\u0440\u044F\u043C\u043E\u0435 \u0441\u043A\u0430\u0447\u0438\u0432\u0430\u043D\u0438\u0435 \u0438\u0437 \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430 VK \u0437\u0434\u0435\u0441\u044C \u043D\u0435 \u043F\u043E\u0434\u043A\u043B\u044E\u0447\u0435\u043D\u043E. \u0424\u0430\u0439\u043B\u044B \u043D\u0435 \u0437\u0430\u0433\u0440\u0443\u0436\u0430\u044E\u0442\u0441\u044F \u0434\u043E \u043D\u0430\u0436\u0430\u0442\u0438\u044F \u00AB\u0418\u043C\u043F\u043E\u0440\u0442\u0438\u0440\u043E\u0432\u0430\u0442\u044C\u00BB."),
        h("pre", null, '[{"file":"song.mp3","title":"Название","artist":"Исполнитель","playlists":["Любимое"]}]'),
        h("label", null,
            "\u0421\u043F\u0438\u0441\u043E\u043A JSON",
            h("input", { disabled: busy, type: "file", accept: ".json,application/json", onChange: e => void read(e.currentTarget.files?.[0]) })),
        h("label", null,
            "\u0410\u0443\u0434\u0438\u043E\u0444\u0430\u0439\u043B\u044B",
            h("input", { disabled: busy, type: "file", multiple: true, accept: ".mp3,.ogg,.flac,.wav,.m4a", onChange: e => setFiles(Array.from(e.currentTarget.files || [])) })),
        h("p", null,
            "\u0412 \u0441\u043F\u0438\u0441\u043A\u0435: ",
            entries.length,
            ". \u0412\u044B\u0431\u0440\u0430\u043D\u043E \u0444\u0430\u0439\u043B\u043E\u0432: ",
            files.length,
            "."),
        h("p", null, "\u041F\u043B\u0435\u0439\u043B\u0438\u0441\u0442\u044B \u0441\u043E\u0437\u0434\u0430\u044E\u0442\u0441\u044F \u0437\u0430\u043D\u043E\u0432\u043E. \u041F\u043E\u0432\u0442\u043E\u0440\u043D\u044B\u0439 \u0438\u043C\u043F\u043E\u0440\u0442 \u043C\u043E\u0436\u0435\u0442 \u0441\u043E\u0437\u0434\u0430\u0442\u044C \u0434\u0443\u0431\u043B\u0438\u043A\u0430\u0442\u044B. \u0422\u0440\u0435\u043A\u0438 \u0431\u0435\u0437 \u0444\u0430\u0439\u043B\u0430 \u043E\u0441\u0442\u0430\u044E\u0442\u0441\u044F \u0432 \u0438\u0441\u0445\u043E\u0434\u043D\u043E\u043C JSON; \u043E\u043D\u0438 \u043D\u0435 \u043F\u043E\u044F\u0432\u044F\u0442\u0441\u044F \u043A\u0430\u043A \u0434\u043E\u0441\u0442\u0443\u043F\u043D\u044B\u0435 \u0434\u043B\u044F \u043F\u0440\u043E\u0441\u043B\u0443\u0448\u0438\u0432\u0430\u043D\u0438\u044F."),
        h("button", { disabled: busy || !entries.length || !files.length, onClick: run }, busy ? "Импорт…" : "Импортировать"),
        h(ErrorLine, { error: error }),
        h("ul", null, results.map((line, i) => h("li", { key: i }, line))));
}
