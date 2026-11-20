import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, put } from "./api.js";
import { ErrorLine } from "./ui.js";
export function SMTPPanel() {
    const [data, setData] = useState();
    const [password, setPassword] = useState("");
    const [clear, setClear] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    const [saved, setSaved] = useState(false);
    useEffect(() => { void get("/admin/smtp").then(setData).catch(setError); }, []);
    function change(key, value) { if (data)
        setData({ ...data, [key]: value }); setSaved(false); }
    async function save(e) { e.preventDefault(); if (!data || busy)
        return; setBusy(true); setError(null); setSaved(false); try {
        await put("/admin/smtp", { ...data, password, clear_password: clear });
        setPassword("");
        setClear(false);
        setData(await get("/admin/smtp"));
        setSaved(true);
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    return h("section", { class: "music-panel" },
        h("h2", null, "\u041F\u043E\u0447\u0442\u0430 \u2014 SMTP"),
        h(ErrorLine, { error: error }),
        data && h("form", { onSubmit: save },
            h("fieldset", { disabled: busy, style: "display:grid;gap:14px;border:0;padding:0;min-width:0" },
                h("label", null,
                    "SMTP-\u0441\u0435\u0440\u0432\u0435\u0440",
                    h("input", { required: true, maxLength: 253, placeholder: "smtp.example.com", value: data.host, onInput: e => change("host", e.currentTarget.value) })),
                h("label", null,
                    "\u041F\u043E\u0440\u0442",
                    h("input", { type: "number", required: true, min: 1, max: 65535, value: data.port, onInput: e => change("port", Number(e.currentTarget.value)) })),
                h("label", null,
                    "\u0428\u0438\u0444\u0440\u043E\u0432\u0430\u043D\u0438\u0435",
                    h("select", { required: true, value: data.tls_mode, onChange: e => change("tls_mode", e.currentTarget.value) },
                        data.tls_mode === "auto" && h("option", { value: "auto", disabled: true }, "\u0418\u0437 \u043E\u043A\u0440\u0443\u0436\u0435\u043D\u0438\u044F \u2014 \u0432\u044B\u0431\u0435\u0440\u0438\u0442\u0435 \u0440\u0435\u0436\u0438\u043C"),
                        h("option", { value: "starttls" }, "STARTTLS (\u043E\u0431\u044B\u0447\u043D\u043E \u043F\u043E\u0440\u0442 587)"),
                        h("option", { value: "tls" }, "TLS (\u043E\u0431\u044B\u0447\u043D\u043E \u043F\u043E\u0440\u0442 465)"),
                        h("option", { value: "none" }, "\u0411\u0435\u0437 TLS \u2014 \u043B\u043E\u043A\u0430\u043B\u044C\u043D\u044B\u0439 Mailpit, \u0431\u0435\u0437 \u043B\u043E\u0433\u0438\u043D\u0430"))),
                h("label", null,
                    "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u0435\u043B\u044C",
                    h("input", { required: true, maxLength: 320, value: data.sender, placeholder: "Socialnet <no-reply@example.com>", onInput: e => change("sender", e.currentTarget.value) })),
                h("label", null,
                    "\u041B\u043E\u0433\u0438\u043D",
                    h("input", { autoComplete: "off", maxLength: 320, value: data.username, onInput: e => change("username", e.currentTarget.value) })),
                h("label", null,
                    "\u041F\u0430\u0440\u043E\u043B\u044C \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u044F",
                    h("input", { type: "password", autoComplete: "new-password", maxLength: 1024, disabled: clear, value: password, placeholder: data.password_set ? "Сохранён; пустое поле сохраняет прежний пароль" : "Пароль SMTP", onInput: e => { setPassword(e.currentTarget.value); setSaved(false); } })),
                h("label", null,
                    h("input", { type: "checkbox", checked: clear, onChange: e => { setClear(e.currentTarget.checked); setSaved(false); } }),
                    " \u0423\u0434\u0430\u043B\u0438\u0442\u044C \u0441\u043E\u0445\u0440\u0430\u043D\u0451\u043D\u043D\u044B\u0439 \u043F\u0430\u0440\u043E\u043B\u044C"),
                h("p", { class: "muted" },
                    "\u0418\u0441\u0442\u043E\u0447\u043D\u0438\u043A: ",
                    data.source === "database" ? "админ-панель" : "окружение сервера",
                    ". \u041F\u0430\u0440\u043E\u043B\u044C \u0445\u0440\u0430\u043D\u0438\u0442\u0441\u044F \u0437\u0430\u0448\u0438\u0444\u0440\u043E\u0432\u0430\u043D\u043D\u044B\u043C. \u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u043F\u0440\u0438\u043C\u0435\u043D\u044F\u044E\u0442\u0441\u044F \u043A \u0441\u043B\u0435\u0434\u0443\u044E\u0449\u0438\u043C \u043E\u0442\u043F\u0440\u0430\u0432\u043A\u0430\u043C \u0431\u0435\u0437 \u043F\u0435\u0440\u0435\u0437\u0430\u043F\u0443\u0441\u043A\u0430. \u0421\u043E\u0445\u0440\u0430\u043D\u0435\u043D\u0438\u0435 \u043D\u0435 \u043E\u0442\u043F\u0440\u0430\u0432\u043B\u044F\u0435\u0442 \u0442\u0435\u0441\u0442\u043E\u0432\u043E\u0435 \u043F\u0438\u0441\u044C\u043C\u043E."),
                h("button", { disabled: data.tls_mode === "auto", type: "submit" }, busy ? "Сохранение…" : "Сохранить настройки SMTP")),
            saved && h("p", { role: "status" }, "\u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u0441\u043E\u0445\u0440\u0430\u043D\u0435\u043D\u044B. \u0414\u043E\u0441\u0442\u0430\u0432\u043A\u0430 \u0432\u043D\u0435\u0448\u043D\u0435\u043C\u0443 \u043F\u043E\u043B\u0443\u0447\u0430\u0442\u0435\u043B\u044E \u043F\u043E\u043A\u0430 \u043D\u0435 \u043F\u0440\u043E\u0432\u0435\u0440\u0435\u043D\u0430.")));
}
