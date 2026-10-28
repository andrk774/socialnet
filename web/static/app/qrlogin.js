// Вход по QR-коду: на новом устройстве — код и ожидание, на телефоне, где
// вы уже вошли, — подтверждение с описанием устройства.
import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { ApiError, get, post, qrLoginPoll, qrLoginStart } from "./api.js";
import { navigate, query } from "./router.js";
import { ErrorLine, Loading } from "./ui.js";
// QRLoginBox — на странице входа: код обновляется сам, пока его не подтвердят.
export function QRLoginBox({ onCancel }) {
    const [start, setStart] = useState(null);
    const [left, setLeft] = useState(0);
    const [state, setState] = useState("wait");
    const [error, setError] = useState(null);
    const [round, setRound] = useState(0);
    useEffect(() => {
        let stop = false;
        setStart(null);
        setState("wait");
        setError(null);
        qrLoginStart().then((s) => {
            if (stop)
                return;
            setStart(s);
            const until = new Date(s.expires_at).getTime();
            const tick = async () => {
                if (stop)
                    return;
                setLeft(Math.max(0, Math.round((until - Date.now()) / 1000)));
                try {
                    if (await qrLoginPoll(s.id, s.poll_token))
                        return; // вошли: App переключится сам
                }
                catch (e) {
                    if (e instanceof ApiError && e.code === "qr_rejected") {
                        setState("rejected");
                        return;
                    }
                    if (e instanceof ApiError && e.code === "qr_expired") {
                        setState("expired");
                        return;
                    }
                    if (!(e instanceof ApiError && e.status === 429)) {
                        setError(e);
                        return;
                    }
                }
                setTimeout(tick, 2000);
            };
            tick();
        }, setError);
        return () => { stop = true; };
    }, [round]);
    const src = start ? "data:image/svg+xml;charset=utf-8," + encodeURIComponent(start.qr_svg) : "";
    return (h("section", { class: "qr-login", "aria-live": "polite" },
        h("h2", null, "\u0412\u0445\u043E\u0434 \u043F\u043E QR-\u043A\u043E\u0434\u0443"),
        state === "wait" && h("ol", { class: "muted" },
            h("li", null, "\u041E\u0442\u043A\u0440\u043E\u0439\u0442\u0435 \u043A\u0430\u043C\u0435\u0440\u0443 \u043D\u0430 \u0442\u0435\u043B\u0435\u0444\u043E\u043D\u0435, \u0433\u0434\u0435 \u0432\u044B \u0443\u0436\u0435 \u0432\u043E\u0448\u043B\u0438 \u0432 socialnet."),
            h("li", null, "\u041D\u0430\u0432\u0435\u0434\u0438\u0442\u0435 \u043D\u0430 \u043A\u043E\u0434 \u0438 \u043E\u0442\u043A\u0440\u043E\u0439\u0442\u0435 \u0441\u0441\u044B\u043B\u043A\u0443."),
            h("li", null, "\u041F\u0440\u043E\u0432\u0435\u0440\u044C\u0442\u0435 \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u043E \u0438 \u043D\u0430\u0436\u043C\u0438\u0442\u0435 \u00AB\u041F\u043E\u0434\u0442\u0432\u0435\u0440\u0434\u0438\u0442\u044C \u0432\u0445\u043E\u0434\u00BB.")),
        state === "wait" && (start ? h("img", { src: src, width: 220, height: 220, alt: "QR-\u043A\u043E\u0434 \u0434\u043B\u044F \u0432\u0445\u043E\u0434\u0430" }) : h(Loading, null)),
        state === "wait" && start && h("p", { class: "muted" },
            "\u041A\u043E\u0434 \u0434\u0435\u0439\u0441\u0442\u0432\u0443\u0435\u0442 \u0435\u0449\u0451 ",
            left,
            " \u0441."),
        state === "expired" && h("p", null, "\u041A\u043E\u0434 \u0443\u0441\u0442\u0430\u0440\u0435\u043B."),
        state === "rejected" && h("p", null, "\u0412\u0445\u043E\u0434 \u043E\u0442\u043A\u043B\u043E\u043D\u0451\u043D \u043D\u0430 \u0442\u0435\u043B\u0435\u0444\u043E\u043D\u0435."),
        h(ErrorLine, { error: error }),
        h("div", { class: "row" },
            state !== "wait" && h("button", { class: "primary", onClick: () => setRound(round + 1) }, "\u041D\u043E\u0432\u044B\u0439 \u043A\u043E\u0434"),
            h("button", { class: "ghost", onClick: onCancel }, "\u0412\u0445\u043E\u0434 \u043F\u043E \u043F\u0430\u0440\u043E\u043B\u044E"))));
}
// describeUA — коротко: браузер и система, без разбора всех вариантов.
export function describeUA(ua) {
    const browser = /YaBrowser/.test(ua) ? "Яндекс Браузер" : /Edg\//.test(ua) ? "Edge" : /OPR\//.test(ua) ? "Opera" :
        /Firefox\//.test(ua) ? "Firefox" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "Браузер";
    const os = /Windows/.test(ua) ? "Windows" : /Android/.test(ua) ? "Android" : /iPhone|iPad/.test(ua) ? "iOS" :
        /Mac OS X/.test(ua) ? "macOS" : /Linux/.test(ua) ? "Linux" : "неизвестная система";
    return `${browser}, ${os}`;
}
// QRApprovePage — /qr-login?c=… на телефоне.
export function QRApprovePage() {
    const id = query("c") ?? "";
    const [info, setInfo] = useState(null);
    const [done, setDone] = useState(null);
    const [error, setError] = useState(null);
    useEffect(() => { get(`/qr-login/${encodeURIComponent(id)}`).then(setInfo, setError); }, [id]);
    const decide = (approve) => post(`/qr-login/${encodeURIComponent(id)}/${approve ? "approve" : "reject"}`).then(() => setDone(approve ? "approved" : "rejected"), setError);
    if (done)
        return (h("section", { class: "qr-approve" },
            h("h1", null, done === "approved" ? "Вход подтверждён" : "Вход отклонён"),
            h("p", null, done === "approved" ? "На том устройстве откроется ваш аккаунт. Завершить сеанс можно в разделе «Безопасность»." : "Устройство не получило доступ."),
            h("button", { class: "ghost", onClick: () => navigate("/") }, "\u0412 \u043B\u0435\u043D\u0442\u0443")));
    if (error)
        return (h("section", { class: "qr-approve" },
            h("h1", null, "\u041A\u043E\u0434 \u043D\u0435 \u0434\u0435\u0439\u0441\u0442\u0432\u0443\u0435\u0442"),
            h("p", null, error instanceof ApiError && error.code === "qr_expired" ? "Код устарел или уже использован. Обновите его на том устройстве и отсканируйте снова." : ""),
            h(ErrorLine, { error: error instanceof ApiError && error.code === "qr_expired" ? null : error })));
    if (!info)
        return h(Loading, null);
    return (h("section", { class: "qr-approve" },
        h("h1", null, "\u0412\u043E\u0439\u0442\u0438 \u043D\u0430 \u0434\u0440\u0443\u0433\u043E\u043C \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0435?"),
        h("dl", { class: "facts" },
            h("div", null,
                h("dt", null, "\u0423\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u043E"),
                h("dd", null, describeUA(info.user_agent))),
            info.ip && h("div", null,
                h("dt", null, "IP-\u0430\u0434\u0440\u0435\u0441"),
                h("dd", null, info.ip))),
        h("p", { class: "warn", role: "note" }, "\u041F\u043E\u0434\u0442\u0432\u0435\u0440\u0436\u0434\u0430\u0439\u0442\u0435, \u0442\u043E\u043B\u044C\u043A\u043E \u0435\u0441\u043B\u0438 \u044D\u0442\u043E\u0442 \u043A\u043E\u0434 \u0441\u0435\u0439\u0447\u0430\u0441 \u043F\u0435\u0440\u0435\u0434 \u0432\u0430\u043C\u0438 \u2014 \u043D\u0430 \u0432\u0430\u0448\u0435\u043C \u043A\u043E\u043C\u043F\u044C\u044E\u0442\u0435\u0440\u0435 \u0438\u043B\u0438 \u043F\u043B\u0430\u043D\u0448\u0435\u0442\u0435. \u0415\u0441\u043B\u0438 \u043A\u043E\u0434 \u043F\u0440\u0438\u0441\u043B\u0430\u043B\u0438 \u0432 \u0441\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0438 \u0438\u043B\u0438 \u043F\u043E\u043A\u0430\u0437\u0430\u043B\u0438 \u043D\u0430 \u0447\u0443\u0436\u043E\u043C \u0441\u0430\u0439\u0442\u0435 \u2014 \u044D\u0442\u043E \u043F\u043E\u043F\u044B\u0442\u043A\u0430 \u0443\u043A\u0440\u0430\u0441\u0442\u044C \u0430\u043A\u043A\u0430\u0443\u043D\u0442: \u043D\u0430\u0436\u043C\u0438\u0442\u0435 \u00AB\u041E\u0442\u043A\u043B\u043E\u043D\u0438\u0442\u044C\u00BB."),
        h("div", { class: "row" },
            h("button", { class: "primary", onClick: () => decide(true) }, "\u041F\u043E\u0434\u0442\u0432\u0435\u0440\u0434\u0438\u0442\u044C \u0432\u0445\u043E\u0434"),
            h("button", { class: "ghost danger", onClick: () => decide(false) }, "\u041E\u0442\u043A\u043B\u043E\u043D\u0438\u0442\u044C"))));
}
