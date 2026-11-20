import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, put } from "./api.js";
import { ErrorLine } from "./ui.js";
export function TURNPanel() {
    const [data, setData] = useState();
    const [secret, setSecret] = useState("");
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    const [saved, setSaved] = useState(false);
    useEffect(() => { void get("/admin/turn").then(setData).catch(setError); }, []);
    async function save(e) {
        e.preventDefault();
        if (!data || busy)
            return;
        setBusy(true);
        setError(null);
        setSaved(false);
        try {
            await put("/admin/turn", { ...data, secret });
            setSecret("");
            setData(await get("/admin/turn"));
            setSaved(true);
        }
        catch (e) {
            setError(e);
        }
        finally {
            setBusy(false);
        }
    }
    return h("section", { class: "music-panel" },
        h("h2", null, "TURN-\u0441\u0435\u0440\u0432\u0435\u0440 \u0434\u043B\u044F \u0437\u0432\u043E\u043D\u043A\u043E\u0432"),
        h(ErrorLine, { error: error }),
        data && h("form", { onSubmit: save },
            h("fieldset", { disabled: busy, style: "border:0;padding:0;display:grid;gap:14px;min-width:0" },
                h("label", null,
                    h("input", { type: "checkbox", checked: data.enabled, onChange: e => { setData({ ...data, enabled: e.currentTarget.checked }); setSaved(false); } }),
                    " \u0418\u0441\u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u044C TURN"),
                h("label", null,
                    "\u0410\u0434\u0440\u0435\u0441 STUN",
                    h("input", { value: data.stun_url, maxLength: 512, placeholder: "stun:turn.example.org:3478", onInput: e => { setData({ ...data, stun_url: e.currentTarget.value }); setSaved(false); } })),
                h("label", null,
                    "\u0410\u0434\u0440\u0435\u0441 TURN",
                    h("input", { value: data.turn_url, maxLength: 512, required: data.enabled, placeholder: "turns:turn.example.org:5349?transport=tcp", onInput: e => { setData({ ...data, turn_url: e.currentTarget.value }); setSaved(false); } })),
                h("label", null,
                    "\u041E\u0431\u0449\u0438\u0439 \u0441\u0435\u043A\u0440\u0435\u0442 coturn",
                    h("input", { type: "password", autoComplete: "new-password", value: secret, maxLength: 512, placeholder: data.secret_set ? "Секрет сохранён — оставьте пустым, чтобы не менять" : "Введите static-auth-secret", onInput: e => { setSecret(e.currentTarget.value); setSaved(false); } })),
                h("p", { class: "muted" }, "\u0421\u0435\u043A\u0440\u0435\u0442 \u0434\u043E\u043B\u0436\u0435\u043D \u0441\u043E\u0432\u043F\u0430\u0434\u0430\u0442\u044C \u0441\u043E static-auth-secret \u043D\u0430 \u0441\u0435\u0440\u0432\u0435\u0440\u0435 coturn (use-auth-secret). \u0421\u043E\u0445\u0440\u0430\u043D\u0451\u043D\u043D\u044B\u0439 \u0441\u0435\u043A\u0440\u0435\u0442 \u043D\u0435 \u043F\u043E\u043A\u0430\u0437\u044B\u0432\u0430\u0435\u0442\u0441\u044F. \u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u043F\u0440\u0438\u043C\u0435\u043D\u044F\u044E\u0442\u0441\u044F \u043A \u043D\u043E\u0432\u044B\u043C \u0437\u0432\u043E\u043D\u043A\u0430\u043C \u0431\u0435\u0437 \u043F\u0435\u0440\u0435\u0437\u0430\u043F\u0443\u0441\u043A\u0430 API."),
                h("p", { class: "muted" },
                    "\u0418\u0441\u0442\u043E\u0447\u043D\u0438\u043A: ",
                    data.source === "database" ? "админ-панель" : "переменные окружения",
                    ". \u042D\u0442\u0430 \u0444\u043E\u0440\u043C\u0430 \u043F\u043E\u0434\u043A\u043B\u044E\u0447\u0430\u0435\u0442 \u0443\u0436\u0435 \u0443\u0441\u0442\u0430\u043D\u043E\u0432\u043B\u0435\u043D\u043D\u044B\u0439 TURN-\u0441\u0435\u0440\u0432\u0435\u0440; \u043E\u043D\u0430 \u043D\u0435 \u0443\u0441\u0442\u0430\u043D\u0430\u0432\u043B\u0438\u0432\u0430\u0435\u0442 coturn \u0438 \u043D\u0435 \u043E\u0442\u043A\u0440\u044B\u0432\u0430\u0435\u0442 \u043F\u043E\u0440\u0442\u044B."),
                h("button", { type: "submit" }, busy ? "Сохранение…" : "Сохранить настройки TURN")),
            saved && h("p", { role: "status" }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0435\u043D\u043E. \u041F\u0440\u043E\u0432\u0435\u0440\u044C\u0442\u0435 \u0437\u0432\u043E\u043D\u043E\u043A \u043C\u0435\u0436\u0434\u0443 \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0430\u043C\u0438 \u0432 \u0440\u0430\u0437\u043D\u044B\u0445 \u0441\u0435\u0442\u044F\u0445.")));
}
