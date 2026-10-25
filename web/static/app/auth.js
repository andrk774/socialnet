import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { login, register, verify2FA } from "./api.js";
import { ErrorLine } from "./ui.js";
export function AuthPage() {
    const [mode, setMode] = useState("login");
    const [mfa, setMfa] = useState(null);
    const [f, setF] = useState({ email: "", password: "", username: "", first_name: "", last_name: "", code: "" });
    const [error, setError] = useState(null);
    const [busy, setBusy] = useState(false);
    const field = (k) => ({ value: f[k], onInput: (e) => setF({ ...f, [k]: e.target.value }) });
    async function submit(e) {
        e.preventDefault();
        setBusy(true);
        setError(null);
        try {
            if (mfa)
                await verify2FA(mfa, f.code.trim());
            else if (mode === "login") {
                const r = await login(f.email, f.password);
                if (r.mfa_token)
                    setMfa(r.mfa_token);
            }
            else
                await register({ email: f.email, password: f.password, username: f.username, first_name: f.first_name, last_name: f.last_name });
        }
        catch (err) {
            setError(err);
        }
        finally {
            setBusy(false);
        }
    }
    return (h("main", { class: "auth" },
        h("div", { class: "auth-mark", "aria-hidden": "true" }),
        h("h1", { class: "wordmark" }, "socialnet"),
        h("p", { class: "auth-lead" }, "\u0414\u0440\u0443\u0437\u044C\u044F, \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F \u0438 \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432\u0430 \u2014 \u0431\u0435\u0437 \u0440\u0435\u043A\u043B\u0430\u043C\u044B \u0432 \u043B\u0435\u043D\u0442\u0435."),
        h("form", { class: "auth-form", onSubmit: submit },
            mfa ? (h("label", null,
                "\u041A\u043E\u0434 \u0438\u0437 \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u044F \u0438\u043B\u0438 \u0440\u0435\u0437\u0435\u0440\u0432\u043D\u044B\u0439 \u043A\u043E\u0434",
                h("input", { ...field("code"), inputMode: "numeric", autoComplete: "one-time-code", autoFocus: true, required: true }))) : (h("fieldset", null,
                h("legend", { class: "sr-only" }, mode === "login" ? "Вход" : "Регистрация"),
                mode === "register" && (h("div", { class: "row" },
                    h("label", null,
                        "\u0418\u043C\u044F",
                        h("input", { ...field("first_name"), autoComplete: "given-name", required: true, maxLength: 64 })),
                    h("label", null,
                        "\u0424\u0430\u043C\u0438\u043B\u0438\u044F",
                        h("input", { ...field("last_name"), autoComplete: "family-name", maxLength: 64 })))),
                mode === "register" && h("label", null,
                    "\u0418\u043C\u044F \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044F",
                    h("input", { ...field("username"), autoComplete: "username", pattern: "[A-Za-z0-9_]{3,32}", required: true, title: "3\u201332 \u0441\u0438\u043C\u0432\u043E\u043B\u0430: \u043B\u0430\u0442\u0438\u043D\u0438\u0446\u0430, \u0446\u0438\u0444\u0440\u044B, _" })),
                h("label", null,
                    "\u041F\u043E\u0447\u0442\u0430",
                    h("input", { ...field("email"), type: "email", autoComplete: "email", required: true })),
                h("label", null,
                    "\u041F\u0430\u0440\u043E\u043B\u044C",
                    h("input", { ...field("password"), type: "password", autoComplete: mode === "login" ? "current-password" : "new-password", minLength: 8, maxLength: 128, required: true })))),
            h(ErrorLine, { error: error }),
            h("button", { class: "primary wide", disabled: busy }, mfa ? "Подтвердить" : mode === "login" ? "Войти" : "Создать аккаунт")),
        !mfa && (h("p", { class: "auth-switch" },
            mode === "login" ? "Ещё нет аккаунта? " : "Уже есть аккаунт? ",
            h("button", { class: "link", onClick: () => { setMode(mode === "login" ? "register" : "login"); setError(null); } }, mode === "login" ? "Зарегистрироваться" : "Войти")))));
}
