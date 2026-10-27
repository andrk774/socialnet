// Подписка «Плюс» за монеты.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { get, patch, post } from "./api.js";
import { loadMe } from "./session.js";
import { ErrorLine, Loading, useLoad } from "./ui.js";
const limitNames = {
    antennas: "Своих лент", companion_ads: "Открытых объявлений о попутчиках", articles_day: "Статей в сутки",
    offline_tiles: "Тайлов карты на офлайн-маршрут", club_emoji: "Эмодзи в вашем сообществе",
};
const dateFmt = new Intl.DateTimeFormat("ru", { day: "numeric", month: "long", year: "numeric" });
const newKey = () => (crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`);
export function PlusPage() {
    const st = useLoad(() => get("/plus"), []);
    const [key, setKey] = useState(newKey); // один ключ на одну покупку: повтор не спишет дважды
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    if (st.error)
        return h(ErrorLine, { error: st.error });
    if (!st.data)
        return h(Loading, null);
    const p = st.data;
    const buy = async () => {
        setBusy(true);
        setError(null);
        try {
            await post("/plus/buy", { idempotency_key: key });
            setKey(newKey());
            st.reload();
            await loadMe();
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    };
    return (h("section", { class: "plus-page" },
        h("h1", null,
            "socialnet ",
            h("span", { class: "plus-badge" }, "\u041F\u043B\u044E\u0441")),
        p.active
            ? h("p", { class: "note" },
                "\u041F\u043E\u0434\u043F\u0438\u0441\u043A\u0430 \u0434\u0435\u0439\u0441\u0442\u0432\u0443\u0435\u0442 \u0434\u043E ",
                dateFmt.format(new Date(p.expires_at)),
                ".",
                " ",
                h("label", { class: "check inline-check" },
                    h("input", { type: "checkbox", checked: p.auto_renew, onChange: (e) => patch("/plus", { auto_renew: e.target.checked }).then(st.reload, setError) }),
                    " \u043F\u0440\u043E\u0434\u043B\u0435\u0432\u0430\u0442\u044C \u0430\u0432\u0442\u043E\u043C\u0430\u0442\u0438\u0447\u0435\u0441\u043A\u0438"))
            : h("p", null,
                "\u0411\u043E\u043B\u044C\u0448\u0435 \u0432\u043E\u0437\u043C\u043E\u0436\u043D\u043E\u0441\u0442\u0435\u0439 \u0434\u043B\u044F \u043F\u043E\u0445\u043E\u0434\u043E\u0432 \u0438 \u043A\u043B\u0443\u0431\u043E\u0432 \u2014 ",
                p.price,
                " \u043C\u043E\u043D\u0435\u0442 \u0437\u0430 ",
                p.days,
                " \u0434\u043D\u0435\u0439."),
        h("table", { class: "plus-table" },
            h("thead", null,
                h("tr", null,
                    h("th", null),
                    h("th", null, "\u041E\u0431\u044B\u0447\u043D\u043E"),
                    h("th", null, "\u0421 \u00AB\u041F\u043B\u044E\u0441\u043E\u043C\u00BB"))),
            h("tbody", null,
                Object.entries(p.limits).map(([k, [a, b]]) => h("tr", { key: k },
                    h("td", null, limitNames[k] ?? k),
                    h("td", null, a),
                    h("td", null,
                        h("strong", null, b)))),
                h("tr", null,
                    h("td", null, "\u0417\u043D\u0430\u0447\u043E\u043A \u0432 \u043F\u0440\u043E\u0444\u0438\u043B\u0435"),
                    h("td", null, "\u2014"),
                    h("td", null,
                        h("span", { class: "plus-badge" }, "\u041F\u043B\u044E\u0441"))),
                h("tr", null,
                    h("td", null, "\u0421\u043F\u0438\u0441\u043E\u043A \u0433\u0440\u0443\u043F\u043F\u044B CSV \u0434\u043B\u044F \u0440\u0435\u0433\u0438\u0441\u0442\u0440\u0430\u0446\u0438\u0438 \u0432 \u041C\u0427\u0421"),
                    h("td", null, "\u2014"),
                    h("td", null, "\u2713")))),
        h(ErrorLine, { error: error }),
        h("button", { class: "primary", disabled: busy, onClick: buy }, p.active ? `Продлить на ${p.days} дней — ${p.price} монет` : `Подключить за ${p.price} монет`),
        h("p", { class: "hint" },
            "\u041C\u043E\u043D\u0435\u0442\u044B \u0441\u043F\u0438\u0441\u044B\u0432\u0430\u044E\u0442\u0441\u044F \u0441 \u0432\u0430\u0448\u0435\u0433\u043E \u043A\u043E\u0448\u0435\u043B\u044C\u043A\u0430. \u041F\u043E\u043F\u043E\u043B\u043D\u0438\u0442\u044C \u2014 \u0432 \u0440\u0430\u0437\u0434\u0435\u043B\u0435 ",
            h("a", { href: "/wallet" }, "\u041A\u043E\u0448\u0435\u043B\u0451\u043A"),
            ".")));
}
