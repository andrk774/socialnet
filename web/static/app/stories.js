import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { fullName, get, post, upload } from "./api.js";
import { Avatar, ErrorLine, useLoad } from "./ui.js";
import { useMe } from "./session.js";
import { StoryViewer } from "./storyviewer.js";
import { Icon } from "./shellparts.js";
function StoryDialog({ children, title, close }) {
    const ref = useRef(null);
    useEffect(() => { ref.current?.showModal(); }, []);
    return h("dialog", { class: "story-dialog", ref: ref, "aria-label": title, onClose: close, onClick: (e) => { if (e.target === ref.current)
            close(); } },
        h("div", { class: "story-dialog-inner" },
            h("button", { class: "story-close", "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C \u0438\u0441\u0442\u043E\u0440\u0438\u044E", onClick: close }, "\u00D7"),
            children));
}
export function Stories() {
    const me = useMe();
    const feed = useLoad(() => get("/stories/feed"), []);
    const [selected, setSelected] = useState(null);
    const [creating, setCreating] = useState(false);
    const [file, setFile] = useState(null);
    const [caption, setCaption] = useState("");
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    const story = selected?.group.stories[selected.index];
    useEffect(() => { if (story)
        post(`/stories/${story.id}/view`).catch(setError); }, [story?.id]);
    const close = () => { setSelected(null); feed.reload(); };
    async function create() {
        if (!file || busy)
            return;
        setBusy(true);
        setError(null);
        try {
            const photo = await upload("/media", file);
            await post("/stories", { photo_id: photo.id, caption });
            setCreating(false);
            setFile(null);
            setCaption("");
            feed.reload();
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    }
    return h("div", { class: "stories-block" },
        h("div", { class: "stories-strip", "aria-label": "\u0418\u0441\u0442\u043E\u0440\u0438\u0438" },
            h("button", { class: "story-card story-create", onClick: () => { setError(null); setCreating(true); } },
                me && h(Avatar, { user: me, size: 48 }),
                h("span", { class: "story-plus" },
                    h(Icon, { name: "plus" })),
                h("strong", null, "\u041C\u043E\u044F \u0438\u0441\u0442\u043E\u0440\u0438\u044F")),
            feed.data?.items.filter((g) => g.stories.length > 0).map((g) => h("button", { key: g.author.id, class: `story-card ${g.all_seen ? "story-seen" : ""}`, "aria-label": `Истории: ${fullName(g.author)}`, onClick: () => { setError(null); setSelected({ group: g, index: 0 }); } },
                (g.stories[0].photo?.url || g.stories[0].video?.poster_url) && h("img", { class: "story-cover", src: g.stories[0].photo?.url ?? g.stories[0].video?.poster_url, alt: "" }),
                h("span", { class: "story-author" },
                    h(Avatar, { user: g.author, size: 36 }),
                    h("strong", null, fullName(g.author))))),
            feed.data && !feed.data.items.length && h("div", { class: "stories-empty" },
                h("strong", null, "\u041F\u043E\u0434\u0435\u043B\u0438\u0442\u0435\u0441\u044C \u043C\u043E\u043C\u0435\u043D\u0442\u043E\u043C"),
                h("span", null,
                    "\u0424\u043E\u0442\u043E \u0438\u0441\u0447\u0435\u0437\u043D\u0435\u0442 \u0447\u0435\u0440\u0435\u0437 24 \u0447\u0430\u0441\u0430.",
                    h("br", null),
                    "\u0417\u0434\u0435\u0441\u044C \u043F\u043E\u044F\u0432\u044F\u0442\u0441\u044F \u0438\u0441\u0442\u043E\u0440\u0438\u0438 \u0432\u0430\u0448\u0438\u0445 \u0434\u0440\u0443\u0437\u0435\u0439."))),
        feed.error && h(ErrorLine, { error: feed.error }),
        creating && h(StoryDialog, { title: "\u041D\u043E\u0432\u0430\u044F \u0438\u0441\u0442\u043E\u0440\u0438\u044F", close: () => { if (!busy)
                setCreating(false); } },
            h("h2", null, "\u041D\u043E\u0432\u0430\u044F \u0438\u0441\u0442\u043E\u0440\u0438\u044F"),
            h("p", { class: "muted" }, "\u0424\u043E\u0442\u043E \u0431\u0443\u0434\u0435\u0442 \u0434\u043E\u0441\u0442\u0443\u043F\u043D\u043E 24 \u0447\u0430\u0441\u0430."),
            h("form", { onSubmit: (e) => { e.preventDefault(); void create(); } },
                h("label", null,
                    "\u0424\u043E\u0442\u043E \u0434\u043B\u044F \u0438\u0441\u0442\u043E\u0440\u0438\u0438",
                    h("input", { type: "file", accept: "image/jpeg,image/png,image/gif", required: true, disabled: busy, onChange: (e) => setFile(e.target.files?.[0] ?? null) })),
                h("label", null,
                    "\u041F\u043E\u0434\u043F\u0438\u0441\u044C",
                    h("input", { value: caption, maxLength: 200, disabled: busy, onInput: (e) => setCaption(e.target.value) })),
                h(ErrorLine, { error: error }),
                h("button", { class: "primary", disabled: !file || busy }, busy ? "Публикуем…" : "Опубликовать историю"))),
        selected && story && h(StoryViewer, { groups: feed.data?.items.filter(g => g.stories.length > 0) ?? [], group: selected.group, index: selected.index, choose: (group, index) => setSelected({ group, index }), close: close }));
}
