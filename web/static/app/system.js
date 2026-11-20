import { SiteSettingsPanel } from "./site-settings.js";
import { AIPanel } from "./ai.js";
import { SMTPPanel } from "./smtp.js";
import { TURNPanel } from "./turn.js";
import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get } from "./api.js";
import { ErrorLine, Loading } from "./ui.js";
import { KubernetesPanel } from "./kubernetes.js";
export function SystemTab() {
    const [data, setData] = useState();
    const [error, setError] = useState(null);
    const [history, setHistory] = useState([]);
    const [refresh, setRefresh] = useState(0);
    useEffect(() => {
        let live = true;
        let timer;
        const poll = async () => {
            try {
                const next = await get("/admin/system");
                if (live) {
                    setData(next);
                    setError(null);
                    setHistory((old) => [...(old.at(-1)?.started_at === next.started_at ? old : []), next].slice(-60));
                }
            }
            catch (e) {
                if (live)
                    setError(e);
            }
            finally {
                if (live)
                    timer = setTimeout(poll, 10000);
            }
        };
        void poll();
        return () => { live = false; clearTimeout(timer); };
    }, [refresh]);
    if (!data)
        return h(Fragment, null,
            h(ErrorLine, { error: error }),
            h(Loading, null));
    const mib = (v) => (v / 1048576).toFixed(1) + " МБ";
    const prev = history.at(-2);
    const seconds = prev ? (Date.parse(data.at) - Date.parse(prev.at)) / 1000 : 0;
    const rate = prev && seconds > 0 ? Math.max(0, (data.requests - prev.requests) / seconds).toFixed(1) : "—";
    const tiles = [["PostgreSQL", data.database_ok ? "Доступен" : "Ошибка"], ["Redis", data.redis === "ok" ? "Доступен" : data.redis === "disabled" ? "Не настроен" : "Ошибка"], ["Время работы API", `${Math.floor(data.uptime_seconds / 60)} мин`], ["Память объектов Go", mib(data.heap_bytes)], ["Память среды Go", mib(data.runtime_bytes)], ["Горутины", data.goroutines], ["WebSocket-соединения", data.websockets], ["Запросов с запуска", data.requests], ["Ошибок HTTP 5xx с запуска", data.errors_5xx], ["Запросов в секунду", rate], ["Среднее время с запуска", `${data.mean_latency_ms.toFixed(1)} мс`], ["Соединения БД: занято / максимум", `${data.pool.acquired} / ${data.pool.max}`]];
    const max = Math.max(1, ...history.map((x) => x.heap_bytes));
    const points = history.map((x, i) => `${i * 600 / Math.max(1, history.length - 1)},${110 - x.heap_bytes / max * 95}`).join(" ");
    return h("section", { class: "system-monitor" },
        h("div", { class: "row" },
            h("button", { class: "ghost", onClick: () => setRefresh((n) => n + 1) }, "\u041E\u0431\u043D\u043E\u0432\u0438\u0442\u044C"),
            h("span", { class: "muted" },
                "\u041E\u0431\u043D\u043E\u0432\u043B\u0435\u043D\u0438\u0435 \u043A\u0430\u0436\u0434\u044B\u0435 10 \u0441\u0435\u043A\u0443\u043D\u0434 \u00B7 ",
                new Date(data.at).toLocaleTimeString("ru"))),
        h(ErrorLine, { error: error }),
        error && h("p", { class: "error" }, "\u041F\u043E\u043A\u0430\u0437\u0430\u043D\u044B \u043F\u043E\u0441\u043B\u0435\u0434\u043D\u0438\u0435 \u043F\u043E\u043B\u0443\u0447\u0435\u043D\u043D\u044B\u0435 \u0434\u0430\u043D\u043D\u044B\u0435."),
        h("p", { class: "muted" }, "\u041F\u043E\u043A\u0430\u0437\u0430\u0442\u0435\u043B\u0438 \u0442\u0435\u043A\u0443\u0449\u0435\u0433\u043E \u043F\u0440\u043E\u0446\u0435\u0441\u0441\u0430 API. \u0421\u0447\u0451\u0442\u0447\u0438\u043A\u0438 \u043E\u0431\u043D\u0443\u043B\u044F\u044E\u0442\u0441\u044F \u043F\u0440\u0438 \u0435\u0433\u043E \u043F\u0435\u0440\u0435\u0437\u0430\u043F\u0443\u0441\u043A\u0435."),
        h("ul", { class: "tiles" }, tiles.map(([name, value]) => h("li", { class: "tile", key: name },
            h("strong", { class: "tile-value" }, value),
            h("span", null, name)))),
        h("div", { class: "music-panel" },
            h("h2", null, "\u041F\u0430\u043C\u044F\u0442\u044C \u043E\u0431\u044A\u0435\u043A\u0442\u043E\u0432 Go"),
            h("svg", { viewBox: "0 0 600 120", role: "img", "aria-label": "\u0418\u0437\u043C\u0435\u043D\u0435\u043D\u0438\u0435 \u043F\u0430\u043C\u044F\u0442\u0438 \u043E\u0431\u044A\u0435\u043A\u0442\u043E\u0432 Go \u0437\u0430 \u0432\u0440\u0435\u043C\u044F \u043D\u0430\u0431\u043B\u044E\u0434\u0435\u043D\u0438\u044F" },
                h("polyline", { points: points, fill: "none", stroke: "currentColor", "stroke-width": "2" })),
            h("p", { class: "muted" },
                "\u0418\u0441\u0442\u043E\u0440\u0438\u044F \u043F\u043E\u0441\u043B\u0435\u0434\u043D\u0438\u0445 60 \u0438\u0437\u043C\u0435\u0440\u0435\u043D\u0438\u0439 \u0432 \u044D\u0442\u043E\u0439 \u0432\u043A\u043B\u0430\u0434\u043A\u0435. \u0412\u0435\u0440\u0445\u043D\u044F\u044F \u0433\u0440\u0430\u043D\u0438\u0446\u0430: ",
                mib(max),
                ".")),
        h("div", { class: "music-panel" },
            h("h2", null, "\u041E\u0447\u0435\u0440\u0435\u0434\u0438"),
            data.queues ? h("dl", { class: "system-queues" },
                h("dt", null, "\u0417\u0430\u0434\u0430\u0447\u0438 \u043E\u0431\u0440\u0430\u0431\u043E\u0442\u043A\u0438"),
                h("dd", null, data.queues.jobs),
                h("dt", null, "\u0414\u043E\u0441\u0442\u0430\u0432\u043A\u0430 \u0441\u043E\u0431\u044B\u0442\u0438\u0439"),
                h("dd", null, data.queues.callbacks),
                h("dt", null, "\u041E\u0442\u043A\u0440\u044B\u0442\u044B\u0435 \u0442\u0440\u0435\u0432\u043E\u0433\u0438"),
                h("dd", null,
                    h("a", { href: "/admin?tab=alerts" }, data.queues.open_alerts))) : h("p", { class: "error" }, "\u041D\u0435 \u0443\u0434\u0430\u043B\u043E\u0441\u044C \u043F\u043E\u043B\u0443\u0447\u0438\u0442\u044C \u043E\u0447\u0435\u0440\u0435\u0434\u0438.")),
        h("div", { class: "music-panel" },
            h("h2", null, "\u0420\u0435\u0437\u0435\u0440\u0432\u043D\u043E\u0435 \u043A\u043E\u043F\u0438\u0440\u043E\u0432\u0430\u043D\u0438\u0435"),
            h("p", null, data.backup?.last_run_timestamp ? `Последний запуск: ${new Date(data.backup.last_run_timestamp * 1000).toLocaleString("ru")}` : "Нет данных о запусках."),
            h("p", null, data.backup?.last_success_timestamp ? `Последняя копия с успешной проверкой восстановления: ${new Date(data.backup.last_success_timestamp * 1000).toLocaleString("ru")}` : "Нет подтверждённой проверкой восстановления копии.")),
        h(SiteSettingsPanel, null),
        h(AIPanel, null),
        h(SMTPPanel, null),
        h(TURNPanel, null),
        h(KubernetesPanel, null));
}
