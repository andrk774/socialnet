import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get } from "./api.js";
import { ErrorLine } from "./ui.js";
export function KubernetesPanel() {
    const [data, setData] = useState(), [error, setError] = useState(null), [refresh, setRefresh] = useState(0);
    useEffect(() => { let alive = true; let timer; async function load() { try {
        const d = await get("/admin/kubernetes");
        if (alive) {
            setData(d);
            setError(null);
        }
    }
    catch (e) {
        if (alive)
            setError(e);
    }
    finally {
        if (alive)
            timer = setTimeout(load, 30000);
    } } void load(); return () => { alive = false; clearTimeout(timer); }; }, [refresh]);
    return h("section", { class: "music-panel" },
        h("div", { class: "admin-section-heading" },
            h("h2", null, "Kubernetes"),
            h("button", { class: "ghost", onClick: () => setRefresh(n => n + 1) }, "\u041E\u0431\u043D\u043E\u0432\u0438\u0442\u044C Kubernetes")),
        h(ErrorLine, { error: error }),
        error && data && h("p", { class: "error" }, "\u041F\u043E\u043A\u0430\u0437\u0430\u043D\u044B \u043F\u043E\u0441\u043B\u0435\u0434\u043D\u0438\u0435 \u043F\u043E\u043B\u0443\u0447\u0435\u043D\u043D\u044B\u0435 \u0434\u0430\u043D\u043D\u044B\u0435."),
        !data && !error && h("p", null, "\u041F\u0440\u043E\u0432\u0435\u0440\u043A\u0430 \u043F\u043E\u0434\u043A\u043B\u044E\u0447\u0435\u043D\u0438\u044F\u2026"),
        data && (!data.enabled ? h("p", { class: "muted" }, "\u041A\u043B\u0430\u0441\u0442\u0435\u0440 \u043D\u0435 \u043F\u043E\u0434\u043A\u043B\u044E\u0447\u0451\u043D. \u041F\u043E\u0441\u043B\u0435 \u043D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u043F\u043E\u0434\u043A\u043B\u044E\u0447\u0435\u043D\u0438\u044F \u0437\u0434\u0435\u0441\u044C \u043F\u043E\u044F\u0432\u0438\u0442\u0441\u044F \u0441\u043E\u0441\u0442\u043E\u044F\u043D\u0438\u0435 \u0440\u0435\u0441\u0443\u0440\u0441\u043E\u0432 socialnet.") : h(Fragment, null,
            h("p", { class: "muted" },
                "Namespace: ",
                data.namespace,
                " \u00B7 ",
                new Date(data.at).toLocaleTimeString("ru"),
                " \u00B7 \u0422\u043E\u043B\u044C\u043A\u043E \u043F\u0440\u043E\u0441\u043C\u043E\u0442\u0440 \u00B7 \u041E\u0431\u043D\u043E\u0432\u043B\u0435\u043D\u0438\u0435 \u043A\u0430\u0436\u0434\u044B\u0435 30 \u0441\u0435\u043A\u0443\u043D\u0434"),
            data.errors.map(e => h("p", { class: "error" }, e)),
            data.truncated && h("p", { class: "note" }, "\u041F\u043E\u043A\u0430\u0437\u0430\u043D\u043E \u043D\u0435 \u0431\u043E\u043B\u0435\u0435 200 \u0440\u0435\u0441\u0443\u0440\u0441\u043E\u0432 \u043A\u0430\u0436\u0434\u043E\u0433\u043E \u0432\u0438\u0434\u0430."),
            h("h3", null, "\u0420\u0430\u0437\u0432\u0451\u0440\u0442\u044B\u0432\u0430\u043D\u0438\u044F"),
            data.deployments.length ? data.deployments.map(d => h("p", null,
                h("strong", null, d.name),
                " \u00B7 \u0413\u043E\u0442\u043E\u0432\u043E ",
                d.ready,
                " / ",
                d.desired,
                " \u00B7 \u041E\u0431\u043D\u043E\u0432\u043B\u0435\u043D\u043E ",
                d.updated)) : h("p", { class: "muted" }, "\u041D\u0435\u0442 \u0434\u0430\u043D\u043D\u044B\u0445 \u043E \u0440\u0430\u0437\u0432\u0451\u0440\u0442\u044B\u0432\u0430\u043D\u0438\u044F\u0445."),
            h("h3", null, "\u041F\u043E\u0434\u044B"),
            h("div", { class: "admin-table-scroll" },
                h("table", { class: "tx" },
                    h("thead", null,
                        h("tr", null,
                            h("th", null, "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435"),
                            h("th", null, "\u0421\u043E\u0441\u0442\u043E\u044F\u043D\u0438\u0435"),
                            h("th", null, "\u0413\u043E\u0442\u043E\u0432\u043D\u043E\u0441\u0442\u044C"),
                            h("th", null, "\u041F\u0435\u0440\u0435\u0437\u0430\u043F\u0443\u0441\u043A\u0438"),
                            h("th", null, "\u0423\u0437\u0435\u043B"))),
                    h("tbody", null, data.pods.map(p => h("tr", null,
                        h("td", null, p.name),
                        h("td", null,
                            p.phase,
                            p.reasons.length > 0 && h("small", { class: "error" }, p.reasons.join(", "))),
                        h("td", null,
                            p.ready,
                            " / ",
                            p.containers),
                        h("td", null, p.restarts),
                        h("td", null, p.node || "Не назначен")))))),
            h("h3", null, "\u0421\u043E\u0431\u044B\u0442\u0438\u044F"),
            data.events.length ? h("ul", { class: "plain-list" }, data.events.slice().sort((a, b) => b.at.localeCompare(a.at)).slice(0, 30).map(e => h("li", null,
                h("strong", null,
                    e.type,
                    " \u00B7 ",
                    e.reason),
                h("p", null,
                    e.object,
                    " \u00B7 ",
                    e.count,
                    " \u0440\u0430\u0437",
                    e.at && ` · ${new Date(e.at).toLocaleString("ru")}`)))) : h("p", { class: "muted" }, "\u041D\u0435\u0442 \u0441\u043E\u0431\u044B\u0442\u0438\u0439."))));
}
