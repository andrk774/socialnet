// Согласия: три отдельные отметки при регистрации и при новой редакции
// документов; удаление аккаунта (отзыв согласия на обработку ПДн).
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { api, logout, post } from "./api.js";
import { loadMe } from "./session.js";
import { ErrorLine } from "./ui.js";
export const noConsents = { age_confirmed: false, accept_terms: false, pd_consent: false };
const doc = (key, text) => h("a", { href: `/legal/${key}`, target: "_blank", rel: "noopener" }, text);
export function ConsentChecks({ value, onChange }) {
    const box = (k) => ({
        type: "checkbox", required: true, checked: value[k],
        onChange: (e) => onChange({ ...value, [k]: e.target.checked }),
    });
    return (h("fieldset", { class: "consents" },
        h("legend", { class: "sr-only" }, "\u0421\u043E\u0433\u043B\u0430\u0441\u0438\u044F"),
        h("label", { class: "check" },
            h("input", { ...box("age_confirmed") }),
            " ",
            h("span", null, "\u041C\u043D\u0435 \u0438\u0441\u043F\u043E\u043B\u043D\u0438\u043B\u043E\u0441\u044C 18 \u043B\u0435\u0442")),
        h("label", { class: "check" },
            h("input", { ...box("accept_terms") }),
            " ",
            h("span", null,
                "\u041F\u0440\u0438\u043D\u0438\u043C\u0430\u044E ",
                doc("terms", "пользовательское соглашение"),
                " \u0438 ",
                doc("privacy", "политику обработки персональных данных"))),
        h("label", { class: "check" },
            h("input", { ...box("pd_consent") }),
            " ",
            h("span", null,
                "\u0414\u0430\u044E ",
                doc("consent", "согласие на обработку персональных данных")))));
}
// ConsentGate: документы обновились — до принятия сайт только для чтения.
export function ConsentGate() {
    const [c, setC] = useState(noConsents);
    const [error, setError] = useState(null);
    async function submit(e) {
        e.preventDefault();
        setError(null);
        try {
            await post("/users/me/consents", c);
            await loadMe();
        }
        catch (err) {
            setError(err);
        }
    }
    return (h("main", { class: "auth consent-gate" },
        h("h1", null, "\u041C\u044B \u043E\u0431\u043D\u043E\u0432\u0438\u043B\u0438 \u0434\u043E\u043A\u0443\u043C\u0435\u043D\u0442\u044B"),
        h("p", null, "\u0427\u0442\u043E\u0431\u044B \u043F\u0440\u043E\u0434\u043E\u043B\u0436\u0438\u0442\u044C, \u043F\u0440\u043E\u0447\u0438\u0442\u0430\u0439\u0442\u0435 \u043D\u043E\u0432\u0443\u044E \u0440\u0435\u0434\u0430\u043A\u0446\u0438\u044E \u0438 \u043F\u043E\u0434\u0442\u0432\u0435\u0440\u0434\u0438\u0442\u0435 \u0441\u043E\u0433\u043B\u0430\u0441\u0438\u0435. \u0415\u0441\u043B\u0438 \u043D\u0435 \u0441\u043E\u0433\u043B\u0430\u0441\u043D\u044B, \u043C\u043E\u0436\u043D\u043E \u0432\u044B\u0439\u0442\u0438 \u0438\u043B\u0438 \u0443\u0434\u0430\u043B\u0438\u0442\u044C \u0430\u043A\u043A\u0430\u0443\u043D\u0442 \u0432 \u0440\u0430\u0437\u0434\u0435\u043B\u0435 \u00AB\u0411\u0435\u0437\u043E\u043F\u0430\u0441\u043D\u043E\u0441\u0442\u044C\u00BB."),
        h("form", { class: "auth-form", onSubmit: submit },
            h(ConsentChecks, { value: c, onChange: setC }),
            h(ErrorLine, { error: error }),
            h("button", { class: "primary wide" }, "\u041F\u0440\u043E\u0434\u043E\u043B\u0436\u0438\u0442\u044C")),
        h("p", { class: "auth-switch" },
            h("button", { class: "link", onClick: () => logout() }, "\u0412\u044B\u0439\u0442\u0438"),
            " \u00B7 ",
            h("a", { href: "/settings" }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C \u0430\u043A\u043A\u0430\u0443\u043D\u0442"))));
}
export function DeleteAccount() {
    const [open, setOpen] = useState(false);
    const [password, setPassword] = useState("");
    const [sure, setSure] = useState("");
    const [error, setError] = useState(null);
    async function submit(e) {
        e.preventDefault();
        setError(null);
        try {
            await api("DELETE", "/users/me", { password });
            await logout().catch(() => { });
            location.assign("/");
        }
        catch (err) {
            setError(err);
        }
    }
    return (h("section", { class: "danger-zone" },
        h("h2", null, "\u0423\u0434\u0430\u043B\u0435\u043D\u0438\u0435 \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430"),
        h("p", { class: "muted" }, "\u0423\u0434\u0430\u043B\u044F\u044E\u0442\u0441\u044F \u0437\u0430\u043F\u0438\u0441\u0438, \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438, \u0444\u043E\u0442\u043E, \u0432\u0438\u0434\u0435\u043E, \u0434\u043E\u043A\u0443\u043C\u0435\u043D\u0442\u044B, \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u044B, \u043F\u043E\u0445\u043E\u0434\u044B \u0438 \u043E\u0431\u044A\u044F\u0432\u043B\u0435\u043D\u0438\u044F, \u0438\u043C\u044F \u0438 \u043F\u043E\u0447\u0442\u0430 \u0441\u0442\u0438\u0440\u0430\u044E\u0442\u0441\u044F \u2014 \u044D\u0442\u043E \u043E\u0442\u0437\u044B\u0432 \u0441\u043E\u0433\u043B\u0430\u0441\u0438\u044F \u043D\u0430 \u043E\u0431\u0440\u0430\u0431\u043E\u0442\u043A\u0443 \u043F\u0435\u0440\u0441\u043E\u043D\u0430\u043B\u044C\u043D\u044B\u0445 \u0434\u0430\u043D\u043D\u044B\u0445. \u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F, \u0443\u0436\u0435 \u043E\u0442\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u043D\u044B\u0435 \u0434\u0440\u0443\u0433\u0438\u043C, \u043E\u0441\u0442\u0430\u043D\u0443\u0442\u0441\u044F \u0443 \u043D\u0438\u0445 \u043E\u0442 \u00AB\u0443\u0434\u0430\u043B\u0451\u043D\u043D\u043E\u0433\u043E \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430\u00BB. \u041C\u043E\u043D\u0435\u0442\u044B \u043D\u0430 \u0441\u0447\u0451\u0442\u0435 \u0441\u0433\u043E\u0440\u044F\u0442. \u0421\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u0430 \u043F\u0435\u0440\u0435\u0439\u0434\u0443\u0442 \u043A \u0441\u0442\u0430\u0440\u0448\u0438\u043C \u0443\u0447\u0430\u0441\u0442\u043D\u0438\u043A\u0430\u043C. \u041E\u0442\u043C\u0435\u043D\u0438\u0442\u044C \u0443\u0434\u0430\u043B\u0435\u043D\u0438\u0435 \u043D\u0435\u043B\u044C\u0437\u044F."),
        !open ? h("button", { class: "ghost danger", onClick: () => setOpen(true) }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u2026") : (h("form", { class: "stack", onSubmit: submit },
            h("label", null,
                "\u041F\u0430\u0440\u043E\u043B\u044C",
                h("input", { type: "password", autoComplete: "current-password", required: true, value: password, onInput: (e) => setPassword(e.target.value) })),
            h("label", null,
                "\u041D\u0430\u043F\u0438\u0448\u0438\u0442\u0435 \u00AB\u0443\u0434\u0430\u043B\u0438\u0442\u044C\u00BB",
                h("input", { value: sure, required: true, onInput: (e) => setSure(e.target.value) })),
            h(ErrorLine, { error: error }),
            h("div", { class: "row" },
                h("button", { class: "ghost danger", disabled: sure.trim().toLowerCase() !== "удалить" }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C \u043D\u0430\u0432\u0441\u0435\u0433\u0434\u0430"),
                h("button", { type: "button", class: "ghost", onClick: () => setOpen(false) }, "\u041E\u0442\u043C\u0435\u043D\u0430"))))));
}
