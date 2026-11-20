import { h } from "../vendor/preact/index.js";
export function MiniPlayer({ player }) {
    if (!player)
        return null;
    return h("div", { class: "header-player", role: "group", "aria-label": "\u041C\u0443\u0437\u044B\u043A\u0430\u043B\u044C\u043D\u044B\u0439 \u043C\u0438\u043D\u0438-\u043F\u043B\u0435\u0435\u0440" },
        h("button", { "aria-label": "\u041C\u0438\u043D\u0438-\u043F\u043B\u0435\u0435\u0440: \u043F\u0440\u0435\u0434\u044B\u0434\u0443\u0449\u0438\u0439 \u0442\u0440\u0435\u043A", title: "\u041F\u0440\u0435\u0434\u044B\u0434\u0443\u0449\u0438\u0439 \u0442\u0440\u0435\u043A", disabled: !player.previousEnabled, onClick: player.previous },
            h("svg", { viewBox: "0 0 24 24", "aria-hidden": "true" },
                h("path", { d: "M5 4h3v16H5zM19 4v16L8 12z" }))),
        h("button", { "aria-label": player.paused ? "Мини-плеер: воспроизвести" : "Мини-плеер: пауза", title: player.paused ? "Воспроизвести" : "Пауза", onClick: player.toggle },
            h("svg", { viewBox: "0 0 24 24", "aria-hidden": "true" },
                h("path", { d: player.paused ? "M7 3v18l15-9z" : "M5 4h5v16H5zM14 4h5v16h-5z" }))),
        h("button", { "aria-label": "\u041C\u0438\u043D\u0438-\u043F\u043B\u0435\u0435\u0440: \u0441\u043B\u0435\u0434\u0443\u044E\u0449\u0438\u0439 \u0442\u0440\u0435\u043A", title: "\u0421\u043B\u0435\u0434\u0443\u044E\u0449\u0438\u0439 \u0442\u0440\u0435\u043A", disabled: !player.nextEnabled, onClick: player.next },
            h("svg", { viewBox: "0 0 24 24", "aria-hidden": "true" },
                h("path", { d: "M16 4h3v16h-3zM5 4l11 8-11 8z" }))),
        h("a", { href: "/music", class: "header-player-title", title: `${player.artist} — ${player.title}` },
            player.artist,
            " \u2014 ",
            player.title));
}
