import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, post, del } from "./api.js";
export function NearbyPage() {
    const [data, setData] = useState({ visible: false, people: [] });
    const [busy, setBusy] = useState(false), [error, setError] = useState("");
    async function refresh() { try {
        setData(await get("/nearby"));
    }
    catch {
        setError("Не удалось загрузить людей рядом.");
    } }
    useEffect(() => { void refresh(); const timer = setInterval(() => void refresh(), 60000); return () => clearInterval(timer); }, []);
    async function enable() {
        setBusy(true);
        setError("");
        try {
            if (!window.isSecureContext || !navigator.geolocation)
                throw new Error("Для геолокации откройте socialnet по HTTPS.");
            const fix = await new Promise((resolve, reject) => navigator.geolocation.getCurrentPosition(resolve, reject, { enableHighAccuracy: false, timeout: 15000, maximumAge: 60000 }));
            setData(await post("/nearby", { lat: fix.coords.latitude, lon: fix.coords.longitude, consent: true }));
        }
        catch (e) {
            setError(e instanceof Error ? e.message : "Разрешите доступ к местоположению и повторите попытку.");
        }
        finally {
            setBusy(false);
        }
    }
    async function hide() { setBusy(true); setError(""); try {
        await del("/nearby");
        setData({ visible: false, people: [] });
    }
    catch {
        setError("Не удалось скрыться. Повторите попытку.");
    }
    finally {
        setBusy(false);
    } }
    return h("section", null,
        h("h1", null, "\u041B\u044E\u0434\u0438 \u0440\u044F\u0434\u043E\u043C"),
        h("p", null, "\u041D\u0430\u0439\u0434\u0438\u0442\u0435 \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u0435\u0439 socialnet \u0432 \u043F\u0440\u0435\u0434\u0435\u043B\u0430\u0445 25 \u043A\u043C. \u041F\u043E\u0441\u043B\u0435 \u0441\u043E\u0433\u043B\u0430\u0441\u0438\u044F \u0432\u044B \u0432\u0438\u0434\u043D\u044B 15 \u043C\u0438\u043D\u0443\u0442. \u041C\u044B \u043F\u043E\u043A\u0430\u0437\u044B\u0432\u0430\u0435\u043C \u0442\u043E\u043B\u044C\u043A\u043E \u043F\u0440\u0438\u043C\u0435\u0440\u043D\u043E\u0435 \u0440\u0430\u0441\u0441\u0442\u043E\u044F\u043D\u0438\u0435, \u0431\u0435\u0437 \u0442\u043E\u0447\u043A\u0438 \u043D\u0430 \u043A\u0430\u0440\u0442\u0435."),
        h("p", null,
            h("strong", null, data.visible ? "Вы видны людям рядом" : "Вы скрыты")),
        h("button", { disabled: busy, onClick: enable }, busy ? "Подождите…" : data.visible ? "Обновить местоположение и видимость" : "Разрешить геолокацию и стать видимым"),
        data.visible && h("button", { disabled: busy, onClick: hide }, "\u0421\u043A\u0440\u044B\u0442\u044C\u0441\u044F"),
        error && h("p", { role: "alert" }, error),
        data.visible && !data.people.length && h("p", null, "\u041F\u043E\u043A\u0430 \u043D\u0438\u043A\u043E\u0433\u043E \u0440\u044F\u0434\u043E\u043C \u043D\u0435\u0442. \u0414\u0440\u0443\u0433\u043E\u043C\u0443 \u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u0442\u0435\u043B\u044E \u0442\u043E\u0436\u0435 \u043D\u0443\u0436\u043D\u043E \u0432\u043A\u043B\u044E\u0447\u0438\u0442\u044C \u0432\u0438\u0434\u0438\u043C\u043E\u0441\u0442\u044C."),
        data.people.map(p => h("article", { key: p.id },
            h("h3", null,
                h("a", { href: `/u/${encodeURIComponent(p.username)}` },
                    p.first_name,
                    " ",
                    p.last_name)),
            h("p", null,
                "\u041F\u0440\u0438\u043C\u0435\u0440\u043D\u043E ",
                p.distance),
            h("a", { href: `/im/u/${p.id}` }, "\u041D\u0430\u043F\u0438\u0441\u0430\u0442\u044C \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435"))),
        h("h2", null, "\u041C\u0435\u0441\u0441\u0435\u043D\u0434\u0436\u0435\u0440 \u043F\u043E Wi-Fi"),
        h("p", null, "\u041F\u043E\u0434\u043A\u043B\u044E\u0447\u0438\u0442\u0435\u0441\u044C \u043A \u043E\u0431\u0449\u0435\u0439 Wi-Fi-\u0441\u0435\u0442\u0438 \u0438 \u043E\u0442\u043A\u0440\u043E\u0439\u0442\u0435 \u043E\u0434\u0438\u043D \u0438 \u0442\u043E\u0442 \u0436\u0435 \u043B\u043E\u043A\u0430\u043B\u044C\u043D\u044B\u0439 \u0441\u0435\u0440\u0432\u0435\u0440 socialnet. \u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u044F \u043F\u0440\u043E\u0445\u043E\u0434\u044F\u0442 \u0447\u0435\u0440\u0435\u0437 \u044D\u0442\u043E\u0442 \u0441\u0435\u0440\u0432\u0435\u0440 \u0434\u0430\u0436\u0435 \u0431\u0435\u0437 \u0432\u043D\u0435\u0448\u043D\u0435\u0433\u043E \u0438\u043D\u0442\u0435\u0440\u043D\u0435\u0442\u0430. \u0421\u0435\u0440\u0432\u0435\u0440 \u0438 Wi-Fi \u0434\u043E\u043B\u0436\u043D\u044B \u043E\u0441\u0442\u0430\u0432\u0430\u0442\u044C\u0441\u044F \u0432\u043A\u043B\u044E\u0447\u0451\u043D\u043D\u044B\u043C\u0438."),
        h("a", { href: "/im" }, "\u041E\u0442\u043A\u0440\u044B\u0442\u044C \u043C\u0435\u0441\u0441\u0435\u043D\u0434\u0436\u0435\u0440"));
}
