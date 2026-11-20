import { h, Fragment } from '../vendor/preact/index.js';
import { useRef, useState } from '../vendor/preact/hooks.js';
import { patch } from './api.js';
import { ErrorLine } from './ui.js';
import { Icon } from './shellparts.js';
import { setMe } from './session.js';
export function ProfileDetails({ user }) { const dialog = useRef(null); return h(Fragment, null,
    h("button", { class: "link", onClick: () => dialog.current?.showModal() }, "\u24D8 \u041F\u043E\u0434\u0440\u043E\u0431\u043D\u0435\u0435"),
    h("dialog", { ref: dialog, class: "music-dialog profile-info", "aria-label": "\u041F\u043E\u0434\u0440\u043E\u0431\u043D\u0430\u044F \u0438\u043D\u0444\u043E\u0440\u043C\u0430\u0446\u0438\u044F" },
        h("header", null,
            h("h2", null, "\u041F\u043E\u0434\u0440\u043E\u0431\u043D\u0430\u044F \u0438\u043D\u0444\u043E\u0440\u043C\u0430\u0446\u0438\u044F"),
            h("button", { type: "button", class: "profile-dialog-close", "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C", onClick: () => dialog.current?.close() },
                h(Icon, { name: "close" }))),
        h("dl", null,
            h("dt", null, "\u0418\u043C\u044F \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044F"),
            h("dd", null,
                "@",
                user.username),
            h("dt", null, "ID"),
            h("dd", null, user.id),
            h("dt", null, "\u0413\u043E\u0440\u043E\u0434"),
            h("dd", null, user.city || 'Не указан'),
            h("dt", null, "\u041E \u0441\u0435\u0431\u0435"),
            h("dd", null, user.about || 'Не заполнено'),
            user.status_text && h(Fragment, null,
                h("dt", null, "\u0421\u0442\u0430\u0442\u0443\u0441"),
                h("dd", null, user.status_text))))); }
export function ProfileActions({ user, reload }) {
    const dialog = useRef(null);
    const [draft, setDraft] = useState({ first_name: user.first_name, last_name: user.last_name ?? '', city: user.city ?? '', about: user.about ?? '', status_text: user.status_text ?? '' });
    const [error, setError] = useState(null);
    const [busy, setBusy] = useState(false);
    return h("div", { class: "profile-actions own-profile-actions" },
        h("button", { class: "ghost", onClick: () => { setDraft({ first_name: user.first_name, last_name: user.last_name ?? '', city: user.city ?? '', about: user.about ?? '', status_text: user.status_text ?? '' }); setError(null); dialog.current?.showModal(); } }, "\u0420\u0435\u0434\u0430\u043A\u0442\u0438\u0440\u043E\u0432\u0430\u0442\u044C \u043F\u0440\u043E\u0444\u0438\u043B\u044C"),
        h("a", { class: "button ghost", href: "/statistics", "aria-label": "\u0421\u0442\u0430\u0442\u0438\u0441\u0442\u0438\u043A\u0430" },
            h(Icon, { name: "statistics" })),
        h("details", { class: "profile-more" },
            h("summary", null,
                "\u0415\u0449\u0451 ",
                h(Icon, { name: "chevron" })),
            h("nav", null,
                h("a", { href: "/bookmarks" },
                    h(Icon, { name: "bookmark" }),
                    "\u041C\u043E\u0438 \u0437\u0430\u043A\u043B\u0430\u0434\u043A\u0438"),
                h("a", { href: "/articles" },
                    h(Icon, { name: "file" }),
                    "\u041C\u043E\u0438 \u0441\u0442\u0430\u0442\u044C\u0438"),
                h("a", { href: "/settings?tab=privacy" },
                    h(Icon, { name: "shield" }),
                    "\u041F\u0440\u0438\u0432\u0430\u0442\u043D\u043E\u0441\u0442\u044C"),
                h("a", { href: "/plus" },
                    h(Icon, { name: "plus" }),
                    "\u041F\u043E\u0434\u043F\u0438\u0441\u043A\u0430 \u041F\u043B\u044E\u0441"))),
        h("dialog", { ref: dialog, class: "music-dialog profile-info", "aria-label": "\u0420\u0435\u0434\u0430\u043A\u0442\u0438\u0440\u043E\u0432\u0430\u0442\u044C \u043F\u0440\u043E\u0444\u0438\u043B\u044C" },
            h("header", null,
                h("h2", null, "\u0420\u0435\u0434\u0430\u043A\u0442\u0438\u0440\u043E\u0432\u0430\u0442\u044C \u043F\u0440\u043E\u0444\u0438\u043B\u044C"),
                h("button", { type: "button", class: "profile-dialog-close", disabled: busy, "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C", onClick: () => dialog.current?.close() },
                    h(Icon, { name: "close" }))),
            h("form", { onSubmit: async (e) => { e.preventDefault(); setBusy(true); setError(null); try {
                    setMe(await patch('/users/me', draft));
                    reload();
                    dialog.current?.close();
                }
                catch (e) {
                    setError(e);
                }
                finally {
                    setBusy(false);
                } } },
                h("fieldset", { disabled: busy },
                    [['first_name', 'Имя', 64], ['last_name', 'Фамилия', 64], ['city', 'Город', 64], ['status_text', 'Статус', 140], ['about', 'О себе', 1000]].map(([key, label, max]) => h("label", null,
                        label,
                        h("input", { required: key === 'first_name', maxLength: max, value: draft[key], onInput: e => setDraft({ ...draft, [key]: e.currentTarget.value }) }))),
                    h(ErrorLine, { error: error }),
                    h("button", { class: "primary" }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C")))));
}
