// Офлайн: сохранённые маршруты с картой — открываются без сети и без входа
// (данные лежат в браузере этого устройства).
import { Fragment, h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { removeOffline, savedRoutes } from "./offline.js";
import { coords } from "./print.js";
import { ElevationProfile, km, RouteMap } from "./routes.js";
import { Empty, Loading } from "./ui.js";
export function OfflinePage() {
    const [list, setList] = useState(null);
    const [open, setOpen] = useState(null);
    const load = () => savedRoutes().then(setList, () => setList([]));
    useEffect(() => { load(); }, []);
    if (!list)
        return h(Loading, null);
    return (h("section", { class: "offline" },
        h("h1", null, "\u041C\u0430\u0440\u0448\u0440\u0443\u0442\u044B \u043E\u0444\u043B\u0430\u0439\u043D"),
        !navigator.onLine && h("p", { class: "note" }, "\u041D\u0435\u0442 \u0441\u0432\u044F\u0437\u0438. \u0417\u0434\u0435\u0441\u044C \u2014 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u044B, \u0441\u043E\u0445\u0440\u0430\u043D\u0451\u043D\u043D\u044B\u0435 \u043D\u0430 \u044D\u0442\u043E\u043C \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0435."),
        list.length === 0 ? h(Empty, null, "\u0421\u043E\u0445\u0440\u0430\u043D\u0451\u043D\u043D\u044B\u0445 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u043E\u0432 \u043D\u0435\u0442. \u041E\u0442\u043A\u0440\u043E\u0439\u0442\u0435 \u043C\u0430\u0440\u0448\u0440\u0443\u0442 \u043F\u0440\u0438 \u0441\u0432\u044F\u0437\u0438 \u0438 \u043D\u0430\u0436\u043C\u0438\u0442\u0435 \u00AB\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C \u043E\u0444\u043B\u0430\u0439\u043D\u00BB.") : (h("ul", { class: "ads" }, list.map(({ route: r, saved_at }) => (h("li", { key: r.id, class: "ad" },
            h("div", { class: "route-head" },
                h("strong", null, r.title),
                h("span", { class: "route-actions" },
                    h("button", { class: "link", onClick: () => setOpen(open === r.id ? null : r.id) }, open === r.id ? "Скрыть" : "Открыть"),
                    h("button", { class: "link", onClick: () => removeOffline(r.id).then(load) }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C \u0441 \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0430"))),
            h("p", { class: "muted" },
                km(r.distance_m),
                " \u00B7 \u2191 ",
                r.gain_m,
                " \u043C \u00B7 \u0441\u043E\u0445\u0440\u0430\u043D\u0451\u043D ",
                new Date(saved_at).toLocaleDateString("ru")),
            open === r.id && h(Fragment, null,
                h(RouteMap, { route: r }),
                r.profile.length > 1 && h(ElevationProfile, { profile: r.profile }),
                (r.waypoints ?? []).length > 0 && h("ul", { class: "plain-list" }, r.waypoints.map((w, i) => h("li", { key: i },
                    h("strong", null, w.name || `Точка ${i + 1}`),
                    " \u2014 ",
                    coords(w.lat, w.lon)))))))))),
        h("p", { class: "muted" }, "\u042D\u043A\u0441\u0442\u0440\u0435\u043D\u043D\u0430\u044F \u0441\u0432\u044F\u0437\u044C \u2014 112. \u0421\u043E\u0445\u0440\u0430\u043D\u0451\u043D\u043D\u044B\u0435 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u044B \u0441\u0442\u0438\u0440\u0430\u044E\u0442\u0441\u044F \u043F\u0440\u0438 \u0432\u044B\u0445\u043E\u0434\u0435 \u0438\u0437 \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430.")));
}
