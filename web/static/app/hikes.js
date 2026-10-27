// Походы: список ближайших, страница похода с записью, листом ожидания и
// заявками, форма организатора. Маршрут похода — карточка из routes.tsx.
import { Fragment, h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { del, get, patch, post } from "./api.js";
import { navigate } from "./router.js";
import { RouteCard } from "./routes.js";
import { Weather } from "./weather.js";
import { Rides } from "./rides.js";
import { useMe } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, RichText, Time, UserLink, useLoad } from "./ui.js";
export const difficulties = ["", "Лёгкий", "Средний", "Сложный", "Очень сложный", "Экстремальный"];
const dateFmt = new Intl.DateTimeFormat("ru", { weekday: "short", day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });
const when = (iso) => dateFmt.format(new Date(iso));
// дата по екатеринбургскому времени (так считает прогноз)
const ymd = (iso) => new Date(new Date(iso).getTime() + 5 * 36e5).toISOString().slice(0, 10);
function Difficulty({ level }) {
    if (!level)
        return null;
    return h("span", { class: `difficulty d${level}`, title: "\u0421\u043B\u043E\u0436\u043D\u043E\u0441\u0442\u044C" },
        "●".repeat(level),
        "○".repeat(5 - level),
        " ",
        difficulties[level]);
}
function Seats({ e }) {
    if (e.capacity === null)
        return h("span", null,
            e.going_count,
            " \u0438\u0434\u0443\u0442");
    const free = Math.max(e.capacity - e.going_count, 0);
    return h("span", null,
        e.going_count,
        " \u0438\u0437 ",
        e.capacity,
        free === 0 ? " · мест нет" : ` · свободно ${free}`,
        e.waitlist_count ? ` · в очереди ${e.waitlist_count}` : "");
}
export function HikesPage() {
    const [maxDiff, setMaxDiff] = useState("");
    const list = useLoad(() => get(`/hikes${maxDiff ? `?max_difficulty=${maxDiff}` : ""}`), [maxDiff]);
    return (h("section", null,
        h("div", { class: "page-head" },
            h("h1", null, "\u041F\u043E\u0445\u043E\u0434\u044B"),
            h("a", { class: "primary button-link", href: "/hikes/new" }, "\u041E\u0440\u0433\u0430\u043D\u0438\u0437\u043E\u0432\u0430\u0442\u044C \u043F\u043E\u0445\u043E\u0434")),
        h("label", { class: "inline-field" },
            "\u0421\u043B\u043E\u0436\u043D\u043E\u0441\u0442\u044C \u0434\u043E",
            h("select", { value: maxDiff, onChange: (e) => setMaxDiff(e.target.value) },
                h("option", { value: "" }, "\u043B\u044E\u0431\u043E\u0439"),
                [1, 2, 3, 4, 5].map((d) => h("option", { key: d, value: String(d) }, difficulties[d])))),
        list.error ? h(ErrorLine, { error: list.error }) : !list.data ? h(Loading, null) : list.data.items.length === 0 ? h(Empty, null, "\u0411\u043B\u0438\u0436\u0430\u0439\u0448\u0438\u0445 \u043F\u043E\u0445\u043E\u0434\u043E\u0432 \u043D\u0435\u0442. \u041E\u0440\u0433\u0430\u043D\u0438\u0437\u0443\u0439\u0442\u0435 \u0441\u0432\u043E\u0439!") : (h("ul", { class: "hike-list" }, list.data.items.map((e) => (h("li", { key: e.id, class: "hike-item" },
            h("a", { href: `/events/${e.id}`, class: "hike-title" }, e.title),
            h("div", { class: "hike-meta" },
                h("time", { dateTime: e.starts_at }, when(e.starts_at)),
                h(Difficulty, { level: e.difficulty }),
                h(Seats, { e: e }),
                e.approval && h("span", { class: "muted" }, "\u043F\u043E \u0437\u0430\u044F\u0432\u043A\u0435"),
                e.my_status && h("span", { class: `pill st-${e.my_status}` }, statusText(e))))))))));
}
function statusText(e) {
    switch (e.my_status) {
        case "going": return "Вы идёте";
        case "pending": return "Заявка на рассмотрении";
        case "waitlist": return `Вы в очереди: ${e.my_waitlist_position ?? "?"}-й`;
        case "maybe": return "Возможно";
    }
    return "";
}
export function EventPage({ id }) {
    const me = useMe();
    const ev = useLoad(() => get(`/events/${id}`), [id]);
    const route = useLoad(() => (ev.data?.route_id ? get(`/routes/${ev.data.route_id}`) : Promise.resolve(null)), [ev.data?.route_id]);
    const going = useLoad(() => get(`/events/${id}/attendees?status=going&limit=100`), [id, ev.data]);
    const queue = useLoad(() => get(`/events/${id}/attendees?status=waitlist&limit=100`), [id, ev.data]);
    const [note, setNote] = useState("");
    const [error, setError] = useState(null);
    const [editing, setEditing] = useState(false);
    if (ev.error)
        return h(ErrorLine, { error: ev.error });
    if (!ev.data || !me)
        return h(Loading, null);
    const e = ev.data;
    const isOrganizer = e.creator_id === me.id;
    const act = async (fn) => { setError(null); try {
        await fn();
        ev.reload();
    }
    catch (err) {
        setError(err);
    } };
    if (editing)
        return h(HikeForm, { initial: e, onDone: () => { setEditing(false); ev.reload(); } });
    return (h("section", { class: "event" },
        h("p", { class: "muted" },
            h("a", { href: "/hikes" }, "\u2190 \u041F\u043E\u0445\u043E\u0434\u044B")),
        h("h1", null, e.title),
        h("dl", { class: "facts" },
            h("div", null,
                h("dt", null, "\u041A\u043E\u0433\u0434\u0430"),
                h("dd", null,
                    h("time", { dateTime: e.starts_at }, when(e.starts_at)),
                    e.ends_at && h(Fragment, null,
                        " \u2014 ",
                        h("time", { dateTime: e.ends_at }, when(e.ends_at))))),
            e.difficulty && h("div", null,
                h("dt", null, "\u0421\u043B\u043E\u0436\u043D\u043E\u0441\u0442\u044C"),
                h("dd", null,
                    h(Difficulty, { level: e.difficulty }))),
            h("div", null,
                h("dt", null, "\u0423\u0447\u0430\u0441\u0442\u043D\u0438\u043A\u0438"),
                h("dd", null,
                    h(Seats, { e: e }))),
            e.meeting_point && h("div", null,
                h("dt", null, "\u041C\u0435\u0441\u0442\u043E \u0441\u0431\u043E\u0440\u0430"),
                h("dd", null, e.meeting_point)),
            e.place && h("div", null,
                h("dt", null, "\u0413\u0434\u0435"),
                h("dd", null, e.place)),
            e.community && h("div", null,
                h("dt", null, "\u0421\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u043E"),
                h("dd", null,
                    h("a", { href: `/c/${e.community.slug}` }, e.community.name)))),
        h("div", { class: "event-actions" },
            e.my_status && h("span", { class: `pill st-${e.my_status}` }, statusText(e)),
            !e.my_status && (h("form", { class: "join", onSubmit: (x) => { x.preventDefault(); act(() => post(`/events/${id}/rsvp`, { status: "going", note })); } },
                e.approval && h("label", null,
                    "\u041F\u0430\u0440\u0430 \u0441\u043B\u043E\u0432 \u043E\u0440\u0433\u0430\u043D\u0438\u0437\u0430\u0442\u043E\u0440\u0443: \u043E\u043F\u044B\u0442, \u0441\u043D\u0430\u0440\u044F\u0436\u0435\u043D\u0438\u0435",
                    h("textarea", { value: note, maxLength: 500, rows: 2, onInput: (x) => setNote(x.target.value) })),
                h("button", { class: "primary" }, e.approval ? "Подать заявку" : e.capacity !== null && e.going_count >= e.capacity ? "Встать в очередь" : "Иду"))),
            e.my_status && !isOrganizer && h("button", { class: "ghost", onClick: () => act(() => del(`/events/${id}/rsvp`)) }, e.my_status === "pending" ? "Отозвать заявку" : e.my_status === "waitlist" ? "Выйти из очереди" : "Не пойду"),
            h("a", { class: "ghost button-link", href: `/events/${id}/print` }, "\u041B\u0438\u0441\u0442 \u0434\u043B\u044F \u043F\u0435\u0447\u0430\u0442\u0438"),
            isOrganizer && h("button", { class: "ghost", onClick: () => setEditing(true) }, "\u0418\u0437\u043C\u0435\u043D\u0438\u0442\u044C"),
            isOrganizer && h("button", { class: "ghost danger", onClick: () => confirm("Удалить поход? Участники его больше не увидят.") && act(async () => { await del(`/events/${id}`); navigate("/hikes"); }) }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C")),
        h(ErrorLine, { error: error }),
        e.description && h("div", { class: "event-text" },
            h(RichText, { text: e.description })),
        e.gear && (h("section", { class: "gear" },
            h("h2", null, "\u0421\u043D\u0430\u0440\u044F\u0436\u0435\u043D\u0438\u0435"),
            h("ul", null, e.gear.split("\n").map((g) => g.trim()).filter(Boolean).map((g, i) => h("li", { key: i }, g))))),
        route.data && h("section", null,
            h("h2", null, "\u041C\u0430\u0440\u0448\u0440\u0443\u0442"),
            h(RouteCard, { route: route.data })),
        route.data && route.data.preview.length > 0 && new Date(e.starts_at).getTime() - Date.now() < 10 * 864e5 && (e.ends_at ?? e.starts_at) >= new Date().toISOString().slice(0, 10) && (h("section", null,
            h("h2", null, "\u041F\u043E\u0433\u043E\u0434\u0430"),
            h(Weather, { lat: route.data.preview[0][0], lon: route.data.preview[0][1], elevation: route.data.max_ele, from: ymd(e.starts_at), to: ymd(e.ends_at ?? e.starts_at) }))),
        e.kind === "hike" && e.control_at && h(Control, { e: e, canMark: e.my_status === "going" || isOrganizer, onChange: ev.reload }),
        h("h2", null, "\u041F\u043E\u0434\u0432\u043E\u0437"),
        h(Rides, { eventId: e.id, meeting: e.meeting_point }),
        isOrganizer && e.pending_count > 0 && h(Applications, { id: id, onChange: ev.reload }),
        h("h2", null, "\u0418\u0434\u0443\u0442"),
        going.data ? h(People, { list: going.data.items, canRemove: isOrganizer, creator: e.creator_id, onRemove: (u) => act(() => del(`/events/${id}/attendees/${u.id}`)) }) : h(Loading, null),
        queue.data && queue.data.items.length > 0 && h(Fragment, null,
            h("h2", null, "\u041B\u0438\u0441\u0442 \u043E\u0436\u0438\u0434\u0430\u043D\u0438\u044F"),
            h(People, { list: queue.data.items, canRemove: isOrganizer, creator: e.creator_id, onRemove: (u) => act(() => del(`/events/${id}/attendees/${u.id}`)) }))));
}
// Control — контрольный срок: «вернулись» и доверенные контакты участника.
function Control({ e, canMark, onChange }) {
    const going = e.my_status === "going";
    const contacts = useLoad(() => (going && !e.returned_at ? get(`/events/${e.id}/contacts`) : Promise.resolve({ items: [] })), [e.id, going, e.returned_at]);
    const [f, setF] = useState({ name: "", email: "", consent: false });
    const [error, setError] = useState(null);
    const run = async (fn, after) => { setError(null); try {
        await fn();
        after();
    }
    catch (err) {
        setError(err);
    } };
    const open = !e.returned_at;
    return (h("section", { class: `control ${e.returned_at ? "ok" : e.alert_sent ? "alert" : ""}` },
        h("h2", null, "\u041A\u043E\u043D\u0442\u0440\u043E\u043B\u044C\u043D\u044B\u0439 \u0441\u0440\u043E\u043A"),
        h("p", null,
            h("strong", null,
                h("time", { dateTime: e.control_at }, when(e.control_at))),
            e.returned_at ? h(Fragment, null,
                " \u2014 \u0433\u0440\u0443\u043F\u043F\u0430 \u0432\u0435\u0440\u043D\u0443\u043B\u0430\u0441\u044C ",
                h("time", { dateTime: e.returned_at }, when(e.returned_at)),
                ".")
                : e.alert_sent ? " — срок прошёл, доверенным контактам отправлены письма. Отметьте возвращение, как только будет связь." : null),
        open && h("p", { class: "muted" }, "\u0415\u0441\u043B\u0438 \u043A \u0441\u0440\u043E\u043A\u0443 \u043D\u0438\u043A\u0442\u043E \u0438\u0437 \u0433\u0440\u0443\u043F\u043F\u044B \u043D\u0435 \u043D\u0430\u0436\u043C\u0451\u0442 \u00AB\u041C\u044B \u0432\u0435\u0440\u043D\u0443\u043B\u0438\u0441\u044C\u00BB, \u0434\u043E\u0432\u0435\u0440\u0435\u043D\u043D\u044B\u0435 \u043A\u043E\u043D\u0442\u0430\u043A\u0442\u044B \u0443\u0447\u0430\u0441\u0442\u043D\u0438\u043A\u043E\u0432 \u043F\u043E\u043B\u0443\u0447\u0430\u0442 \u043F\u0438\u0441\u044C\u043C\u043E: \u0447\u0442\u043E \u0441\u043E\u043E\u0431\u0449\u0438\u0442\u044C \u043F\u043E \u043D\u043E\u043C\u0435\u0440\u0443 112. \u041A\u043E\u043D\u0442\u0430\u043A\u0442\u044B \u0432\u0438\u0434\u0438\u0442\u0435 \u0442\u043E\u043B\u044C\u043A\u043E \u0432\u044B; \u043E\u043D\u0438 \u0443\u0434\u0430\u043B\u044F\u044E\u0442\u0441\u044F \u043F\u043E\u0441\u043B\u0435 \u0432\u043E\u0437\u0432\u0440\u0430\u0449\u0435\u043D\u0438\u044F \u0438\u043B\u0438 \u0447\u0435\u0440\u0435\u0437 7 \u0434\u043D\u0435\u0439 \u043F\u043E\u0441\u043B\u0435 \u0441\u0440\u043E\u043A\u0430."),
        open && canMark && h("button", { class: "primary", onClick: () => run(() => post(`/events/${e.id}/returned`), onChange) }, "\u041C\u044B \u0432\u0435\u0440\u043D\u0443\u043B\u0438\u0441\u044C"),
        open && going && !e.alert_sent && contacts.data && (h("div", { class: "contacts" },
            h("h3", null, "\u041C\u043E\u0438 \u0434\u043E\u0432\u0435\u0440\u0435\u043D\u043D\u044B\u0435 \u043A\u043E\u043D\u0442\u0430\u043A\u0442\u044B"),
            contacts.data.items.length === 0 ? h("p", { class: "muted" }, "\u041F\u043E\u043A\u0430 \u043D\u0435\u0442. \u0414\u043E\u0431\u0430\u0432\u044C\u0442\u0435 \u0442\u043E\u0433\u043E, \u043A\u0442\u043E \u0431\u0443\u0434\u0435\u0442 \u0437\u043D\u0430\u0442\u044C, \u0447\u0442\u043E \u0432\u044B \u0432 \u043F\u043E\u0445\u043E\u0434\u0435.") : (h("ul", { class: "people" }, contacts.data.items.map((c) => (h("li", { key: c.id },
                h("span", { class: "grow" },
                    c.name,
                    " \u00B7 ",
                    h("span", { class: "muted" }, c.email)),
                h("button", { class: "link", onClick: () => run(() => del(`/events/${e.id}/contacts/${c.id}`), contacts.reload) }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C")))))),
            contacts.data.items.length < 3 && (h("form", { class: "stack", onSubmit: (x) => { x.preventDefault(); run(() => post(`/events/${e.id}/contacts`, f), () => { setF({ name: "", email: "", consent: false }); contacts.reload(); }); } },
                h("div", { class: "row-2" },
                    h("label", null,
                        "\u041A\u0442\u043E \u044D\u0442\u043E",
                        h("input", { value: f.name, required: true, maxLength: 80, placeholder: "\u041C\u0430\u043C\u0430", onInput: (x) => setF({ ...f, name: x.target.value }) })),
                    h("label", null,
                        "\u041F\u043E\u0447\u0442\u0430",
                        h("input", { type: "email", value: f.email, required: true, onInput: (x) => setF({ ...f, email: x.target.value }) }))),
                h("label", { class: "check" },
                    h("input", { type: "checkbox", checked: f.consent, required: true, onChange: (x) => setF({ ...f, consent: x.target.checked }) }),
                    " \u042D\u0442\u043E\u0442 \u0447\u0435\u043B\u043E\u0432\u0435\u043A \u0437\u043D\u0430\u0435\u0442 \u043E \u043F\u043E\u0445\u043E\u0434\u0435 \u0438 \u0441\u043E\u0433\u043B\u0430\u0441\u0435\u043D \u043F\u043E\u043B\u0443\u0447\u0438\u0442\u044C \u043F\u0438\u0441\u044C\u043C\u043E"),
                h("button", { class: "ghost" }, "\u0414\u043E\u0431\u0430\u0432\u0438\u0442\u044C \u043A\u043E\u043D\u0442\u0430\u043A\u0442"))))),
        h(ErrorLine, { error: error })));
}
function People({ list, canRemove, creator, onRemove }) {
    if (list.length === 0)
        return h(Empty, null, "\u041F\u043E\u043A\u0430 \u043D\u0438\u043A\u043E\u0433\u043E.");
    return (h("ul", { class: "people" }, list.map((u) => (h("li", { key: u.id },
        h(Avatar, { user: u, size: 32 }),
        " ",
        h(UserLink, { user: u }),
        u.id === creator && h("span", { class: "muted" }, " \u00B7 \u043E\u0440\u0433\u0430\u043D\u0438\u0437\u0430\u0442\u043E\u0440"),
        canRemove && u.id !== creator && h("button", { class: "link", onClick: () => confirm("Убрать участника из похода?") && onRemove(u) }, "\u0423\u0431\u0440\u0430\u0442\u044C"))))));
}
function Applications({ id, onChange }) {
    const apps = useLoad(() => get(`/events/${id}/applications`), [id]);
    const [error, setError] = useState(null);
    const decide = async (u, approve) => {
        setError(null);
        try {
            await post(`/events/${id}/applications/${u.id}`, { approve });
            apps.reload();
            onChange();
        }
        catch (e) {
            setError(e);
        }
    };
    if (!apps.data)
        return h(Loading, null);
    return (h("section", { class: "applications" },
        h("h2", null, "\u0417\u0430\u044F\u0432\u043A\u0438"),
        h(ErrorLine, { error: error }),
        h("ul", { class: "people" }, apps.data.items.map((a) => (h("li", { key: a.user.id },
            h(Avatar, { user: a.user, size: 32 }),
            h("span", { class: "grow" },
                h(UserLink, { user: a.user }),
                " ",
                h(Time, { iso: a.created_at }),
                a.note && h(Fragment, null,
                    h("br", null),
                    h("span", { class: "muted" }, a.note))),
            h("button", { class: "primary", onClick: () => decide(a.user, true) }, "\u041F\u0440\u0438\u043D\u044F\u0442\u044C"),
            h("button", { class: "ghost", onClick: () => decide(a.user, false) }, "\u041E\u0442\u043A\u043B\u043E\u043D\u0438\u0442\u044C")))))));
}
const localInput = (iso) => {
    if (!iso)
        return "";
    const d = new Date(iso);
    return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
};
export function HikeForm({ initial, onDone }) {
    const routes = useLoad(() => get("/routes?limit=100"), []);
    const [f, setF] = useState({
        title: initial?.title ?? "", starts_at: localInput(initial?.starts_at), ends_at: localInput(initial?.ends_at),
        difficulty: initial?.difficulty ? String(initial.difficulty) : "", capacity: initial?.capacity ? String(initial.capacity) : "",
        meeting_point: initial?.meeting_point ?? "", gear: initial?.gear ?? "", description: initial?.description ?? "",
        route_id: initial?.route_id ?? "", approval: initial?.approval ?? false, control_at: localInput(initial?.control_at ?? undefined),
    });
    const [error, setError] = useState(null);
    const set = (k) => (e) => {
        const t = e.target;
        setF({ ...f, [k]: t.type === "checkbox" ? t.checked : t.value });
    };
    async function submit(e) {
        e.preventDefault();
        setError(null);
        const body = {
            title: f.title, starts_at: new Date(f.starts_at).toISOString(), ends_at: f.ends_at ? new Date(f.ends_at).toISOString() : null,
            difficulty: f.difficulty ? +f.difficulty : null, capacity: f.capacity ? +f.capacity : null,
            meeting_point: f.meeting_point, gear: f.gear, description: f.description, route_id: f.route_id || null, approval: f.approval,
            ...(initial?.alert_sent ? {} : { control_at: f.control_at ? new Date(f.control_at).toISOString() : null }),
        };
        try {
            if (initial) {
                await patch(`/events/${initial.id}`, body);
                onDone?.();
            }
            else {
                const ev = await post("/events", { ...body, kind: "hike" });
                navigate(`/events/${ev.id}`);
            }
        }
        catch (err) {
            setError(err);
        }
    }
    return (h("section", null,
        h("h1", null, initial ? "Изменить поход" : "Новый поход"),
        h("form", { class: "stack hike-form", onSubmit: submit },
            h("label", null,
                "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435",
                h("input", { value: f.title, required: true, maxLength: 200, onInput: set("title"), placeholder: "\u0422\u0430\u0433\u0430\u043D\u0430\u0439: \u0414\u0432\u0443\u0433\u043B\u0430\u0432\u0430\u044F \u0441\u043E\u043F\u043A\u0430" })),
            h("div", { class: "row-2" },
                h("label", null,
                    "\u041D\u0430\u0447\u0430\u043B\u043E",
                    h("input", { type: "datetime-local", value: f.starts_at, required: true, onInput: set("starts_at") })),
                h("label", null,
                    "\u041E\u043A\u043E\u043D\u0447\u0430\u043D\u0438\u0435",
                    h("input", { type: "datetime-local", value: f.ends_at, onInput: set("ends_at") }))),
            h("div", { class: "row-2" },
                h("label", null,
                    "\u0421\u043B\u043E\u0436\u043D\u043E\u0441\u0442\u044C",
                    h("select", { value: f.difficulty, onChange: set("difficulty") },
                        h("option", { value: "" }, "\u043D\u0435 \u0443\u043A\u0430\u0437\u0430\u043D\u0430"),
                        [1, 2, 3, 4, 5].map((d) => h("option", { key: d, value: String(d) }, difficulties[d])))),
                h("label", null,
                    "\u041C\u0435\u0441\u0442 (\u0441 \u043E\u0440\u0433\u0430\u043D\u0438\u0437\u0430\u0442\u043E\u0440\u043E\u043C)",
                    h("input", { type: "number", min: 1, max: 500, value: f.capacity, onInput: set("capacity"), placeholder: "\u0431\u0435\u0437 \u043E\u0433\u0440\u0430\u043D\u0438\u0447\u0435\u043D\u0438\u044F" }))),
            h("label", null,
                "\u041A\u043E\u043D\u0442\u0440\u043E\u043B\u044C\u043D\u044B\u0439 \u0441\u0440\u043E\u043A \u0432\u043E\u0437\u0432\u0440\u0430\u0449\u0435\u043D\u0438\u044F",
                h("input", { type: "datetime-local", value: f.control_at, disabled: initial?.alert_sent, onInput: set("control_at") }),
                h("span", { class: "hint" }, "\u041D\u0435 \u043E\u0442\u043C\u0435\u0442\u0438\u0442\u0435\u0441\u044C \u043A \u044D\u0442\u043E\u043C\u0443 \u0432\u0440\u0435\u043C\u0435\u043D\u0438 \u2014 \u0434\u043E\u0432\u0435\u0440\u0435\u043D\u043D\u044B\u043C \u043A\u043E\u043D\u0442\u0430\u043A\u0442\u0430\u043C \u0443\u0447\u0430\u0441\u0442\u043D\u0438\u043A\u043E\u0432 \u0443\u0439\u0434\u0451\u0442 \u043F\u0438\u0441\u044C\u043C\u043E")),
            h("label", null,
                "\u041C\u0435\u0441\u0442\u043E \u0441\u0431\u043E\u0440\u0430",
                h("input", { value: f.meeting_point, maxLength: 300, onInput: set("meeting_point"), placeholder: "\u0416/\u0434 \u0432\u043E\u043A\u0437\u0430\u043B \u0417\u043B\u0430\u0442\u043E\u0443\u0441\u0442\u0430, 7:00" })),
            h("label", null,
                "\u041C\u0430\u0440\u0448\u0440\u0443\u0442",
                h("select", { value: f.route_id, onChange: set("route_id") },
                    h("option", { value: "" }, "\u0431\u0435\u0437 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u0430"),
                    routes.data?.items.map((r) => h("option", { key: r.id, value: r.id }, r.title)))),
            h("label", null,
                "\u0421\u043D\u0430\u0440\u044F\u0436\u0435\u043D\u0438\u0435 \u2014 \u043F\u043E \u043F\u0443\u043D\u043A\u0442\u0443 \u0432 \u0441\u0442\u0440\u043E\u043A\u0435",
                h("textarea", { rows: 4, value: f.gear, maxLength: 2000, onInput: set("gear"), placeholder: "Треккинговые ботинки\nДождевик\n2 л воды" })),
            h("label", null,
                "\u041E\u043F\u0438\u0441\u0430\u043D\u0438\u0435",
                h("textarea", { rows: 4, value: f.description, maxLength: 5000, onInput: set("description") })),
            h("label", { class: "check" },
                h("input", { type: "checkbox", checked: f.approval, onChange: set("approval") }),
                " \u0423\u0447\u0430\u0441\u0442\u0438\u0435 \u043F\u043E \u0437\u0430\u044F\u0432\u043A\u0435: \u044F \u043E\u0434\u043E\u0431\u0440\u044F\u044E \u043A\u0430\u0436\u0434\u043E\u0433\u043E"),
            h(ErrorLine, { error: error }),
            h("div", { class: "row" },
                h("button", { class: "primary" }, initial ? "Сохранить" : "Создать поход"),
                initial && h("button", { type: "button", class: "ghost", onClick: onDone }, "\u041E\u0442\u043C\u0435\u043D\u0430")))));
}
