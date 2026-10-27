// Кнопка «Пожаловаться»: причина из готовых вариантов или своя, отправка в
// очередь модераторов (POST /reports). Жалоба анонимна для автора.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { post } from "./api.js";
import { ErrorLine } from "./ui.js";
const reasons = ["Спам или реклама", "Оскорбления", "Ложные или опасные сведения", "Чужие личные данные", "Незаконный контент"];
export function ReportButton({ target, cls = "link" }) {
    const [open, setOpen] = useState(false);
    const [reason, setReason] = useState(reasons[0]);
    const [other, setOther] = useState("");
    const [sent, setSent] = useState(false);
    const [error, setError] = useState(null);
    if (sent)
        return h("span", { class: "muted", role: "status" }, "\u0416\u0430\u043B\u043E\u0431\u0430 \u043E\u0442\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u0430 \u043C\u043E\u0434\u0435\u0440\u0430\u0442\u043E\u0440\u0430\u043C.");
    async function submit(e) {
        e.preventDefault();
        setError(null);
        const text = reason === "" ? other.trim() : reason;
        try {
            await post("/reports", "uuid" in target
                ? { target_type: target.type, target_uuid: target.uuid, reason: text }
                : { target_type: target.type, target_id: target.id, reason: text });
            setSent(true);
        }
        catch (err) {
            setError(err);
        }
    }
    if (!open)
        return h("button", { type: "button", class: cls, onClick: () => setOpen(true) }, "\u041F\u043E\u0436\u0430\u043B\u043E\u0432\u0430\u0442\u044C\u0441\u044F");
    return (h("form", { class: "report-form", onSubmit: submit },
        h("label", null,
            "\u041F\u0440\u0438\u0447\u0438\u043D\u0430",
            h("select", { value: reason, onChange: (e) => setReason(e.target.value) },
                reasons.map((r) => h("option", { key: r, value: r }, r)),
                h("option", { value: "" }, "\u0414\u0440\u0443\u0433\u043E\u0435\u2026"))),
        reason === "" && h("label", null,
            "\u041E\u043F\u0438\u0448\u0438\u0442\u0435",
            h("input", { required: true, maxLength: 1000, value: other, onInput: (e) => setOther(e.target.value) })),
        h("span", { class: "row tight" },
            h("button", { class: "primary" }, "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C \u0436\u0430\u043B\u043E\u0431\u0443"),
            h("button", { type: "button", class: "ghost", onClick: () => setOpen(false) }, "\u041E\u0442\u043C\u0435\u043D\u0430")),
        h(ErrorLine, { error: error })));
}
