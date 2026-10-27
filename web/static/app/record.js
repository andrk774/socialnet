// /record — запись трека телефоном: старт, пауза, финиш, сохранение как
// маршрут (тот же путь, что загрузка GPX) и публикация в ленте.
import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { post, upload } from "./api.js";
import { RouteCard, hours } from "./routes.js";
import { addFix, backgroundCapable, distance, elapsed, loadDraft, newDraft, pause, pointCount, positionSource, resume, saveDraft, toGPX, } from "./track.js";
import { ErrorLine } from "./ui.js";
const dayName = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long" });
const km = (m) => (m < 1000 ? `${Math.round(m)} м` : `${(m / 1000).toFixed(m < 10000 ? 2 : 1)} км`);
const clock = (ms) => {
    const s = Math.floor(ms / 1000);
    return `${Math.floor(s / 3600)}:${String(Math.floor(s / 60) % 60).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
};
export function RecordPage() {
    const [draft, setDraftState] = useState(() => loadDraft());
    const [recording, setRecording] = useState(false); // идёт ли приём точек в этой вкладке
    const [finishing, setFinishing] = useState(false);
    const [last, setLast] = useState(null);
    const [gpsError, setGpsError] = useState("");
    const [, tick] = useState(0);
    const [saved, setSaved] = useState(null);
    const draftRef = useRef(draft);
    const setDraft = (d) => { draftRef.current = d; saveDraft(d); setDraftState(d); };
    // приём точек, пока запись идёт и не на паузе
    useEffect(() => {
        if (!recording)
            return;
        const stop = positionSource()((f) => {
            setGpsError("");
            setLast(f);
            const cur = draftRef.current;
            if (cur) {
                const next = addFix(cur, f);
                if (next !== cur)
                    setDraft(next);
            }
        }, setGpsError);
        const timer = setInterval(() => tick((n) => n + 1), 1000);
        // экран не гаснет — иначе браузер перестаёт получать координаты
        let lock = null;
        const wl = navigator.wakeLock;
        const grab = () => { if (wl && document.visibilityState === "visible")
            wl.request("screen").then((l) => { lock = l; }, () => { }); };
        grab();
        document.addEventListener("visibilitychange", grab);
        return () => { stop(); clearInterval(timer); document.removeEventListener("visibilitychange", grab); lock?.release().catch(() => { }); };
    }, [recording]);
    const start = () => { setSaved(null); setDraft(newDraft()); setRecording(true); };
    const togglePause = () => { if (draft)
        setDraft(draft.paused ? resume(draft) : pause(draft)); };
    const continueDraft = () => { if (draft) {
        setDraft(resume(draft));
        setRecording(true);
    } };
    const finish = () => { if (draft)
        setDraft(pause(draft)); setRecording(false); setFinishing(true); };
    const discard = () => { if (confirm("Удалить запись без сохранения?")) {
        setDraft(null);
        setFinishing(false);
        setRecording(false);
    } };
    if (saved)
        return h(Saved, { route: saved, onNew: () => setSaved(null) });
    if (finishing && draft)
        return h(FinishForm, { draft: draft, onBack: () => { setFinishing(false); continueDraft(); }, onDiscard: discard, onSaved: (r) => { setDraft(null); setFinishing(false); setSaved(r); } });
    const dist = draft ? distance(draft.segments) : 0;
    return (h("section", { class: "record" },
        h("h1", null, "\u0417\u0430\u043F\u0438\u0441\u0430\u0442\u044C \u0442\u0440\u0435\u043A"),
        !draft && h(Fragment, null,
            h("p", null, "\u0422\u0435\u043B\u0435\u0444\u043E\u043D \u0437\u0430\u043F\u0438\u0448\u0435\u0442 \u043F\u0443\u0442\u044C \u043F\u043E GPS. \u041F\u043E\u0442\u043E\u043C \u0442\u0440\u0435\u043A \u0441\u0442\u0430\u043D\u0435\u0442 \u043C\u0430\u0440\u0448\u0440\u0443\u0442\u043E\u043C: \u0441 \u043F\u0440\u043E\u0444\u0438\u043B\u0435\u043C \u0432\u044B\u0441\u043E\u0442, \u0434\u043B\u044F \u043F\u043E\u0445\u043E\u0434\u0430, \u0441\u0442\u0430\u0442\u044C\u0438 \u0438\u043B\u0438 \u0437\u0430\u043F\u0438\u0441\u0438 \u0432 \u043B\u0435\u043D\u0442\u0435."),
            !backgroundCapable && h("p", { class: "note" }, "\u0414\u0435\u0440\u0436\u0438\u0442\u0435 \u044D\u043A\u0440\u0430\u043D \u0432\u043A\u043B\u044E\u0447\u0451\u043D\u043D\u044B\u043C \u0438 \u0441\u0442\u0440\u0430\u043D\u0438\u0446\u0443 \u043E\u0442\u043A\u0440\u044B\u0442\u043E\u0439: \u0431\u0440\u0430\u0443\u0437\u0435\u0440 \u043D\u0435 \u043F\u0438\u0448\u0435\u0442 \u0442\u0440\u0435\u043A \u0432 \u0444\u043E\u043D\u0435. \u0421 \u0432\u044B\u043A\u043B\u044E\u0447\u0435\u043D\u043D\u044B\u043C \u044D\u043A\u0440\u0430\u043D\u043E\u043C \u043F\u0438\u0448\u0435\u0442 \u043F\u0440\u0438\u043B\u043E\u0436\u0435\u043D\u0438\u0435 \u0434\u043B\u044F Android."),
            h("button", { class: "primary big", onClick: start }, "\u041D\u0430\u0447\u0430\u0442\u044C \u0437\u0430\u043F\u0438\u0441\u044C")),
        draft && !recording && h(Fragment, null,
            h("p", { class: "note" },
                "\u0415\u0441\u0442\u044C \u043D\u0435\u0437\u0430\u0432\u0435\u0440\u0448\u0451\u043D\u043D\u0430\u044F \u0437\u0430\u043F\u0438\u0441\u044C \u043E\u0442 ",
                dayName.format(new Date(draft.startedAt)),
                ": ",
                km(dist),
                ", \u0442\u043E\u0447\u0435\u043A ",
                pointCount(draft),
                "."),
            h("div", { class: "row" },
                h("button", { class: "primary", onClick: continueDraft }, "\u041F\u0440\u043E\u0434\u043E\u043B\u0436\u0438\u0442\u044C"),
                h("button", { class: "ghost", onClick: () => setFinishing(true), disabled: pointCount(draft) < 2 }, "\u0417\u0430\u0432\u0435\u0440\u0448\u0438\u0442\u044C"),
                h("button", { class: "ghost danger", onClick: discard }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C"))),
        draft && recording && h(Fragment, null,
            h("div", { class: draft.paused ? "rec-stats paused" : "rec-stats", "aria-live": "polite" },
                h("div", null,
                    h("span", { class: "rec-num", "data-stat": "distance" }, km(dist)),
                    h("span", { class: "muted" }, "\u043F\u0440\u043E\u0439\u0434\u0435\u043D\u043E")),
                h("div", null,
                    h("span", { class: "rec-num" }, clock(elapsed(draft))),
                    h("span", { class: "muted" }, "\u0432 \u043F\u0443\u0442\u0438")),
                h("div", null,
                    h("span", { class: "rec-num", "data-stat": "points" }, pointCount(draft)),
                    h("span", { class: "muted" }, "\u0442\u043E\u0447\u0435\u043A")),
                h("div", null,
                    h("span", { class: "rec-num" }, last ? `±${Math.round(last.acc)} м` : "…"),
                    h("span", { class: "muted" }, "\u0442\u043E\u0447\u043D\u043E\u0441\u0442\u044C"))),
            h("p", { class: "muted", role: "status" }, draft.paused ? "Пауза — точки не пишутся." : last ? (last.ele !== null ? `Высота ${Math.round(last.ele)} м.` : "Запись идёт.") : "Ищем спутники…"),
            gpsError && h("p", { class: "error", role: "alert" }, gpsError),
            h("div", { class: "row" },
                h("button", { class: "ghost big", onClick: togglePause }, draft.paused ? "Продолжить" : "Пауза"),
                h("button", { class: "primary big", onClick: finish, disabled: pointCount(draft) < 2 }, "\u0417\u0430\u0432\u0435\u0440\u0448\u0438\u0442\u044C")))));
}
function FinishForm({ draft, onBack, onDiscard, onSaved }) {
    const [name, setName] = useState(`Трек ${dayName.format(new Date(draft.startedAt))}`);
    const dist = distance(draft.segments);
    // скрыть можно только меньше половины пути — иначе от трека ничего не останется
    const fits = (m) => m === 0 || dist > 2 * m + 50;
    const [hideEnds, setHideEnds] = useState(fits(200) ? "200" : "0");
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    async function save(e) {
        e.preventDefault();
        setBusy(true);
        setError(null);
        try {
            const file = new File([toGPX(draft, name.trim() || "Трек")], "track.gpx", { type: "application/gpx+xml" });
            onSaved(await upload("/routes", file, { hidden_ends_m: hideEnds }));
        }
        catch (err) {
            setError(err);
        }
        finally {
            setBusy(false);
        }
    }
    return (h("form", { class: "stack record", onSubmit: save },
        h("h1", null, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C \u0442\u0440\u0435\u043A"),
        h("p", null,
            km(dist),
            " \u00B7 ",
            clock(elapsed(draft)),
            " \u00B7 \u0442\u043E\u0447\u0435\u043A ",
            pointCount(draft)),
        h("label", null,
            "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435",
            h("input", { required: true, maxLength: 200, value: name, onInput: (e) => setName(e.target.value) })),
        h("label", null,
            "\u041D\u0430\u0447\u0430\u043B\u043E \u0438 \u043A\u043E\u043D\u0435\u0446",
            h("select", { value: hideEnds, onChange: (e) => setHideEnds(e.target.value) }, [[0, "видны всем"], [200, "скрыть 200 м"], [500, "скрыть 500 м"], [1000, "скрыть 1 км"]].map(([m, label]) => h("option", { key: m, value: String(m), disabled: !fits(m) }, label)))),
        h("p", { class: "muted" },
            "\u0421\u043A\u0440\u044B\u0442\u044B\u0435 \u043A\u043E\u043D\u0446\u044B \u043D\u0435 \u0432\u0438\u0434\u043D\u044B \u0434\u0440\u0443\u0433\u0438\u043C, \u0447\u0442\u043E\u0431\u044B \u043F\u043E \u0442\u0440\u0435\u043A\u0443 \u043D\u0435\u043B\u044C\u0437\u044F \u0431\u044B\u043B\u043E \u043D\u0430\u0439\u0442\u0438 \u0432\u0430\u0448 \u0434\u043E\u043C.",
            !fits(200) && " Трек короткий — скрыть концы не получится."),
        h(ErrorLine, { error: error }),
        h("div", { class: "row" },
            h("button", { class: "primary", disabled: busy }, busy ? "Сохраняем…" : "Сохранить маршрут"),
            h("button", { type: "button", class: "ghost", onClick: onBack }, "\u041F\u0440\u043E\u0434\u043E\u043B\u0436\u0438\u0442\u044C \u0437\u0430\u043F\u0438\u0441\u044C"),
            h("button", { type: "button", class: "ghost danger", onClick: onDiscard }, "\u0423\u0434\u0430\u043B\u0438\u0442\u044C"))));
}
function Saved({ route, onNew }) {
    const [text, setText] = useState("");
    const [audience, setAudience] = useState("friends");
    const [posted, setPosted] = useState(null);
    const [error, setError] = useState(null);
    async function publish(e) {
        e.preventDefault();
        setError(null);
        try {
            setPosted((await post("/posts", { text, route_ids: [route.id], audience })).id);
        }
        catch (err) {
            setError(err);
        }
    }
    return (h("section", { class: "record" },
        h("h1", null, "\u041C\u0430\u0440\u0448\u0440\u0443\u0442 \u0441\u043E\u0445\u0440\u0430\u043D\u0451\u043D"),
        h("p", { class: "muted" },
            route.title,
            " \u00B7 \u0432 \u043F\u0443\u0442\u0438 ",
            hours(route.duration_sec)),
        h(RouteCard, { route: route }),
        posted ? h("p", null,
            "\u041E\u043F\u0443\u0431\u043B\u0438\u043A\u043E\u0432\u0430\u043D\u043E \u2014 ",
            h("a", { href: `/p/${posted}` }, "\u043E\u0442\u043A\u0440\u044B\u0442\u044C \u0437\u0430\u043F\u0438\u0441\u044C"),
            ".") : (h("form", { class: "stack", onSubmit: publish },
            h("label", null,
                "\u0420\u0430\u0441\u0441\u043A\u0430\u0437\u0430\u0442\u044C \u0432 \u043B\u0435\u043D\u0442\u0435",
                h("textarea", { rows: 3, maxLength: 5000, value: text, placeholder: "\u041A\u0430\u043A \u043F\u0440\u043E\u0448\u043B\u0438, \u0447\u0442\u043E \u0432\u0438\u0434\u0435\u043B\u0438", onInput: (e) => setText(e.target.value) })),
            h("label", null,
                "\u041A\u0442\u043E \u0432\u0438\u0434\u0438\u0442",
                h("select", { value: audience, onChange: (e) => setAudience(e.target.value) },
                    h("option", { value: "everyone" }, "\u0432\u0441\u0435"),
                    h("option", { value: "friends" }, "\u0434\u0440\u0443\u0437\u044C\u044F"),
                    h("option", { value: "only_me" }, "\u0442\u043E\u043B\u044C\u043A\u043E \u044F"))),
            h(ErrorLine, { error: error }),
            h("div", { class: "row" },
                h("button", { class: "primary" }, "\u041E\u043F\u0443\u0431\u043B\u0438\u043A\u043E\u0432\u0430\u0442\u044C"),
                h("button", { type: "button", class: "ghost", onClick: onNew }, "\u0417\u0430\u043F\u0438\u0441\u0430\u0442\u044C \u0435\u0449\u0451"))))));
}
