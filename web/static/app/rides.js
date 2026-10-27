// Подвоз к началу маршрута: водитель предлагает места, попутчики просятся.
// Оплату приложение не принимает — о бензине договариваются в сообщениях.
import { Fragment, h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { del, get, post } from "./api.js";
import { useMe } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, UserLink, useLoad } from "./ui.js";
const fmt = new Intl.DateTimeFormat("ru", { weekday: "short", day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });
function RideCard({ ride, onChange }) {
    const me = useMe();
    const mine = ride.driver.id === me?.id;
    const [note, setNote] = useState("");
    const [asking, setAsking] = useState(false);
    const [error, setError] = useState(null);
    const reqs = useLoad(() => (mine ? get(`/rides/${ride.id}/requests`) : Promise.resolve({ items: [] })), [ride.id, mine, ride.taken, ride.pending]);
    const run = (fn) => { setError(null); fn().then(onChange, setError); };
    const free = ride.seats - ride.taken;
    return (h("li", { class: "ride", id: `ride-${ride.id}` },
        h("div", { class: "ad-head" },
            h(Avatar, { user: ride.driver, size: 36 }),
            h("div", { class: "grow" },
                h("strong", null,
                    ride.from,
                    " \u2192 ",
                    ride.to),
                h("div", { class: "hike-meta" },
                    h("span", null, fmt.format(new Date(ride.departs_at))),
                    h("span", null,
                        "\u043C\u0435\u0441\u0442: ",
                        free > 0 ? `свободно ${free} из ${ride.seats}` : "нет"),
                    h("span", null,
                        "\u0432\u043E\u0434\u0438\u0442\u0435\u043B\u044C ",
                        h(UserLink, { user: ride.driver }))))),
        ride.note && h("p", { class: "ad-body" }, ride.note),
        h("div", { class: "row tight" },
            ride.my_status === "accepted" && h("span", { class: "pill st-going" }, "\u0412\u044B \u0435\u0434\u0435\u0442\u0435"),
            ride.my_status === "pending" && h("span", { class: "pill st-pending" }, "\u0417\u0430\u044F\u0432\u043A\u0430 \u0443 \u0432\u043E\u0434\u0438\u0442\u0435\u043B\u044F"),
            !mine && !ride.my_status && !asking && h("button", { class: "ghost", onClick: () => setAsking(true) }, "\u041F\u043E\u043F\u0440\u043E\u0441\u0438\u0442\u044C\u0441\u044F"),
            !mine && ride.my_status && h("button", { class: "link", onClick: () => run(() => del(`/rides/${ride.id}/request`)) }, ride.my_status === "accepted" ? "Не поеду" : "Отозвать"),
            !mine && h("a", { class: "link", href: `/im/u/${ride.driver.id}` }, "\u041D\u0430\u043F\u0438\u0441\u0430\u0442\u044C \u0432\u043E\u0434\u0438\u0442\u0435\u043B\u044E"),
            mine && h("button", { class: "link", onClick: () => confirm("Отменить поездку? Попутчики получат уведомление.") && run(() => del(`/rides/${ride.id}`)) }, "\u041E\u0442\u043C\u0435\u043D\u0438\u0442\u044C \u043F\u043E\u0435\u0437\u0434\u043A\u0443")),
        asking && (h("form", { class: "reply", onSubmit: (e) => { e.preventDefault(); run(() => post(`/rides/${ride.id}/request`, { note })); setAsking(false); } },
            h("label", null,
                "\u041F\u0430\u0440\u0430 \u0441\u043B\u043E\u0432 \u0432\u043E\u0434\u0438\u0442\u0435\u043B\u044E",
                h("input", { value: note, maxLength: 300, onInput: (e) => setNote(e.target.value), placeholder: "\u041E\u0442\u043A\u0443\u0434\u0430 \u0437\u0430\u0431\u0435\u0440\u0451\u0442\u0435, \u0441\u043A\u043E\u043B\u044C\u043A\u043E \u0432\u0435\u0449\u0435\u0439" })),
            h("button", { class: "primary" }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C"))),
        mine && reqs.data && reqs.data.items.length > 0 && (h("ul", { class: "people" }, reqs.data.items.map((q) => (h("li", { key: q.user.id },
            h(Avatar, { user: q.user, size: 28 }),
            h("span", { class: "grow" },
                h(UserLink, { user: q.user }),
                q.status === "accepted" ? " · едет" : "",
                q.note && h(Fragment, null,
                    h("br", null),
                    h("span", { class: "muted" }, q.note))),
            q.status === "pending" && h("button", { class: "primary", disabled: free <= 0, onClick: () => run(() => post(`/rides/${ride.id}/requests/${q.user.id}`, { accept: true })) }, "\u0412\u0437\u044F\u0442\u044C"),
            h("button", { class: "ghost", onClick: () => run(() => post(`/rides/${ride.id}/requests/${q.user.id}`, { accept: false })) }, q.status === "accepted" ? "Высадить" : "Отказать")))))),
        h(ErrorLine, { error: error })));
}
function OfferRide({ eventId, meeting, onDone }) {
    const [f, setF] = useState({ from: "Миасс, ", to: meeting ?? "", departs_at: "", seats: "3", note: "" });
    const [error, setError] = useState(null);
    const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
    return (h("details", { class: "create" },
        h("summary", null, "\u041F\u0440\u0435\u0434\u043B\u043E\u0436\u0438\u0442\u044C \u043C\u0435\u0441\u0442\u0430 \u0432 \u043C\u0430\u0448\u0438\u043D\u0435"),
        h("form", { class: "stack hike-form", onSubmit: (e) => {
                e.preventDefault();
                setError(null);
                post("/rides", { ...f, seats: +f.seats, departs_at: new Date(f.departs_at).toISOString(), event_id: eventId ?? null })
                    .then(() => { setF({ ...f, departs_at: "", note: "" }); onDone(); }, setError);
            } },
            h("div", { class: "row-2" },
                h("label", null,
                    "\u041E\u0442\u043A\u0443\u0434\u0430",
                    h("input", { value: f.from, required: true, maxLength: 150, onInput: set("from") })),
                h("label", null,
                    "\u041A\u0443\u0434\u0430",
                    h("input", { value: f.to, required: true, maxLength: 150, onInput: set("to"), placeholder: "\u0422\u0430\u0433\u0430\u043D\u0430\u0439, \u0411\u0435\u043B\u044B\u0439 \u043A\u043B\u044E\u0447" }))),
            h("div", { class: "row-2" },
                h("label", null,
                    "\u041E\u0442\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u0438\u0435",
                    h("input", { type: "datetime-local", value: f.departs_at, required: true, onInput: set("departs_at") })),
                h("label", null,
                    "\u0421\u0432\u043E\u0431\u043E\u0434\u043D\u044B\u0445 \u043C\u0435\u0441\u0442",
                    h("input", { type: "number", min: 1, max: 8, value: f.seats, onInput: set("seats") }))),
            h("label", null,
                "\u041A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0439",
                h("input", { value: f.note, maxLength: 500, onInput: set("note"), placeholder: "\u041C\u0430\u0448\u0438\u043D\u0430, \u0431\u0430\u0433\u0430\u0436, \u0433\u0434\u0435 \u0437\u0430\u0431\u0440\u0430\u0442\u044C" })),
            h("p", { class: "hint" }, "\u041E\u043F\u043B\u0430\u0442\u0443 \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u0435 \u043D\u0435 \u043F\u0440\u0438\u043D\u0438\u043C\u0430\u0435\u0442 \u2014 \u043E \u0441\u043A\u043B\u0430\u0434\u0447\u0438\u043D\u0435 \u043D\u0430 \u0431\u0435\u043D\u0437\u0438\u043D \u0434\u043E\u0433\u043E\u0432\u043E\u0440\u0438\u0442\u0435\u0441\u044C \u0432 \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F\u0445."),
            h(ErrorLine, { error: error }),
            h("button", { class: "primary" }, "\u041F\u0440\u0435\u0434\u043B\u043E\u0436\u0438\u0442\u044C"))));
}
export function Rides({ eventId, meeting }) {
    const [to, setTo] = useState("");
    const [q, setQ] = useState("");
    const url = eventId ? `/rides?event_id=${eventId}` : `/rides${q ? `?to=${encodeURIComponent(q)}` : ""}`;
    const list = useLoad(() => get(url), [url]);
    return (h("section", { class: "rides" },
        !eventId && h("form", { class: "row", onSubmit: (e) => { e.preventDefault(); setQ(to); } },
            h("label", null,
                "\u041A\u0443\u0434\u0430",
                h("input", { value: to, onInput: (e) => setTo(e.target.value), placeholder: "\u0422\u0430\u0433\u0430\u043D\u0430\u0439" })),
            h("button", { class: "primary" }, "\u041D\u0430\u0439\u0442\u0438")),
        list.error ? h(ErrorLine, { error: list.error }) : !list.data ? h(Loading, null) : list.data.items.length === 0
            ? h(Empty, null, eventId ? "Пока никто не предложил места." : "Поездок не нашлось.")
            : h("ul", { class: "ads" }, list.data.items.map((r) => h(RideCard, { key: r.id, ride: r, onChange: list.reload }))),
        h(OfferRide, { eventId: eventId, meeting: meeting, onDone: list.reload })));
}
export function RidesPage() {
    return h("section", null,
        h("h1", null, "\u041F\u043E\u0434\u0432\u043E\u0437 \u043A \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u0430\u043C"),
        h(Rides, null));
}
