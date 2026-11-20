import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { get } from "./api.js";
import { ErrorLine } from "./ui.js";
export function CommentArchivePage() {
    const [busy, setBusy] = useState(false), [count, setCount] = useState(0), [done, setDone] = useState(false), [error, setError] = useState(null);
    const cancel = useRef(false);
    useEffect(() => () => { cancel.current = true; }, []);
    async function download() {
        cancel.current = false;
        setBusy(true);
        setCount(0);
        setDone(false);
        setError(null);
        try {
            const parts = ["\uFEFFID;Post ID;Date UTC;Comment\r\n"];
            let after = 0, total = 0;
            const cell = (s) => '"' + String(s).replace(/^[=+@\-\t\r]/, "'$&").replace(/"/g, '""') + '"';
            do {
                const page = await get(`/users/me/comments?after=${after}`);
                if (cancel.current)
                    return;
                parts.push(page.items.map(c => [c.id, c.post_id, c.created_at, c.text].map(cell).join(";")).join("\r\n") + (page.items.length ? "\r\n" : ""));
                total += page.items.length;
                setCount(total);
                after = page.next_cursor || 0;
            } while (after);
            const url = URL.createObjectURL(new Blob(parts, { type: "text/csv;charset=utf-8" }));
            const a = document.createElement("a");
            a.href = url;
            a.download = "socialnet-my-comments.csv";
            a.click();
            setTimeout(() => URL.revokeObjectURL(url), 1000);
            setDone(true);
        }
        catch (e) {
            if (!cancel.current)
                setError(e);
        }
        finally {
            setBusy(false);
        }
    }
    return h("article", { class: "library-panel info-page" },
        h("a", { href: "/help" }, "\u2190 \u041F\u043E\u043C\u043E\u0449\u044C"),
        h("h1", null, "\u041A\u0430\u043A \u043D\u0430\u0439\u0442\u0438 \u0432\u0441\u0435 \u043C\u043E\u0438 \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438?"),
        h("p", null, "\u0421\u043A\u0430\u0447\u0430\u0439\u0442\u0435 \u0441\u043E\u0445\u0440\u0430\u043D\u0451\u043D\u043D\u044B\u0435 \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438 \u0441\u0432\u043E\u0435\u0433\u043E \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430 socialnet \u0432 CSV. \u0424\u0430\u0439\u043B \u043E\u0442\u043A\u0440\u044B\u0432\u0430\u0435\u0442\u0441\u044F \u0432 Excel \u0438 \u0434\u0440\u0443\u0433\u0438\u0445 \u0442\u0430\u0431\u043B\u0438\u0447\u043D\u044B\u0445 \u0440\u0435\u0434\u0430\u043A\u0442\u043E\u0440\u0430\u0445."),
        h("ol", null,
            h("li", null, "\u041D\u0430\u0436\u043C\u0438\u0442\u0435 \u00AB\u0421\u043A\u0430\u0447\u0430\u0442\u044C \u043C\u043E\u0438 \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438\u00BB."),
            h("li", null, "\u0414\u043E\u0436\u0434\u0438\u0442\u0435\u0441\u044C \u043E\u043A\u043E\u043D\u0447\u0430\u043D\u0438\u044F \u0432\u044B\u0433\u0440\u0443\u0437\u043A\u0438, \u043D\u0435 \u0437\u0430\u043A\u0440\u044B\u0432\u0430\u044F \u0441\u0442\u0440\u0430\u043D\u0438\u0446\u0443."),
            h("li", null, "\u041E\u0442\u043A\u0440\u043E\u0439\u0442\u0435 \u0444\u0430\u0439\u043B \u0438 \u043D\u0430\u0439\u0434\u0438\u0442\u0435 \u043D\u0443\u0436\u043D\u0443\u044E \u0444\u0440\u0430\u0437\u0443 \u043F\u043E\u0438\u0441\u043A\u043E\u043C.")),
        h("p", { class: "muted" }, "\u0412 \u0444\u0430\u0439\u043B \u0432\u0445\u043E\u0434\u044F\u0442 \u0442\u0435\u043A\u0441\u0442, \u0434\u0430\u0442\u0430, ID \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u044F \u0438 \u0437\u0430\u043F\u0438\u0441\u0438. \u0423\u0434\u0430\u043B\u0451\u043D\u043D\u044B\u0435 \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438 \u043D\u0435 \u0432\u043E\u0441\u0441\u0442\u0430\u043D\u0430\u0432\u043B\u0438\u0432\u0430\u044E\u0442\u0441\u044F. \u042D\u0442\u043E \u0432\u044B\u0433\u0440\u0443\u0437\u043A\u0430 \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0435\u0432, \u0430 \u043D\u0435 \u043F\u043E\u043B\u043D\u044B\u0439 \u0430\u0440\u0445\u0438\u0432 \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430."),
        h(ErrorLine, { error: error }),
        h("button", { class: "primary", disabled: busy, onClick: download }, "\u0421\u043A\u0430\u0447\u0430\u0442\u044C \u043C\u043E\u0438 \u043A\u043E\u043C\u043C\u0435\u043D\u0442\u0430\u0440\u0438\u0438"),
        busy && h("button", { class: "ghost", onClick: () => { cancel.current = true; } }, "\u041E\u0442\u043C\u0435\u043D\u0438\u0442\u044C"),
        h("p", { role: "status" }, busy ? `Загружено комментариев: ${count}` : done ? `Файл подготовлен. Комментариев: ${count}` : ""));
}
