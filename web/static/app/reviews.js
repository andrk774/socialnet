// Отзывы: об организаторе (после похода) и о маршруте каталога.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { del, get, post } from "./api.js";
import { ReportButton } from "./report.js";
import { ErrorLine, Time, UserLink, useLoad } from "./ui.js";
import { me } from "./session.js";
const word = (n) => (n % 10 === 1 && n % 100 !== 11 ? "отзыв" : n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 10 || n % 100 >= 20) ? "отзыва" : "отзывов");
export function Stars({ value }) {
    const full = Math.round(value);
    return h("span", { class: "stars", "aria-label": `${value.toFixed(1)} из 5` },
        "★".repeat(full),
        h("span", { class: "stars-off" }, "★".repeat(5 - full)));
}
export function RatingLine({ r }) {
    if (r.count === 0)
        return h("span", { class: "muted" }, "\u043E\u0442\u0437\u044B\u0432\u043E\u0432 \u043F\u043E\u043A\u0430 \u043D\u0435\u0442");
    return h("span", { class: "rating-line" },
        h(Stars, { value: r.avg }),
        " ",
        r.avg.toFixed(1),
        " \u00B7 ",
        r.count,
        " ",
        word(r.count));
}
// ReviewForm — оценка звёздами и текст; повторная отправка исправляет отзыв.
export function ReviewForm({ path, initial, label, onSaved }) {
    const [rating, setRating] = useState(initial?.rating ?? 0);
    const [text, setText] = useState(initial?.text ?? "");
    const [done, setDone] = useState(false);
    const [error, setError] = useState(null);
    async function save(e) {
        e.preventDefault();
        setError(null);
        try {
            await post(path, { rating, text });
            setDone(true);
            onSaved();
        }
        catch (err) {
            setError(err);
        }
    }
    async function remove() {
        setError(null);
        try {
            await del(path);
            setRating(0);
            setText("");
            onSaved();
        }
        catch (err) {
            setError(err);
        }
    }
    return (h("form", { class: "review-form stack", onSubmit: save },
        h("fieldset", { class: "star-input" },
            h("legend", null, label),
            [1, 2, 3, 4, 5].map((n) => (h("label", { key: n, class: n <= rating ? "on" : "" },
                h("input", { type: "radio", name: "rating", value: n, checked: rating === n, onChange: () => setRating(n) }),
                h("span", { "aria-hidden": "true" }, "\u2605"),
                h("span", { class: "sr-only" },
                    n,
                    " \u0438\u0437 5"))))),
        h("label", null,
            "\u041E\u0442\u0437\u044B\u0432",
            h("textarea", { rows: 3, maxLength: 2000, value: text, placeholder: "\u0427\u0442\u043E \u043F\u043E\u043D\u0440\u0430\u0432\u0438\u043B\u043E\u0441\u044C, \u0447\u0442\u043E \u0441\u0442\u043E\u0438\u0442 \u0437\u043D\u0430\u0442\u044C \u0434\u0440\u0443\u0433\u0438\u043C", onInput: (e) => setText(e.target.value) })),
        h(ErrorLine, { error: error }),
        h("div", { class: "row" },
            h("button", { class: "primary", disabled: rating === 0 }, initial ? "Исправить отзыв" : "Оставить отзыв"),
            initial && h("button", { type: "button", class: "ghost danger", onClick: remove }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C"),
            done && h("span", { class: "muted", role: "status" }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0435\u043D\u043E"))));
}
export function ReviewList({ items, showEvent = false }) {
    const self = me()?.id;
    return (h("ul", { class: "reviews" }, items.map((r) => (h("li", { key: r.id },
        h("div", { class: "row tight" },
            h(Stars, { value: r.rating }),
            " ",
            h(UserLink, { user: r.author }),
            " ",
            h("span", { class: "muted" },
                h(Time, { iso: r.created_at }))),
        showEvent && r.event_id && h("p", { class: "muted" },
            "\u043F\u043E\u0445\u043E\u0434: ",
            h("a", { href: `/events/${r.event_id}` }, r.event_title)),
        r.text && h("p", null, r.text),
        r.author.id !== self && h(ReportButton, { target: { type: "review", id: r.id } }))))));
}
// OrganizerReviews — на странице профиля: оценка как организатора и отзывы.
export function OrganizerReviews({ userId }) {
    const res = useLoad(() => get(`/users/${userId}/reviews?limit=20`), [userId]);
    if (!res.data || res.data.rating.count === 0)
        return null;
    return (h("details", { class: "organizer-reviews" },
        h("summary", null,
            "\u041E\u0440\u0433\u0430\u043D\u0438\u0437\u0430\u0442\u043E\u0440 \u043F\u043E\u0445\u043E\u0434\u043E\u0432: ",
            h(RatingLine, { r: res.data.rating })),
        h(ReviewList, { items: res.data.items, showEvent: true })));
}
// RouteReviews — под маршрутом каталога: оценка, отзывы и свой отзыв.
export function RouteReviews({ routeId, ownerId }) {
    const res = useLoad(() => get(`/routes/${routeId}/reviews?limit=50`), [routeId]);
    const self = me()?.id;
    if (!res.data)
        return null;
    const mine = res.data.items.find((r) => r.author.id === self) ?? null;
    return (h("section", { class: "route-reviews" },
        h("h3", null, "\u041E\u0442\u0437\u044B\u0432\u044B \u043E \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u0435"),
        h("p", null,
            h(RatingLine, { r: res.data.rating })),
        h(ReviewList, { items: res.data.items }),
        self !== ownerId && h(ReviewForm, { path: `/routes/${routeId}/reviews`, initial: mine, label: "\u041F\u0440\u043E\u0448\u043B\u0438 \u043C\u0430\u0440\u0448\u0440\u0443\u0442? \u041E\u0446\u0435\u043D\u0438\u0442\u0435", onSaved: res.reload })));
}
