// Страницы по ссылкам из писем: подтверждение почты и новый пароль, а также
// форма «Забыли пароль?». Работают и без входа.
import { Fragment, h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { api } from "./api.js";
import { Captcha, captchaSiteKey, tokenFrom } from "./captcha.js";
import { query } from "./router.js";
import { ErrorLine, Loading } from "./ui.js";
function Shell({ title, children }) {
    return (h("main", { class: "auth" },
        h("div", { class: "auth-mark", "aria-hidden": "true" }),
        h("h1", { class: "wordmark" }, "socialnet"),
        h("h2", null, title),
        children,
        h("p", { class: "auth-switch" },
            h("a", { href: "/" }, "\u041D\u0430 \u0433\u043B\u0430\u0432\u043D\u0443\u044E"))));
}
export function VerifyEmailPage() {
    const [state, setState] = useState("wait");
    const [error, setError] = useState(null);
    useEffect(() => {
        api("POST", "/auth/email/verify", { token: query("token") ?? "" }).then(() => setState("ok"), (e) => { setError(e); setState("error"); });
    }, []);
    return (h(Shell, { title: "\u041F\u043E\u0434\u0442\u0432\u0435\u0440\u0436\u0434\u0435\u043D\u0438\u0435 \u043F\u043E\u0447\u0442\u044B" },
        state === "wait" && h(Loading, null),
        state === "ok" && h("p", null, "\u041F\u043E\u0447\u0442\u0430 \u043F\u043E\u0434\u0442\u0432\u0435\u0440\u0436\u0434\u0435\u043D\u0430. \u0422\u0435\u043F\u0435\u0440\u044C \u0434\u043E\u0441\u0442\u0443\u043F\u043D\u044B \u043A\u043E\u0448\u0435\u043B\u0451\u043A \u0438 \u043F\u043E\u0434\u043F\u0438\u0441\u043A\u0430."),
        state === "error" && h(Fragment, null,
            h(ErrorLine, { error: error }),
            h("p", { class: "muted" }, "\u0421\u0441\u044B\u043B\u043A\u0430 \u0443\u0441\u0442\u0430\u0440\u0435\u043B\u0430? \u0412\u043E\u0439\u0434\u0438\u0442\u0435 \u0438 \u043E\u0442\u043F\u0440\u0430\u0432\u044C\u0442\u0435 \u043F\u0438\u0441\u044C\u043C\u043E \u0435\u0449\u0451 \u0440\u0430\u0437."))));
}
export function ResetPasswordPage() {
    const [password, setPassword] = useState("");
    const [done, setDone] = useState(false);
    const [error, setError] = useState(null);
    const submit = (e) => {
        e.preventDefault();
        setError(null);
        api("POST", "/auth/password/reset", { token: query("token") ?? "", password }).then(() => setDone(true), setError);
    };
    return (h(Shell, { title: "\u041D\u043E\u0432\u044B\u0439 \u043F\u0430\u0440\u043E\u043B\u044C" }, done ? h("p", null,
        "\u041F\u0430\u0440\u043E\u043B\u044C \u0438\u0437\u043C\u0435\u043D\u0451\u043D, \u0432\u0441\u0435 \u0441\u0435\u0430\u043D\u0441\u044B \u0437\u0430\u0432\u0435\u0440\u0448\u0435\u043D\u044B. ",
        h("a", { href: "/" }, "\u0412\u043E\u0439\u0434\u0438\u0442\u0435"),
        " \u0441 \u043D\u043E\u0432\u044B\u043C \u043F\u0430\u0440\u043E\u043B\u0435\u043C.") : (h("form", { class: "auth-form", onSubmit: submit },
        h("label", null,
            "\u041D\u043E\u0432\u044B\u0439 \u043F\u0430\u0440\u043E\u043B\u044C",
            h("input", { type: "password", autoComplete: "new-password", minLength: 8, maxLength: 128, required: true, value: password, onInput: (e) => setPassword(e.target.value) })),
        h(ErrorLine, { error: error }),
        h("button", { class: "primary wide" }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C")))));
}
export function ForgotPasswordPage() {
    const [email, setEmail] = useState("");
    const [sent, setSent] = useState(false);
    const [siteKey, setSiteKey] = useState("");
    const box = useRef(null);
    const [error, setError] = useState(null);
    useEffect(() => { captchaSiteKey().then(setSiteKey); }, []);
    const submit = (e) => {
        e.preventDefault();
        setError(null);
        api("POST", "/auth/password/forgot", { email, captcha_token: tokenFrom(box.current) }).then(() => setSent(true), setError);
    };
    return (h(Shell, { title: "\u0412\u043E\u0441\u0441\u0442\u0430\u043D\u043E\u0432\u043B\u0435\u043D\u0438\u0435 \u043F\u0430\u0440\u043E\u043B\u044F" }, sent ? h("p", null, "\u0415\u0441\u043B\u0438 \u0442\u0430\u043A\u043E\u0439 \u0430\u0434\u0440\u0435\u0441 \u0437\u0430\u0440\u0435\u0433\u0438\u0441\u0442\u0440\u0438\u0440\u043E\u0432\u0430\u043D, \u043C\u044B \u043E\u0442\u043F\u0440\u0430\u0432\u0438\u043B\u0438 \u043D\u0430 \u043D\u0435\u0433\u043E \u0441\u0441\u044B\u043B\u043A\u0443. \u041E\u043D\u0430 \u0434\u0435\u0439\u0441\u0442\u0432\u0443\u0435\u0442 \u043E\u0433\u0440\u0430\u043D\u0438\u0447\u0435\u043D\u043D\u043E\u0435 \u0432\u0440\u0435\u043C\u044F.") : (h("form", { class: "auth-form", onSubmit: submit },
        h("label", null,
            "\u041F\u043E\u0447\u0442\u0430",
            h("input", { type: "email", autoComplete: "email", required: true, value: email, onInput: (e) => setEmail(e.target.value) })),
        h(Captcha, { siteKey: siteKey, box: box }),
        h(ErrorLine, { error: error }),
        h("button", { class: "primary wide" }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C \u0441\u0441\u044B\u043B\u043A\u0443")))));
}
