import { h, Fragment } from "../vendor/preact/index.js";
import { useRef, useState } from "../vendor/preact/hooks.js";
import { post } from "./api.js";
import { loadMe, useMe } from "./session.js";
import { ErrorLine } from "./ui.js";
export function SecurityFactor() {
    const me = useMe();
    const dialog = useRef(null);
    const [password, setPassword] = useState("");
    const [code, setCode] = useState("");
    const [secret, setSecret] = useState("");
    const [codes, setCodes] = useState([]);
    const [error, setError] = useState(null);
    const [busy, setBusy] = useState(false);
    const [mode, setMode] = useState("setup");
    function clear() { setPassword(""); setCode(""); setSecret(""); setCodes([]); setError(null); }
    async function submit(e) { e.preventDefault(); if (busy)
        return; setBusy(true); setError(null); try {
        if (mode === "disable") {
            await post("/users/me/2fa/disable", { password, code });
            await loadMe();
            dialog.current?.close();
        }
        else if (!secret) {
            const r = await post("/users/me/2fa/setup", { password });
            setSecret(r.secret);
            setPassword("");
        }
        else {
            const r = await post("/users/me/2fa/enable", { code });
            setCodes(r.backup_codes);
            setSecret("");
            setCode("");
            await loadMe();
        }
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    return h("div", { class: "security-factor" },
        h("div", { class: "security-status" },
            h("strong", null, me.two_factor_enabled ? "Двухфакторная защита включена" : "Защиту можно усилить"),
            h("p", { class: "muted" }, me.two_factor_enabled ? "При входе требуется код из приложения-аутентификатора." : "Подключите подтверждение входа одноразовым кодом.")),
        h("div", { class: "settings-row" },
            h("span", null, "\u0414\u0432\u0443\u0445\u0444\u0430\u043A\u0442\u043E\u0440\u043D\u0430\u044F \u0430\u0443\u0442\u0435\u043D\u0442\u0438\u0444\u0438\u043A\u0430\u0446\u0438\u044F"),
            h("button", { class: "ghost", onClick: () => { clear(); setMode(me.two_factor_enabled ? "disable" : "setup"); dialog.current?.showModal(); } }, me.two_factor_enabled ? "Отключить" : "Настроить")),
        h("dialog", { ref: dialog, class: "music-dialog", "aria-label": "\u0414\u0432\u0443\u0445\u0444\u0430\u043A\u0442\u043E\u0440\u043D\u0430\u044F \u0430\u0443\u0442\u0435\u043D\u0442\u0438\u0444\u0438\u043A\u0430\u0446\u0438\u044F", onClose: clear, onCancel: e => { if (busy)
                e.preventDefault(); } }, codes.length ? h(Fragment, null,
            h("h2", null, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u0435 \u0440\u0435\u0437\u0435\u0440\u0432\u043D\u044B\u0435 \u043A\u043E\u0434\u044B"),
            h("p", null, "\u041A\u0430\u0436\u0434\u044B\u0439 \u043A\u043E\u0434 \u043C\u043E\u0436\u043D\u043E \u0438\u0441\u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u044C \u043E\u0434\u0438\u043D \u0440\u0430\u0437. \u041F\u043E\u0441\u043B\u0435 \u0437\u0430\u043A\u0440\u044B\u0442\u0438\u044F \u043E\u043A\u043D\u0430 \u043A\u043E\u0434\u044B \u0431\u043E\u043B\u044C\u0448\u0435 \u043D\u0435 \u0431\u0443\u0434\u0443\u0442 \u043F\u043E\u043A\u0430\u0437\u0430\u043D\u044B."),
            h("pre", { class: "backup-codes" }, codes.join("\n")),
            h("button", { class: "primary", onClick: () => dialog.current?.close() }, "\u042F \u0441\u043E\u0445\u0440\u0430\u043D\u0438\u043B \u043A\u043E\u0434\u044B")) : h("form", { class: "stack", onSubmit: submit },
            h("h2", null, mode === "disable" ? "Отключить защиту" : "Подключить защиту"),
            !secret && h("label", null,
                "\u0422\u0435\u043A\u0443\u0449\u0438\u0439 \u043F\u0430\u0440\u043E\u043B\u044C",
                h("input", { type: "password", autoComplete: "current-password", required: true, value: password, onInput: e => setPassword(e.currentTarget.value) })),
            secret && h(Fragment, null,
                h("p", null, "\u0414\u043E\u0431\u0430\u0432\u044C\u0442\u0435 \u043A\u043B\u044E\u0447 \u0432 \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u0435-\u0430\u0443\u0442\u0435\u043D\u0442\u0438\u0444\u0438\u043A\u0430\u0442\u043E\u0440, \u0437\u0430\u0442\u0435\u043C \u0432\u0432\u0435\u0434\u0438\u0442\u0435 \u043F\u043E\u043B\u0443\u0447\u0435\u043D\u043D\u044B\u0439 \u043A\u043E\u0434."),
                h("code", { class: "totp-secret" }, secret)),
            (secret || mode === "disable") && h("label", null,
                "\u041E\u0434\u043D\u043E\u0440\u0430\u0437\u043E\u0432\u044B\u0439 \u043A\u043E\u0434",
                h("input", { required: true, inputMode: "numeric", autoComplete: "one-time-code", pattern: "[0-9]{6}", maxLength: 6, value: code, onInput: e => setCode(e.currentTarget.value) })),
            h(ErrorLine, { error: error }),
            h("div", { class: "library-actions" },
                h("button", { class: "primary", disabled: busy }, mode === "disable" ? "Отключить" : secret ? "Включить" : "Продолжить"),
                h("button", { class: "ghost", type: "button", disabled: busy, onClick: () => dialog.current?.close() }, "\u041E\u0442\u043C\u0435\u043D\u0430")))));
}
