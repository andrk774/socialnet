import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { del, fullName, patch, put } from "./api.js";
import { Avatar, ErrorLine, Time } from "./ui.js";
import { setMe, useMe } from "./session.js";
import { PhotoComments } from "./photocomments.js";
export function LibraryViewer({ photo, index, total, album, onStep, onClose, onChanged }) {
    const me = useMe();
    const dialog = useRef(null);
    const menu = useRef(null);
    const [editing, setEditing] = useState(false);
    const [text, setText] = useState(photo.description ?? "");
    const [error, setError] = useState(null);
    const [busy, setBusy] = useState(false);
    const [notice, setNotice] = useState("");
    useEffect(() => { dialog.current?.showModal(); const overflow = document.body.style.overflow; document.body.style.overflow = "hidden"; return () => { document.body.style.overflow = overflow; }; }, []);
    useEffect(() => { setEditing(false); setText(photo.description ?? ""); setError(null); setNotice(""); if (menu.current)
        menu.current.open = false; }, [photo.id, photo.description]);
    useEffect(() => { const outside = (e) => { if (menu.current && !menu.current.contains(e.target))
        menu.current.open = false; }; document.addEventListener("pointerdown", outside); return () => document.removeEventListener("pointerdown", outside); }, []);
    async function act(action) { if (busy)
        return; setBusy(true); setError(null); try {
        await action();
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    return h("dialog", { ref: dialog, class: "library-viewer detailed-viewer", "aria-label": "\u041F\u0440\u043E\u0441\u043C\u043E\u0442\u0440 \u0444\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0438", onClose: onClose, onCancel: e => { if (busy)
            e.preventDefault(); }, onClick: e => { if (e.target === e.currentTarget && !busy)
            dialog.current?.close(); }, onKeyDown: e => { if (e.target.closest("input,textarea,select") || busy)
            return; if (e.key === "ArrowRight") {
            e.preventDefault();
            onStep(1);
        } if (e.key === "ArrowLeft") {
            e.preventDefault();
            onStep(-1);
        } } },
        h("button", { class: "photo-view-close", disabled: busy, "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C \u0444\u043E\u0442\u043E", onClick: () => dialog.current?.close() }, "\u00D7"),
        h("div", { class: "photo-view-media" },
            h("div", { class: "photo-view-image" },
                h("img", { src: photo.url, alt: photo.description || "Фотография в полном размере" }),
                h("button", { class: "photo-view-prev", disabled: busy || total < 2, "aria-label": "\u041F\u0440\u0435\u0434\u044B\u0434\u0443\u0449\u0435\u0435 \u0444\u043E\u0442\u043E", onClick: () => onStep(-1) }, "\u2039"),
                h("button", { class: "photo-view-next", disabled: busy || total < 2, "aria-label": "\u0421\u043B\u0435\u0434\u0443\u044E\u0449\u0435\u0435 \u0444\u043E\u0442\u043E", onClick: () => onStep(1) }, "\u203A")),
            h("footer", { class: "photo-view-footer" },
                h("span", null,
                    album,
                    " ",
                    h("small", null,
                        index + 1,
                        " \u0438\u0437 ",
                        total)),
                h("button", { disabled: busy, onClick: () => void act(async () => { await navigator.clipboard.writeText(new URL(photo.url, location.origin).href); setNotice("Ссылка на оригинал скопирована."); }) }, "\u041F\u043E\u0434\u0435\u043B\u0438\u0442\u044C\u0441\u044F"),
                h("button", { disabled: busy, onClick: () => { if (confirm("Удалить фотографию? Она также исчезнет из записей и сообщений; если это аватар, он будет сброшен."))
                        void act(async () => { await del(`/media/${photo.id}`); if (me?.avatar_url?.includes(photo.id))
                            setMe({ ...me, avatar_url: null }); onChanged(); onClose(); }); } }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C"),
                h("details", { ref: menu, class: "photo-view-menu" },
                    h("summary", null, "\u0415\u0449\u0451 \u2304"),
                    h("div", null,
                        h("button", { disabled: busy, onClick: () => void act(async () => { setMe(await put("/users/me/avatar", { media_id: photo.id })); setNotice("Фотография профиля обновлена."); if (menu.current)
                                menu.current.open = false; }) }, "\u0421\u0434\u0435\u043B\u0430\u0442\u044C \u0444\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0435\u0439 \u043F\u0440\u043E\u0444\u0438\u043B\u044F"),
                        h("a", { href: photo.url, target: "_blank", rel: "noopener noreferrer" }, "\u041E\u0442\u043A\u0440\u044B\u0442\u044C \u043E\u0440\u0438\u0433\u0438\u043D\u0430\u043B"))))),
        h("aside", { class: "photo-view-details" },
            me && h("header", null,
                h(Avatar, { user: me, size: 48 }),
                h("div", null,
                    h("a", { href: `/u/${me.username}` }, fullName(me)),
                    h(Time, { iso: photo.created_at }))),
            h("div", { class: "photo-view-description" },
                editing ? h("form", { class: "stack", onSubmit: e => { e.preventDefault(); void act(async () => { await patch(`/photos/${photo.id}`, { description: text }); setEditing(false); onChanged(); }); } },
                    h("label", null,
                        "\u041E\u043F\u0438\u0441\u0430\u043D\u0438\u0435 \u0444\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0438",
                        h("textarea", { value: text, maxLength: 2000, onInput: e => setText(e.currentTarget.value) })),
                    h("div", { class: "library-actions" },
                        h("button", { class: "primary", disabled: busy }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C"),
                        h("button", { class: "ghost", type: "button", disabled: busy, onClick: () => { setText(photo.description ?? ""); setEditing(false); } }, "\u041E\u0442\u043C\u0435\u043D\u0430"))) : h(Fragment, null,
                    h("p", null, photo.description),
                    h("button", { class: "link", onClick: () => setEditing(true) }, "\u0420\u0435\u0434\u0430\u043A\u0442\u0438\u0440\u043E\u0432\u0430\u0442\u044C \u043E\u043F\u0438\u0441\u0430\u043D\u0438\u0435")),
                h(ErrorLine, { error: error }),
                h("p", { role: "status" }, notice)),
            h(PhotoComments, { key: photo.id, id: photo.id })));
}
