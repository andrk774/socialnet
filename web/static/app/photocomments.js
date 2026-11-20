import { h } from '../vendor/preact/index.js';
import { useState } from '../vendor/preact/hooks.js';
import { get, post } from './api.js';
import { ErrorLine, Loading, Time, useLoad } from './ui.js';
import { Icon } from './shellparts.js';
export function PhotoComments({ id }) {
    const data = useLoad(() => get(`/photos/${id}/comments`), [id]);
    const [text, setText] = useState(''), [busy, setBusy] = useState(false), [error, setError] = useState(null);
    return h("section", { class: "photo-comments", "aria-label": "\u041A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438 \u043A \u0444\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0438" },
        h("div", { class: "photo-comments-list", "aria-live": "polite" },
            !data.data && !data.error && h(Loading, null),
            h(ErrorLine, { error: data.error }),
            data.data?.items.length === 0 && h("div", { class: "photo-comments-empty" },
                h(Icon, { name: "message" }),
                h("p", null, "\u041E\u0441\u0442\u0430\u0432\u044C\u0442\u0435 \u043F\u0435\u0440\u0432\u044B\u0439 \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0439 \u043A \u044D\u0442\u043E\u0439 \u0444\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0438")),
            data.data?.items.map(c => h("article", { key: c.id },
                h("a", { href: `/u/${c.username}` }, c.name || c.username),
                h("p", null, c.text),
                h(Time, { iso: c.created_at }))),
            data.data?.next_cursor && h("button", { class: "link", disabled: busy, onClick: async () => { setBusy(true); try {
                    const old = data.data;
                    const page = await get(`/photos/${id}/comments?after=${old.next_cursor}`);
                    data.set({ ...page, items: [...old.items, ...page.items] });
                }
                catch (e) {
                    setError(e);
                }
                finally {
                    setBusy(false);
                } } }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u044C \u0441\u043B\u0435\u0434\u0443\u044E\u0449\u0438\u0435 \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438")),
        h(ErrorLine, { error: error }),
        h("form", { onSubmit: async (e) => { e.preventDefault(); if (busy || !text.trim())
                return; setBusy(true); setError(null); try {
                await post(`/photos/${id}/comments`, { text });
                setText('');
                data.reload();
            }
            catch (e) {
                setError(e);
            }
            finally {
                setBusy(false);
            } } },
            h("textarea", { "aria-label": "\u041A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0439 \u043A \u0444\u043E\u0442\u043E\u0433\u0440\u0430\u0444\u0438\u0438", placeholder: "\u041D\u0430\u043F\u0438\u0441\u0430\u0442\u044C \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0439\u2026", maxLength: 2000, rows: 2, value: text, disabled: busy, onInput: e => setText(e.currentTarget.value) }),
            h("button", { class: "primary", disabled: busy || !text.trim(), "aria-label": "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0439" }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C")));
}
