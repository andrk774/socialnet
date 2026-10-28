// QR-код похода: картинка для объявления или чата, скачивание SVG и
// статистика сканов для организатора.
import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { fetchBlob, get, saveFile } from "./api.js";
import { ErrorLine } from "./ui.js";
export function HikeQR({ id, title, organizer }) {
    const [open, setOpen] = useState(false);
    const [src, setSrc] = useState(null);
    const [stats, setStats] = useState(null);
    const [error, setError] = useState(null);
    useEffect(() => {
        if (!open)
            return;
        // SVG — картинкой через data:, без вставки разметки в страницу
        fetchBlob(`/api/v1/events/${id}/qr.svg`).then((b) => b.text())
            .then((svg) => setSrc("data:image/svg+xml;charset=utf-8," + encodeURIComponent(svg)), setError);
        if (organizer)
            get(`/events/${id}/qr-stats`).then(setStats, setError);
    }, [open, id, organizer]);
    return (h("details", { class: "hike-qr", onToggle: (e) => setOpen(e.target.open) },
        h("summary", null, "QR-\u043A\u043E\u0434 \u043F\u043E\u0445\u043E\u0434\u0430"),
        h("p", { class: "muted" }, "\u0414\u043B\u044F \u043E\u0431\u044A\u044F\u0432\u043B\u0435\u043D\u0438\u044F \u0432 \u043A\u043B\u0443\u0431\u0435, \u043D\u0430 \u0432\u043E\u043A\u0437\u0430\u043B\u0435 \u0438\u043B\u0438 \u0432 \u0447\u0430\u0442\u0435: \u043A\u0430\u043C\u0435\u0440\u0430 \u0442\u0435\u043B\u0435\u0444\u043E\u043D\u0430 \u043E\u0442\u043A\u0440\u043E\u0435\u0442 \u0441\u0442\u0440\u0430\u043D\u0438\u0446\u0443 \u043F\u043E\u0445\u043E\u0434\u0430."),
        src && h("img", { src: src, width: 200, height: 200, alt: `QR-код: ${title}` }),
        h("div", { class: "row" },
            h("button", { class: "ghost", onClick: () => saveFile(`/api/v1/events/${id}/qr.svg`, `qr-${title}.svg`).catch(setError) }, "\u0421\u043A\u0430\u0447\u0430\u0442\u044C SVG"),
            h("a", { class: "ghost button-link", href: `/events/${id}/print` }, "\u041B\u0438\u0441\u0442 \u0434\u043B\u044F \u043F\u0435\u0447\u0430\u0442\u0438")),
        stats && (h("p", { class: "qr-stats", "aria-live": "polite" },
            "\u041E\u0442\u0441\u043A\u0430\u043D\u0438\u0440\u043E\u0432\u0430\u043B\u0438: ",
            h("strong", { "data-stat": "scans" }, stats.scans),
            stats.scans_week > 0 ? ` (за неделю ${stats.scans_week})` : "",
            " · ",
            "\u0437\u0430\u043F\u0438\u0441\u0430\u043B\u0438\u0441\u044C \u043F\u043E\u0441\u043B\u0435 \u0441\u043A\u0430\u043D\u0430: ",
            h("strong", { "data-stat": "joins" }, stats.joins),
            stats.scans > 0 ? ` · ${Math.round((stats.joins / stats.scans) * 100)}%` : "")),
        h(ErrorLine, { error: error })));
}
