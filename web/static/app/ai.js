import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, put, post } from "./api.js";
import { ErrorLine } from "./ui.js";
export function AIPanel() {
    const [data, setData] = useState();
    const [key, setKey] = useState("");
    const [clear, setClear] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    const [saved, setSaved] = useState(false);
    useEffect(() => { void get("/admin/ai").then(setData).catch(setError); }, []);
    function change(k, v) { if (data)
        setData({ ...data, [k]: v }); setSaved(false); }
    return h("section", { class: "music-panel" },
        h("h2", null, "\u0418\u0441\u043A\u0443\u0441\u0441\u0442\u0432\u0435\u043D\u043D\u044B\u0439 \u0438\u043D\u0442\u0435\u043B\u043B\u0435\u043A\u0442"),
        h(ErrorLine, { error: error }),
        data && h("form", { onSubmit: async (e) => { e.preventDefault(); if (busy)
                return; setBusy(true); setError(null); setSaved(false); try {
                await put("/admin/ai", { ...data, api_key: key, clear_key: clear });
                setKey("");
                setClear(false);
                setData(await get("/admin/ai"));
                setSaved(true);
            }
            catch (e) {
                setError(e);
            }
            finally {
                setBusy(false);
            } } },
            h("fieldset", { disabled: busy, style: "display:grid;gap:14px;border:0;padding:0;min-width:0" },
                h("label", null,
                    h("input", { type: "checkbox", checked: data.enabled, onChange: e => change("enabled", e.currentTarget.checked) }),
                    " \u0412\u043A\u043B\u044E\u0447\u0438\u0442\u044C \u0418\u0418"),
                h("label", null,
                    h("input", { type: "checkbox", checked: data.allow_users, onChange: e => change("allow_users", e.currentTarget.checked) }),
                    " \u0420\u0430\u0437\u0440\u0435\u0448\u0438\u0442\u044C \u0432\u0441\u0435\u043C \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044F\u043C (\u0438\u043D\u0430\u0447\u0435 \u0442\u043E\u043B\u044C\u043A\u043E \u0430\u0434\u043C\u0438\u043D\u0438\u0441\u0442\u0440\u0430\u0442\u043E\u0440\u0430\u043C)"),
                h("label", null,
                    "\u041F\u0440\u043E\u0432\u0430\u0439\u0434\u0435\u0440",
                    h("select", { value: data.base_url === "https://api.openai.com/v1" ? "openai" : "custom", onChange: e => change("base_url", e.currentTarget.value === "openai" ? "https://api.openai.com/v1" : "") },
                        h("option", { value: "openai" }, "OpenAI"),
                        h("option", { value: "custom" }, "\u0421\u043E\u0432\u043C\u0435\u0441\u0442\u0438\u043C\u044B\u0439 \u0441 OpenAI API"))),
                h("label", null,
                    "\u0410\u0434\u0440\u0435\u0441 API (HTTPS)",
                    h("input", { type: "url", required: true, maxLength: 512, value: data.base_url, onInput: e => change("base_url", e.currentTarget.value) })),
                h("label", null,
                    "\u041C\u043E\u0434\u0435\u043B\u044C \u0442\u0435\u043A\u0441\u0442\u0430",
                    h("input", { maxLength: 128, value: data.text_model, placeholder: "\u0418\u0434\u0435\u043D\u0442\u0438\u0444\u0438\u043A\u0430\u0442\u043E\u0440 \u043C\u043E\u0434\u0435\u043B\u0438 \u0443 \u043F\u0440\u043E\u0432\u0430\u0439\u0434\u0435\u0440\u0430", onInput: e => change("text_model", e.currentTarget.value) })),
                h("label", null,
                    "\u041C\u043E\u0434\u0435\u043B\u044C \u0438\u0437\u043E\u0431\u0440\u0430\u0436\u0435\u043D\u0438\u0439",
                    h("input", { maxLength: 128, value: data.image_model, placeholder: "\u041E\u0441\u0442\u0430\u0432\u044C\u0442\u0435 \u043F\u0443\u0441\u0442\u044B\u043C, \u0447\u0442\u043E\u0431\u044B \u043E\u0442\u043A\u043B\u044E\u0447\u0438\u0442\u044C \u0433\u0435\u043D\u0435\u0440\u0430\u0446\u0438\u044E \u0441\u0442\u0438\u043A\u0435\u0440\u043E\u0432", onInput: e => change("image_model", e.currentTarget.value) })),
                h("label", null,
                    "API-\u043A\u043B\u044E\u0447",
                    h("input", { type: "password", autoComplete: "new-password", maxLength: 4096, disabled: clear, value: key, placeholder: data.key_set ? "Сохранён — пустое поле не меняет ключ" : "Введите ключ провайдера", onInput: e => { setKey(e.currentTarget.value); setSaved(false); } })),
                h("label", null,
                    h("input", { type: "checkbox", checked: clear, onChange: e => { setClear(e.currentTarget.checked); setSaved(false); } }),
                    " \u0423\u0434\u0430\u043B\u0438\u0442\u044C \u043A\u043B\u044E\u0447"),
                h("p", { class: "muted" }, "\u041B\u0438\u043C\u0438\u0442 \u043D\u0430 \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044F \u0432 \u0441\u0443\u0442\u043A\u0438: 30 \u0437\u0430\u043F\u0440\u043E\u0441\u043E\u0432 \u0442\u0435\u043A\u0441\u0442\u0430 \u0438 5 \u0438\u0437\u043E\u0431\u0440\u0430\u0436\u0435\u043D\u0438\u0439, \u0432\u043A\u043B\u044E\u0447\u0430\u044F \u043D\u0435\u0443\u0434\u0430\u0447\u043D\u044B\u0435 \u043F\u043E\u043F\u044B\u0442\u043A\u0438. \u041F\u0440\u043E\u0432\u0430\u0439\u0434\u0435\u0440 \u043C\u043E\u0436\u0435\u0442 \u0441\u043F\u0438\u0441\u044B\u0432\u0430\u0442\u044C \u043E\u043F\u043B\u0430\u0442\u0443 \u0437\u0430 \u0437\u0430\u043F\u0440\u043E\u0441\u044B. \u041E\u0442\u043F\u0440\u0430\u0432\u043B\u044F\u0435\u0442\u0441\u044F \u0442\u043E\u043B\u044C\u043A\u043E \u0442\u0435\u043A\u0441\u0442, \u0432\u044B\u0431\u0440\u0430\u043D\u043D\u044B\u0439 \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u0435\u043C; \u0438\u0441\u0442\u043E\u0440\u0438\u044F \u0447\u0430\u0442\u0430 \u0430\u0432\u0442\u043E\u043C\u0430\u0442\u0438\u0447\u0435\u0441\u043A\u0438 \u043D\u0435 \u043F\u0435\u0440\u0435\u0434\u0430\u0451\u0442\u0441\u044F."),
                h("button", null, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C \u043D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u0418\u0418")),
            saved && h("p", { role: "status" }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0435\u043D\u043E. \u0417\u0430\u043F\u0440\u043E\u0441\u044B \u043F\u0440\u043E\u0432\u0430\u0439\u0434\u0435\u0440\u0443 \u043F\u0440\u0438 \u0441\u043E\u0445\u0440\u0430\u043D\u0435\u043D\u0438\u0438 \u043D\u0435 \u043E\u0442\u043F\u0440\u0430\u0432\u043B\u044F\u044E\u0442\u0441\u044F.")));
}
export function AIText({ text, apply }) {
    const [enabled, setEnabled] = useState(false);
    const [busy, setBusy] = useState(false);
    const [result, setResult] = useState("");
    const [error, setError] = useState(null);
    useEffect(() => { void get("/ai/status").then(x => setEnabled(x.text)).catch(() => { }); }, []);
    if (!enabled)
        return null;
    return h("aside", null,
        h("p", { class: "muted" }, "\u0418\u0418 \u043F\u043E\u043B\u0443\u0447\u0438\u0442 \u0442\u043E\u043B\u044C\u043A\u043E \u0442\u0435\u043A\u0441\u0442 \u0432\u0430\u0448\u0435\u0433\u043E \u0447\u0435\u0440\u043D\u043E\u0432\u0438\u043A\u0430."),
        h("button", { type: "button", disabled: busy || !text.trim(), onClick: async () => { setBusy(true); setError(null); setResult(""); try {
                setResult((await post("/ai/generate", { kind: "text", prompt: text })).text);
            }
            catch (e) {
                setError(e);
            }
            finally {
                setBusy(false);
            } } }, busy ? "ИИ работает…" : "Улучшить текст с ИИ"),
        h(ErrorLine, { error: error }),
        result && h("div", null,
            h("p", { style: "white-space:pre-wrap" }, result),
            h("button", { type: "button", onClick: () => { apply(result); setResult(""); } }, "\u0412\u0441\u0442\u0430\u0432\u0438\u0442\u044C \u0432 \u0447\u0435\u0440\u043D\u043E\u0432\u0438\u043A"),
            h("button", { type: "button", onClick: () => setResult("") }, "\u041E\u0442\u043C\u0435\u043D\u0430")));
}
export function AISticker({ created }) {
    const [enabled, setEnabled] = useState(false);
    const [prompt, setPrompt] = useState("");
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    useEffect(() => { void get("/ai/status").then(x => setEnabled(x.images)).catch(() => { }); }, []);
    if (!enabled)
        return null;
    return h("div", null,
        h("label", null,
            "\u0421\u0442\u0438\u043A\u0435\u0440 \u0441 \u0418\u0418: \u043E\u043F\u0438\u0448\u0438\u0442\u0435 \u043A\u0430\u0440\u0442\u0438\u043D\u043A\u0443",
            h("textarea", { maxLength: 1000, value: prompt, onInput: e => setPrompt(e.currentTarget.value) })),
        h("p", { class: "muted" }, "\u041E\u043F\u0438\u0441\u0430\u043D\u0438\u0435 \u0431\u0443\u0434\u0435\u0442 \u043E\u0442\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u043E \u043F\u0440\u043E\u0432\u0430\u0439\u0434\u0435\u0440\u0443 \u0418\u0418. \u0413\u043E\u0442\u043E\u0432\u044B\u0439 \u0441\u0442\u0438\u043A\u0435\u0440 \u043D\u0435 \u043E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u0441\u044F \u0432 \u0447\u0430\u0442 \u0430\u0432\u0442\u043E\u043C\u0430\u0442\u0438\u0447\u0435\u0441\u043A\u0438."),
        h("button", { type: "button", disabled: busy || !prompt.trim(), onClick: async () => { setBusy(true); setError(null); try {
                created(await post("/ai/generate", { kind: "image", prompt }));
                setPrompt("");
            }
            catch (e) {
                setError(e);
            }
            finally {
                setBusy(false);
            } } }, busy ? "Создаём картинку…" : "Создать с ИИ"),
        h(ErrorLine, { error: error }));
}
