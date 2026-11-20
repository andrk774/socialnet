import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { fullName, post } from "./api.js";
import { Avatar, ErrorLine, Time } from "./ui.js";
import { Video } from "./posts.js";
import { useMe } from "./session.js";
export function StoryViewer({ groups, group, index, choose, close }) {
    const me = useMe();
    const ref = useRef(null);
    const story = group.stories[index];
    const [elapsed, setElapsed] = useState(0);
    const [paused, setPaused] = useState(false);
    const [ready, setReady] = useState(false);
    const [reply, setReply] = useState("");
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    const [sent, setSent] = useState(false);
    const at = groups.findIndex(g => g.author.id === group.author.id);
    const move = (n) => { if (index + n >= 0 && index + n < group.stories.length)
        choose(group, index + n);
    else if (groups[at + n])
        choose(groups[at + n], n > 0 ? 0 : groups[at + n].stories.length - 1);
    else if (n > 0)
        close(); };
    useEffect(() => { ref.current?.showModal(); const old = document.body.style.overflow; document.body.style.overflow = "hidden"; return () => { document.body.style.overflow = old; }; }, []);
    useEffect(() => { setElapsed(0); setReady(false); setReply(""); setSent(false); setError(null); setPaused(false); }, [story.id]);
    useEffect(() => { if (paused || busy || !ready || !story.photo || reply)
        return; const timer = setInterval(() => setElapsed(n => Math.min(7000, n + 100)), 100); return () => clearInterval(timer); }, [paused, busy, ready, story.id, reply]);
    useEffect(() => { if (elapsed >= 7000 && story.photo)
        move(1); }, [elapsed]);
    async function send(e) { e.preventDefault(); if (!reply.trim() || busy)
        return; setBusy(true); setError(null); try {
        await post(`/users/${group.author.id}/messages`, { body: `Ответ на историю${story.caption ? `: ${story.caption}` : ""}\n${reply.trim()}` });
        setReply("");
        setSent(true);
        setPaused(true);
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
    } }
    return h("dialog", { ref: ref, class: "story-theater", "aria-label": `История: ${fullName(group.author)}`, onClose: close, onKeyDown: e => { if (e.target.closest("input,textarea") || busy)
            return; if (e.key === "ArrowRight") {
            e.preventDefault();
            move(1);
        } if (e.key === "ArrowLeft") {
            e.preventDefault();
            move(-1);
        } } },
        h("button", { class: "story-theater-close", "aria-label": "\u0417\u0430\u043A\u0440\u044B\u0442\u044C \u0438\u0441\u0442\u043E\u0440\u0438\u044E", onClick: close }, "\u00D7"),
        h("div", { class: "story-carousel" },
            groups[at - 1] && h("button", { class: "story-neighbor", "aria-label": `Истории: ${fullName(groups[at - 1].author)}`, onClick: () => choose(groups[at - 1], 0) },
                h(Avatar, { user: groups[at - 1].author }),
                h("span", null, fullName(groups[at - 1].author)),
                h("img", { src: groups[at - 1].stories[0].photo?.url ?? groups[at - 1].stories[0].video?.poster_url, alt: "" })),
            h("article", { class: "story-active" },
                h("div", { class: "story-progress", "aria-label": "\u041F\u0440\u043E\u0433\u0440\u0435\u0441\u0441 \u0438\u0441\u0442\u043E\u0440\u0438\u0438" }, group.stories.map((s, i) => h("progress", { key: s.id, max: 7000, value: i < index ? 7000 : i === index ? elapsed : 0 }))),
                h("header", null,
                    h(Avatar, { user: group.author, size: 40 }),
                    h("div", null,
                        h("a", { href: `/u/${group.author.username}`, onClick: close }, fullName(group.author)),
                        story.created_at && h(Time, { iso: story.created_at })),
                    story.photo && h("button", { "aria-label": paused ? "Продолжить историю" : "Приостановить историю", onClick: () => setPaused(!paused) }, paused ? "▶" : "Ⅱ")),
                h("div", { class: "story-active-media", onTimeUpdateCapture: e => { const v = e.target; if (story.video && Number.isFinite(v.duration) && v.duration > 0)
                        setElapsed(v.currentTime / v.duration * 7000); }, onEndedCapture: () => { if (story.video)
                        move(1); } },
                    story.photo && h("img", { key: story.id, class: "story-image", src: story.photo.url, alt: story.caption || "Фото истории", onLoad: () => setReady(true), onError: () => setError(new Error("Не удалось загрузить историю.")) }),
                    " ",
                    story.video && h(Video, { key: story.id, src: story.video.hls_url, mp4: story.video.mp4_url, poster: story.video.poster_url, title: story.caption }),
                    h("button", { class: "story-step story-back", "aria-label": "\u041F\u0440\u0435\u0434\u044B\u0434\u0443\u0449\u0430\u044F \u0438\u0441\u0442\u043E\u0440\u0438\u044F", disabled: at === 0 && index === 0 || busy, onClick: () => move(-1) }, "\u2039"),
                    h("button", { class: "story-step story-next", "aria-label": "\u0421\u043B\u0435\u0434\u0443\u044E\u0449\u0430\u044F \u0438\u0441\u0442\u043E\u0440\u0438\u044F", disabled: busy, onClick: () => move(1) }, "\u203A")),
                story.caption && h("p", { class: "story-caption" }, story.caption),
                h(ErrorLine, { error: error }),
                group.author.id !== me?.id ? h("form", { class: "story-reply", onSubmit: send },
                    h("input", { "aria-label": "\u041E\u0442\u0432\u0435\u0442 \u043D\u0430 \u0438\u0441\u0442\u043E\u0440\u0438\u044E", placeholder: "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435\u2026", maxLength: 3000, value: reply, onFocus: () => setPaused(true), onInput: e => { setReply(e.currentTarget.value); setSent(false); } }),
                    h("button", { disabled: busy || !reply.trim(), "aria-label": "\u041E\u0442\u043F\u0440\u0430\u0432\u0438\u0442\u044C \u043E\u0442\u0432\u0435\u0442" }, "\u27A4"),
                    sent && h("span", { role: "status" }, "\u0421\u043E\u043E\u0431\u0449\u0435\u043D\u0438\u0435 \u043E\u0442\u043F\u0440\u0430\u0432\u043B\u0435\u043D\u043E")) : h("footer", { class: "story-own" },
                    "\u0412\u0430\u0448\u0430 \u0438\u0441\u0442\u043E\u0440\u0438\u044F \u00B7 ",
                    index + 1,
                    " \u0438\u0437 ",
                    group.stories.length)),
            groups[at + 1] && h("button", { class: "story-neighbor", "aria-label": `Истории: ${fullName(groups[at + 1].author)}`, onClick: () => choose(groups[at + 1], 0) },
                h(Avatar, { user: groups[at + 1].author }),
                h("span", null, fullName(groups[at + 1].author)),
                h("img", { src: groups[at + 1].stories[0].photo?.url ?? groups[at + 1].stories[0].video?.poster_url, alt: "" }))));
}
