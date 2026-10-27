// Сборы сообществ: цель, срок, прогресс, пожертвования монетами.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { get, post } from "./api.js";
import { Empty, ErrorLine, Loading, UserLink, useLoad } from "./ui.js";
const nf = new Intl.NumberFormat("ru");
const dayFmt = new Intl.DateTimeFormat("ru", { day: "numeric", month: "long" });
const newKey = () => (crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`);
function FundCard({ f, onChange, showCommunity }) {
    const [open, setOpen] = useState(false);
    const [amount, setAmount] = useState("100");
    const [memo, setMemo] = useState("");
    const [anon, setAnon] = useState(false);
    const [key, setKey] = useState(newKey);
    const [error, setError] = useState(null);
    const [thanks, setThanks] = useState(false);
    const donations = useLoad(() => (open ? get(`/fundraisers/${f.id}`) : Promise.resolve({ donations: [] })), [open, f.raised]);
    const pct = Math.min(100, Math.round((f.raised / f.goal) * 100));
    const donate = async (e) => {
        e.preventDefault();
        setError(null);
        try {
            await post(`/fundraisers/${f.id}/donate`, { amount: +amount, memo, anonymous: anon, idempotency_key: key });
            setKey(newKey());
            setThanks(true);
            onChange();
        }
        catch (err) {
            setError(err);
        }
    };
    return (h("li", { class: "fund" },
        h("strong", { class: "fund-title" }, f.title),
        showCommunity && h("span", { class: "muted" },
            " \u00B7 ",
            h("a", { href: `/c/${f.community.slug}` }, f.community.name)),
        f.description && h("p", { class: "ad-body" }, f.description),
        h("div", { class: "progress", role: "progressbar", "aria-valuemin": 0, "aria-valuemax": f.goal, "aria-valuenow": f.raised, "aria-label": "\u0421\u043E\u0431\u0440\u0430\u043D\u043E" },
            h("div", { class: "progress-bar", style: { width: `${pct}%` } })),
        h("p", { class: "muted" },
            "\u0421\u043E\u0431\u0440\u0430\u043D\u043E ",
            h("strong", null, nf.format(f.raised)),
            " \u0438\u0437 ",
            nf.format(f.goal),
            " \u043C\u043E\u043D\u0435\u0442 (",
            pct,
            "%) \u00B7 ",
            f.donors,
            " \u0447\u0435\u043B. \u00B7 ",
            f.open ? `до ${dayFmt.format(new Date(f.deadline))}` : "сбор завершён"),
        h("div", { class: "row tight" },
            f.open && !thanks && h("button", { class: "primary", onClick: () => setOpen(!open) }, "\u041F\u043E\u0434\u0434\u0435\u0440\u0436\u0430\u0442\u044C"),
            thanks && h("span", { class: "pill st-going" }, "\u0421\u043F\u0430\u0441\u0438\u0431\u043E!"),
            !f.open && h("button", { class: "link", onClick: () => setOpen(!open) }, "\u041A\u0442\u043E \u043F\u043E\u0434\u0434\u0435\u0440\u0436\u0430\u043B")),
        open && f.open && !thanks && (h("form", { class: "row", onSubmit: donate },
            h("label", null,
                "\u041C\u043E\u043D\u0435\u0442",
                h("input", { type: "number", min: 1, value: amount, onInput: (e) => setAmount(e.target.value) })),
            h("label", null,
                "\u041F\u043E\u0436\u0435\u043B\u0430\u043D\u0438\u0435",
                h("input", { value: memo, maxLength: 200, onInput: (e) => setMemo(e.target.value) })),
            h("label", { class: "check" },
                h("input", { type: "checkbox", checked: anon, onChange: (e) => setAnon(e.target.checked) }),
                " \u0430\u043D\u043E\u043D\u0438\u043C\u043D\u043E"),
            h("button", { class: "primary" }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C"))),
        open && donations.data && donations.data.donations.length > 0 && (h("ul", { class: "plain-list donations" }, donations.data.donations.map((d, i) => h("li", { key: i },
            d.user ? h(UserLink, { user: d.user }) : h("span", { class: "muted" }, "\u0410\u043D\u043E\u043D\u0438\u043C"),
            " \u2014 ",
            nf.format(d.amount),
            d.message && h("span", { class: "muted" },
                " \u00B7 ",
                d.message))))),
        h(ErrorLine, { error: error })));
}
export function CommunityFundraisers({ communityId, admin }) {
    const list = useLoad(() => get(`/communities/${communityId}/fundraisers`), [communityId]);
    const [f, setF] = useState({ title: "", description: "", goal: "", deadline: "" });
    const [error, setError] = useState(null);
    if (!list.data)
        return list.error ? h(ErrorLine, { error: list.error }) : null;
    if (list.data.items.length === 0 && !admin)
        return null;
    const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
    return (h("section", { class: "fundraisers" },
        h("h2", null, "\u0421\u0431\u043E\u0440\u044B"),
        list.data.items.length > 0 && h("ul", { class: "ads" }, list.data.items.map((x) => h(FundCard, { key: x.id, f: x, onChange: list.reload }))),
        admin && (h("details", { class: "create" },
            h("summary", null, "\u041E\u0431\u044A\u044F\u0432\u0438\u0442\u044C \u0441\u0431\u043E\u0440"),
            h("form", { class: "stack hike-form", onSubmit: (e) => {
                    e.preventDefault();
                    setError(null);
                    post("/communities/" + communityId + "/fundraisers", { title: f.title, description: f.description, goal: +f.goal, deadline: new Date(f.deadline).toISOString() })
                        .then(() => { setF({ title: "", description: "", goal: "", deadline: "" }); list.reload(); }, setError);
                } },
                h("label", null,
                    "\u041D\u0430 \u0447\u0442\u043E",
                    h("input", { value: f.title, required: true, maxLength: 120, onInput: set("title"), placeholder: "\u0420\u0430\u0446\u0438\u0438 \u0434\u043B\u044F \u043A\u043B\u0443\u0431\u0430" })),
                h("label", null,
                    "\u041F\u043E\u0434\u0440\u043E\u0431\u043D\u043E\u0441\u0442\u0438",
                    h("textarea", { rows: 2, maxLength: 2000, value: f.description, onInput: set("description") })),
                h("div", { class: "row-2" },
                    h("label", null,
                        "\u0426\u0435\u043B\u044C, \u043C\u043E\u043D\u0435\u0442",
                        h("input", { type: "number", min: 1, required: true, value: f.goal, onInput: set("goal") })),
                    h("label", null,
                        "\u0414\u043E",
                        h("input", { type: "datetime-local", required: true, value: f.deadline, onInput: set("deadline") }))),
                h(ErrorLine, { error: error }),
                h("button", { class: "primary" }, "\u041E\u0431\u044A\u044F\u0432\u0438\u0442\u044C"))))));
}
export function FundraisersPage() {
    const list = useLoad(() => get("/fundraisers"), []);
    return (h("section", null,
        h("h1", null, "\u0421\u0431\u043E\u0440\u044B"),
        h("p", { class: "muted" }, "\u041A\u043B\u0443\u0431\u044B \u0441\u043E\u0431\u0438\u0440\u0430\u044E\u0442 \u043D\u0430 \u0441\u043D\u0430\u0440\u044F\u0436\u0435\u043D\u0438\u0435, \u0441\u043F\u0430\u0441\u0444\u043E\u043D\u0434 \u0438 \u044D\u043A\u043E\u043B\u043E\u0433\u0438\u0447\u0435\u0441\u043A\u0438\u0435 \u0430\u043A\u0446\u0438\u0438. \u041F\u043E\u0436\u0435\u0440\u0442\u0432\u043E\u0432\u0430\u043D\u0438\u044F \u2014 \u043C\u043E\u043D\u0435\u0442\u0430\u043C\u0438 \u043D\u0430 \u0441\u0447\u0451\u0442 \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u0430."),
        list.error ? h(ErrorLine, { error: list.error }) : !list.data ? h(Loading, null) : list.data.items.length === 0 ? h(Empty, null, "\u041E\u0442\u043A\u0440\u044B\u0442\u044B\u0445 \u0441\u0431\u043E\u0440\u043E\u0432 \u043D\u0435\u0442.")
            : h("ul", { class: "ads" }, list.data.items.map((x) => h(FundCard, { key: x.id, f: x, onChange: list.reload, showCommunity: true })))));
}
