import { AISticker } from "./ai.js";
import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, post, upload } from "./api.js";
import { ErrorLine } from "./ui.js";
export function CustomStickers({ select }) {
    const [items, setItems] = useState([]);
    const [title, setTitle] = useState("");
    const [file, setFile] = useState();
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    async function load() { setItems((await get("/stickers")).items); }
    useEffect(() => { void load().catch(setError); }, []);
    async function create() { if (!file || !title.trim() || busy)
        return; setBusy(true); setError(null); try {
        const photo = await upload("/media", file);
        await post("/stickers", { id: photo.id, title: title.trim() });
        setTitle("");
        setFile(undefined);
        await load();
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    return h("section", { class: "custom-stickers" },
        h(AISticker, { created: p => { void post("/stickers", { id: p.id, title: title.trim() || "AI sticker" }).then(load).catch(setError); } }),
        h("h3", null, "\u041C\u043E\u0438 \u0441\u0442\u0438\u043A\u0435\u0440\u044B"),
        h(ErrorLine, { error: error }),
        h("div", { class: "comment-sticker-grid" }, items.map(x => h("div", { key: x.id },
            h("button", { type: "button", disabled: busy, title: `Выбрать: ${x.title}`, onClick: () => select(x) },
                h("img", { src: x.thumb_url, alt: x.title, style: "width:96px;height:96px;object-fit:contain" })),
            h("button", { type: "button", class: "link", disabled: busy, "aria-label": `Удалить стикер ${x.title}`, onClick: async () => { setBusy(true); try {
                    await post("/stickers/remove", { id: x.id });
                    await load();
                }
                catch (e) {
                    setError(e);
                }
                finally {
                    setBusy(false);
                } } }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C")))),
        !items.length && h("p", { class: "muted" }, "\u0417\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u0435 \u043A\u0430\u0440\u0442\u0438\u043D\u043A\u0443 \u0438 \u0434\u0430\u0439\u0442\u0435 \u0435\u0439 \u043D\u0430\u0437\u0432\u0430\u043D\u0438\u0435. \u041E\u043D\u0430 \u0441\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u0441\u044F \u0432 \u0432\u0430\u0448\u0435\u043C \u043D\u0430\u0431\u043E\u0440\u0435."),
        h("label", null,
            "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435 \u0441\u0442\u0438\u043A\u0435\u0440\u0430",
            h("input", { maxLength: 64, value: title, onInput: e => setTitle(e.currentTarget.value) })),
        h("label", null,
            "\u041A\u0430\u0440\u0442\u0438\u043D\u043A\u0430 (PNG, JPEG, GIF, \u0434\u043E 10 \u041C\u0411)",
            h("input", { type: "file", accept: "image/png,image/jpeg,image/gif", disabled: busy, onChange: e => setFile(e.currentTarget.files?.[0]) })),
        h("button", { type: "button", disabled: busy || !file || !title.trim(), onClick: create }, busy ? "Загрузка…" : "Создать стикер"));
}
