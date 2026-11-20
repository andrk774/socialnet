import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { fullName, logout } from "./api.js";
import { Avatar } from "./ui.js";
const paths = {
    phone: "M6 3H3v3c0 8 7 15 15 15h3v-5l-5-2-2 2c-3-1-5-3-6-6l2-2-2-5z",
    photo: "M4 3h16a1 1 0 0 1 1 1v16a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1zM3 17l6-6 4 4 3-3 5 5M15 7h.01",
    profile: "M20 21v-2a7 7 0 0 0-14 0v2M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8",
    feed: "M4 4h16v16H4zM4 9h16M8 13h8M8 17h5",
    message: "M21 11a9 9 0 0 1-9 9H4l-2 2V11a9 9 0 0 1 19 0M7 9h9M7 13h6",
    bell: "M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4",
    people: "M16 21v-2a6 6 0 0 0-12 0v2M10 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8M17 4a4 4 0 0 1 0 7M20 21v-3a5 5 0 0 0-3-4",
    music: "M9 18V5l11-2v13M9 9l11-2M9 18a3 3 0 1 1-3-3c2 0 3 1 3 3M20 16a3 3 0 1 1-3-3c2 0 3 1 3 3",
    bookmark: "M6 3h12v19l-6-4-6 4z",
    video: "M3 4h18v16H3zM10 8l6 4-6 4z",
    upload: "M12 16V3M6 9l6-6 6 6M4 16v5h16v-5",
    file: "M5 2h9l5 5v15H5zM14 2v6h5M8 12h8M8 16h8",
    settings: "M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8M9 3h6l1 3 3 1 2 5-2 5-3 1-1 3H9l-1-3-3-1-2-5 2-5 3-1z",
    search: "M21 21l-5-5M10 18a8 8 0 1 0 0-16 8 8 0 0 0 0 16",
    map: "M3 5l6-3 6 3 6-3v17l-6 3-6-3-6 3zM9 2v17M15 5v17",
    wallet: "M3 5h17v16H3zM3 5V3h14v2M15 11h6v5h-6z",
    qr: "M3 3h6v6H3zM15 3h6v6h-6zM3 15h6v6H3zM15 15h2v2h4v4h-6z",
    theme: "M20 15A9 9 0 0 1 9 4a9 9 0 1 0 11 11",
    exit: "M9 3H3v18h6M8 12h13M17 8l4 4-4 4",
    plus: "M12 5v14M5 12h14",
};
export function Icon({ name }) {
    return h("svg", { class: "nav-icon", width: "22", height: "22", viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", "stroke-width": "1.6", "stroke-linecap": "round", "stroke-linejoin": "round", "aria-hidden": "true" },
        h("path", { d: paths[name] ?? paths.map }));
}
export function navIcon(href) {
    if (href === "/calls")
        return "phone";
    if (href === "/photos")
        return "photo";
    if (href === "/videos")
        return "video";
    return { "/": "feed", "/im": "message", "/notifications": "bell", "/communities": "people", "/music": "music", "/bookmarks": "bookmark", "/documents": "file", "/settings": "settings", "/wallet": "wallet", "/search": "search", "/articles": "file", "/plus": "plus" }[href] ?? "map";
}
export function AccountMenu({ user }) {
    const ref = useRef(null);
    const [theme, setTheme] = useState(() => {
        try {
            const saved = localStorage.getItem("sn-theme");
            return saved === "light" || saved === "dark" ? saved : "system";
        }
        catch {
            return "system";
        }
    });
    useEffect(() => {
        if (theme === "system")
            delete document.documentElement.dataset.theme;
        else
            document.documentElement.dataset.theme = theme;
        try {
            localStorage.setItem("sn-theme", theme);
        }
        catch { /* storage may be unavailable */ }
    }, [theme]);
    useEffect(() => { const sync = () => { try {
        const value = localStorage.getItem("sn-theme");
        setTheme(value === "light" || value === "dark" ? value : "system");
    }
    catch { } }; window.addEventListener("sn-theme-change", sync); return () => window.removeEventListener("sn-theme-change", sync); }, []);
    useEffect(() => {
        const outside = (event) => { if (ref.current && !ref.current.contains(event.target))
            ref.current.open = false; };
        const escape = (event) => { if (event.key === "Escape" && ref.current?.open) {
            ref.current.open = false;
            ref.current.querySelector("summary")?.focus();
        } };
        document.addEventListener("pointerdown", outside);
        document.addEventListener("keydown", escape);
        return () => { document.removeEventListener("pointerdown", outside); document.removeEventListener("keydown", escape); };
    }, []);
    return h("details", { class: "account-menu", ref: ref },
        h("summary", { "aria-label": "\u041C\u0435\u043D\u044E \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430" },
            h(Avatar, { user: user, size: 32 }),
            h("svg", { class: "account-chevron", width: "16", height: "16", viewBox: "0 0 16 16", "aria-hidden": "true" },
                h("path", { d: "m4 6 4 4 4-4", fill: "none", stroke: "currentColor", "stroke-width": "1.8", "stroke-linecap": "round", "stroke-linejoin": "round" }))),
        h("div", { class: "account-panel", onClick: (event) => { if (event.target.closest("a") && ref.current)
                ref.current.open = false; } },
            h("a", { class: "account-identity", href: `/u/${user.username}` },
                h(Avatar, { user: user, size: 64 }),
                h("strong", null, fullName(user)),
                h("span", { class: "muted" },
                    "@",
                    user.username)),
            h("a", { class: "account-manage", href: "/settings" }, "\u0423\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u0438\u0435 \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u043E\u043C"),
            h("nav", { "aria-label": "\u0410\u043A\u043A\u0430\u0443\u043D\u0442" },
                h("a", { href: "/wallet" },
                    h(Icon, { name: "wallet" }),
                    "\u041A\u043E\u0448\u0435\u043B\u0451\u043A"),
                h("a", { href: "/bookmarks" },
                    h(Icon, { name: "bookmark" }),
                    "\u041C\u043E\u0438 \u0437\u0430\u043A\u043B\u0430\u0434\u043A\u0438"),
                h("a", { href: "/settings" },
                    h(Icon, { name: "settings" }),
                    "\u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438"),
                h("button", { onClick: () => setTheme(theme === "system" ? "light" : theme === "light" ? "dark" : "system") },
                    h(Icon, { name: "theme" }),
                    "\u0422\u0435\u043C\u0430: ",
                    { system: "Системная", light: "Светлая", dark: "Тёмная" }[theme]),
                h("button", { onClick: () => logout() },
                    h(Icon, { name: "exit" }),
                    "\u0412\u044B\u0439\u0442\u0438"))));
}
export function FeedAside() {
    return h("aside", { class: "feed-aside", "aria-label": "\u041D\u0430\u0432\u0438\u0433\u0430\u0446\u0438\u044F \u043F\u043E \u043B\u0435\u043D\u0442\u0435" },
        h("nav", { class: "aside-card" },
            h("a", { class: "aside-selected", href: "/" },
                "\u041B\u0435\u043D\u0442\u0430 ",
                h(Icon, { name: "feed" })),
            h("a", { href: "/feeds" }, "\u041D\u0430\u0441\u0442\u0440\u043E\u0438\u0442\u044C \u043C\u043E\u0438 \u043B\u0435\u043D\u0442\u044B"),
            h("a", { href: "/bookmarks" }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0451\u043D\u043D\u044B\u0435 \u0437\u0430\u043F\u0438\u0441\u0438"),
            h("a", { href: "/search" }, "\u041D\u0430\u0439\u0442\u0438 \u043B\u044E\u0434\u0435\u0439 \u0438 \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u0430"),
            h("a", { href: "/scheduled" }, "\u041E\u0442\u043B\u043E\u0436\u0435\u043D\u043D\u044B\u0435 \u043F\u0443\u0431\u043B\u0438\u043A\u0430\u0446\u0438\u0438")),
        h("section", { class: "aside-card aside-discover" },
            h("span", { class: "muted" }, "\u0412\u043D\u0435 \u043B\u0435\u043D\u0442\u044B"),
            h("h2", null, "\u041F\u043B\u0430\u043D\u044B \u043D\u0430 \u0432\u044B\u0445\u043E\u0434\u043D\u044B\u0435"),
            h("p", null, "\u041D\u0430\u0439\u0434\u0438\u0442\u0435 \u043F\u043E\u0445\u043E\u0434 \u0438 \u043A\u043E\u043C\u043F\u0430\u043D\u0438\u044E \u0434\u043B\u044F \u043D\u043E\u0432\u043E\u0433\u043E \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u0430."),
            h("a", { href: "/hikes" }, "\u041F\u043E\u0441\u043C\u043E\u0442\u0440\u0435\u0442\u044C \u043F\u043E\u0445\u043E\u0434\u044B")),
        h("div", { class: "aside-footer" },
            h("a", { href: "/legal/terms" }, "\u0423\u0441\u043B\u043E\u0432\u0438\u044F \u0438\u0441\u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u043D\u0438\u044F"),
            h("a", { href: "/legal/privacy" }, "\u041A\u043E\u043D\u0444\u0438\u0434\u0435\u043D\u0446\u0438\u0430\u043B\u044C\u043D\u043E\u0441\u0442\u044C"),
            h("span", null, "socialnet")));
}
