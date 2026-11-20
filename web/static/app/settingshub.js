import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { del, fullName, get, patch, post, put } from "./api.js";
import { Avatar, Empty, ErrorLine, Loading, useLoad } from "./ui.js";
import { setMe, useMe } from "./session.js";
import { SettingsPage as SecuritySettings, WalletPage, notifText } from "./pages.js";
import { PlusPage } from "./plus.js";
import { query } from "./router.js";
import { hiddenSections, optionalSections } from "./navprefs.js";
import { Icon, navIcon } from "./shellparts.js";
const tabs = [["general", "Аккаунт и внешний вид"], ["security", "Безопасность"], ["privacy", "Приватность"], ["notifications", "Уведомления"], ["blocks", "Чёрный список"], ["wallet", "Кошелёк"], ["plus", "Подписка Плюс"]];
export function SettingsHub() { const tab = query("tab") ?? "general"; return h("div", { class: "settings-hub" },
    h("div", { class: "settings-body" }, tab === "security" ? h(SecuritySettings, null) : tab === "privacy" ? h(PrivacySettings, null) : tab === "notifications" ? h(NotificationSettings, null) : tab === "blocks" ? h(BlocksSettings, null) : tab === "wallet" ? h(WalletPage, null) : tab === "plus" ? h(PlusPage, null) : h(AppearanceSettings, null)),
    h("nav", { class: "settings-navigation", "aria-label": "\u0420\u0430\u0437\u0434\u0435\u043B\u044B \u043D\u0430\u0441\u0442\u0440\u043E\u0435\u043A" }, tabs.map(([id, label]) => h("a", { href: `/settings?tab=${id}`, "aria-current": tab === id ? "page" : undefined }, label)))); }
function AppearanceSettings() {
    const me = useMe();
    const menu = useRef(null);
    const [hidden, setHidden] = useState([]);
    const [error, setError] = useState(null);
    const [theme, setTheme] = useState(() => { try {
        return localStorage.getItem("sn-theme") ?? "system";
    }
    catch {
        return "system";
    } });
    function changeTheme(value) { setTheme(value); try {
        localStorage.setItem("sn-theme", value);
    }
    catch { } if (value === "system")
        delete document.documentElement.dataset.theme;
    else
        document.documentElement.dataset.theme = value; window.dispatchEvent(new Event("sn-theme-change")); }
    return h("section", null,
        h("h1", null, "\u0410\u043A\u043A\u0430\u0443\u043D\u0442 \u0438 \u0432\u043D\u0435\u0448\u043D\u0438\u0439 \u0432\u0438\u0434"),
        h("div", { class: "settings-row" },
            h("span", null, "\u041C\u0435\u043D\u044E \u0441\u0430\u0439\u0442\u0430"),
            h("button", { class: "link", onClick: () => { setHidden(hiddenSections(me.id)); setError(null); menu.current?.showModal(); } }, "\u041D\u0430\u0441\u0442\u0440\u043E\u0438\u0442\u044C \u043F\u0443\u043D\u043A\u0442\u044B \u043C\u0435\u043D\u044E")),
        h("label", { class: "settings-row" },
            h("span", null, "\u0422\u0435\u043C\u0430"),
            h("select", { value: theme, onChange: e => changeTheme(e.currentTarget.value) },
                h("option", { value: "system" }, "\u0421\u0438\u0441\u0442\u0435\u043C\u043D\u0430\u044F"),
                h("option", { value: "light" }, "\u0421\u0432\u0435\u0442\u043B\u0430\u044F"),
                h("option", { value: "dark" }, "\u0422\u0451\u043C\u043D\u0430\u044F"))),
        h("div", { class: "settings-row" },
            h("span", null, "\u0410\u0434\u0440\u0435\u0441 \u0441\u0442\u0440\u0430\u043D\u0438\u0446\u044B"),
            h("a", { href: `/u/${me.username}` },
                "@",
                me.username)),
        h("div", { class: "settings-row" },
            h("span", null, "\u042D\u043B\u0435\u043A\u0442\u0440\u043E\u043D\u043D\u0430\u044F \u043F\u043E\u0447\u0442\u0430"),
            h("span", null, me.email)),
        h("p", { class: "muted" }, "\u0422\u0435\u043C\u0430 \u0438 \u0441\u043E\u0441\u0442\u0430\u0432 \u043C\u0435\u043D\u044E \u0441\u043E\u0445\u0440\u0430\u043D\u044F\u044E\u0442\u0441\u044F \u0432 \u044D\u0442\u043E\u043C \u0431\u0440\u0430\u0443\u0437\u0435\u0440\u0435."),
        h("dialog", { ref: menu, class: "music-dialog navigation-settings", "aria-label": "\u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0430 \u043F\u0443\u043D\u043A\u0442\u043E\u0432 \u043C\u0435\u043D\u044E" },
            h("h2", null, "\u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0430 \u043F\u0443\u043D\u043A\u0442\u043E\u0432 \u043C\u0435\u043D\u044E"),
            h("p", { class: "muted" }, "\u041F\u0440\u043E\u0444\u0438\u043B\u044C, \u043B\u0435\u043D\u0442\u0430, \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F, \u0443\u0432\u0435\u0434\u043E\u043C\u043B\u0435\u043D\u0438\u044F \u0438 \u043D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u0434\u043E\u0441\u0442\u0443\u043F\u043D\u044B \u0432\u0441\u0435\u0433\u0434\u0430."),
            h("div", { class: "navigation-choices" }, optionalSections.map(([url, label]) => h("label", null,
                h(Icon, { name: navIcon(url) }),
                h("span", null, label),
                h("input", { type: "checkbox", checked: !hidden.includes(url), onChange: e => setHidden(e.currentTarget.checked ? hidden.filter(x => x !== url) : [...hidden, url]) })))),
            h(ErrorLine, { error: error }),
            h("footer", { class: "library-actions" },
                h("button", { class: "ghost", onClick: () => menu.current?.close() }, "\u041E\u0442\u043C\u0435\u043D\u0430"),
                h("button", { class: "primary", onClick: () => { try {
                        localStorage.setItem(`sn-navigation-${me.id}`, JSON.stringify(hidden));
                        window.dispatchEvent(new Event("sn-navigation"));
                        menu.current?.close();
                    }
                    catch {
                        setError(new Error("Не удалось сохранить настройки."));
                    } } }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C"))));
}
function PrivacySettings() {
    const load = useLoad(() => get("/users/me"), []);
    const [draft, setDraft] = useState(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    const [saved, setSaved] = useState(false);
    useEffect(() => { if (load.data)
        setDraft(load.data.privacy); }, [load.data]);
    if (!draft)
        return h("section", null,
            h("h1", null, "\u041F\u0440\u0438\u0432\u0430\u0442\u043D\u043E\u0441\u0442\u044C"),
            load.error ? h(ErrorLine, { error: load.error }) : h(Loading, null));
    return h("section", null,
        h("h1", null, "\u041F\u0440\u0438\u0432\u0430\u0442\u043D\u043E\u0441\u0442\u044C"),
        h("form", { onSubmit: async (e) => { e.preventDefault(); setBusy(true); setError(null); setSaved(false); try {
                setMe(await patch("/users/me/privacy", draft));
                setSaved(true);
            }
            catch (e) {
                setError(e);
            }
            finally {
                setBusy(false);
            } } },
            [["profile_visibility", "Кто видит мой профиль"], ["friends_visibility", "Кто видит моих друзей"], ["online_visibility", "Кто видит меня онлайн"], ["message_privacy", "Кто может писать мне"]].map(([key, label]) => h("label", { class: "settings-row" },
                h("span", null, label),
                h("select", { value: String(draft[key]), onChange: e => { setDraft({ ...draft, [key]: e.currentTarget.value }); setSaved(false); } },
                    h("option", { value: "everyone" }, "\u0412\u0441\u0435"),
                    h("option", { value: "friends" }, "\u0414\u0440\u0443\u0437\u044C\u044F"),
                    h("option", { value: key === "message_privacy" ? "nobody" : "only_me" }, key === "message_privacy" ? "Никто" : "Только я")))),
            h("label", { class: "settings-row" },
                h("span", null, "\u041F\u043E\u043A\u0430\u0437\u044B\u0432\u0430\u0442\u044C \u043C\u0435\u043D\u044F \u0432 \u043F\u043E\u0438\u0441\u043A\u0435"),
                h("input", { type: "checkbox", checked: draft.searchable, onChange: e => setDraft({ ...draft, searchable: e.currentTarget.checked }) })),
            h(ErrorLine, { error: error }),
            h("button", { class: "primary", disabled: busy }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C"),
            saved && h("p", { role: "status" }, "\u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u0441\u043E\u0445\u0440\u0430\u043D\u0435\u043D\u044B.")));
}
function NotificationSettings() { const load = useLoad(() => get("/users/me/notification-settings"), []); const [draft, setDraft] = useState(null); const [error, setError] = useState(null); const [busy, setBusy] = useState(false); const [saved, setSaved] = useState(false); useEffect(() => { if (load.data)
    setDraft(load.data.settings); }, [load.data]); if (!draft)
    return h("section", null,
        h("h1", null, "\u0423\u0432\u0435\u0434\u043E\u043C\u043B\u0435\u043D\u0438\u044F"),
        load.error ? h(ErrorLine, { error: load.error }) : h(Loading, null)); return h("section", null,
    h("h1", null, "\u0423\u0432\u0435\u0434\u043E\u043C\u043B\u0435\u043D\u0438\u044F"),
    h("form", { onSubmit: async (e) => { e.preventDefault(); setBusy(true); setError(null); setSaved(false); try {
            await put("/users/me/notification-settings", draft);
            setSaved(true);
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        } } },
        h("h2", null, "\u0414\u043E\u0441\u0442\u0430\u0432\u043A\u0430"),
        h("label", { class: "settings-row" },
            h("span", null, "\u0423\u0432\u0435\u0434\u043E\u043C\u043B\u0435\u043D\u0438\u044F Push"),
            h("input", { type: "checkbox", checked: draft.push_enabled, onChange: e => setDraft({ ...draft, push_enabled: e.currentTarget.checked }) })),
        h("label", { class: "settings-row" },
            h("span", null, "\u0421\u0432\u043E\u0434\u043A\u0430 \u043D\u0430 \u044D\u043B\u0435\u043A\u0442\u0440\u043E\u043D\u043D\u0443\u044E \u043F\u043E\u0447\u0442\u0443"),
            h("input", { type: "checkbox", checked: draft.email_digest, onChange: e => setDraft({ ...draft, email_digest: e.currentTarget.checked }) })),
        h("h2", null, "\u0421\u043E\u0431\u044B\u0442\u0438\u044F"),
        load.data?.available_types.map(type => h("label", { class: "settings-row" },
            h("span", null, notifText[type] ?? type),
            h("input", { type: "checkbox", checked: !draft.muted_types.includes(type), onChange: e => setDraft({ ...draft, muted_types: e.currentTarget.checked ? draft.muted_types.filter(t => t !== type) : [...draft.muted_types, type] }) }))),
        h(ErrorLine, { error: error }),
        h("button", { class: "primary", disabled: busy }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C"),
        saved && h("p", { role: "status" }, "\u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u0441\u043E\u0445\u0440\u0430\u043D\u0435\u043D\u044B."))); }
function BlocksSettings() {
    const me = useMe();
    const list = useLoad(async () => { let cursor = null; const all = []; do {
        const page = await get(`/blocks?limit=50${cursor ? `&after=${cursor}` : ""}`);
        all.push(...page.items);
        cursor = page.next_cursor;
    } while (cursor); return all; }, []);
    const [q, setQ] = useState("");
    const [search, setSearch] = useState("");
    const [error, setError] = useState(null);
    const [busy, setBusy] = useState(false);
    const modal = useRef(null);
    const results = useLoad(() => search.trim().length >= 2 ? get(`/search?type=users&q=${encodeURIComponent(search.trim())}`) : Promise.resolve({ items: [] }), [search]);
    useEffect(() => { const timer = setTimeout(() => setSearch(q), 300); return () => clearTimeout(timer); }, [q]);
    async function change(u, block) { setBusy(true); setError(null); try {
        if (block)
            await post(`/users/${u.id}/block`);
        else
            await del(`/users/${u.id}/block`);
        list.reload();
        if (block)
            modal.current?.close();
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    return h("section", null,
        h("div", { class: "library-toolbar" },
            h("h1", null, "\u0427\u0451\u0440\u043D\u044B\u0439 \u0441\u043F\u0438\u0441\u043E\u043A"),
            h("button", { class: "primary", onClick: () => { setQ(""); setSearch(""); modal.current?.showModal(); } }, "\u0414\u043E\u0431\u0430\u0432\u0438\u0442\u044C \u0432 \u0447\u0451\u0440\u043D\u044B\u0439 \u0441\u043F\u0438\u0441\u043E\u043A")),
        h(ErrorLine, { error: error || list.error }),
        !list.data && !list.error ? h(Loading, null) : list.data?.length ? h("ul", { class: "plain-list" }, list.data.map(u => h("li", null,
            h(Avatar, { user: u }),
            h("a", { href: `/u/${u.username}` }, fullName(u)),
            h("button", { class: "ghost", disabled: busy, onClick: () => change(u, false) }, "\u0420\u0430\u0437\u0431\u043B\u043E\u043A\u0438\u0440\u043E\u0432\u0430\u0442\u044C")))) : h(Empty, null, "\u0412\u0430\u0448 \u0447\u0451\u0440\u043D\u044B\u0439 \u0441\u043F\u0438\u0441\u043E\u043A \u043F\u0443\u0441\u0442."),
        h("dialog", { ref: modal, class: "music-dialog", "aria-label": "\u0414\u043E\u0431\u0430\u0432\u043B\u0435\u043D\u0438\u0435 \u0432 \u0447\u0451\u0440\u043D\u044B\u0439 \u0441\u043F\u0438\u0441\u043E\u043A" },
            h("button", { class: "music-dialog-close", "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C \u0447\u0451\u0440\u043D\u044B\u0439 \u0441\u043F\u0438\u0441\u043E\u043A", onClick: () => modal.current?.close() }, "\u00D7"),
            h("h2", null, "\u0414\u043E\u0431\u0430\u0432\u043B\u0435\u043D\u0438\u0435 \u0432 \u0447\u0451\u0440\u043D\u044B\u0439 \u0441\u043F\u0438\u0441\u043E\u043A"),
            h("label", null,
                "\u0418\u043C\u044F \u0438\u043B\u0438 \u0438\u043C\u044F \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044F",
                h("input", { type: "search", maxLength: 64, value: q, onInput: e => setQ(e.currentTarget.value) })),
            h(ErrorLine, { error: error || results.error }),
            h("ul", { class: "plain-list" }, results.data?.items.filter(u => u.id !== me.id && !list.data?.some(b => b.id === u.id)).map(u => h("li", null,
                h(Avatar, { user: u }),
                h("span", null,
                    fullName(u),
                    h("small", null,
                        " @",
                        u.username)),
                h("button", { class: "ghost", disabled: busy, onClick: () => change(u, true) }, "\u0417\u0430\u0431\u043B\u043E\u043A\u0438\u0440\u043E\u0432\u0430\u0442\u044C")))),
            search.length >= 2 && results.data?.items.length === 0 && h(Empty, null, "\u041F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u0438 \u043D\u0435 \u043D\u0430\u0439\u0434\u0435\u043D\u044B.")));
}
