import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { get } from "./api.js";
import { Empty, ErrorLine, Loading, useLoad } from "./ui.js";
import { Icon } from "./shellparts.js";
import { usePath } from "./router.js";
const questions = [
    { category: "Аккаунт", q: "Как найти все мои комментарии?", a: "Выгрузите сохранённые комментарии своего аккаунта в CSV. В файле будут текст, дата и ID записи. Удалённые комментарии не восстанавливаются.", href: "/help/comments", label: "Скачать мои комментарии" },
    { category: "Аккаунт", q: "Как настроить приватность профиля?", a: "Откройте настройки приватности. Выберите, кто видит профиль, список друзей и ваш статус онлайн, кто может писать вам и находить через поиск. Нажмите «Сохранить».", href: "/settings?tab=privacy", label: "Приватность" },
    { category: "Аккаунт", q: "Как защитить вход в аккаунт?", a: "В настройках безопасности подключите двухфакторную аутентификацию. Добавьте ключ в приложение-аутентификатор, подтвердите одноразовый код и сохраните резервные коды. Там же можно завершить другие сеансы.", href: "/settings?tab=security", label: "Безопасность" },
    { category: "Аккаунт", q: "Как восстановить пароль?", a: "На странице входа выберите восстановление пароля и укажите почту аккаунта. Для получения письма на сервере должна быть настроена почтовая доставка.", href: "/forgot-password", label: "Восстановить пароль" },
    { category: "Фото и истории", q: "Как загрузить несколько фотографий?", a: "В разделе «Фото» нажмите «Загрузить фото» и выберите несколько файлов JPEG, PNG или GIF. Размер каждого файла — до 10 МБ. После загрузки снимки появятся в галерее.", href: "/photos", label: "Мои фотографии" },
    { category: "Фото и истории", q: "Как выбрать альбомы для вкладки «Фото»?", a: "Нажмите шестерёнку в разделе «Фото», отметьте нужные альбомы и сохраните выбор. Настройка действует в этом браузере и не изменяет доступ к опубликованным снимкам.", href: "/photos", label: "Настроить фотографии" },
    { category: "Фото и истории", q: "Что происходит с фотографией в архиве?", a: "Архив скрывает снимок из основной фотогалереи. Фото остаётся в опубликованных записях и сообщениях. В меню «…» откройте архив, выберите снимок и нажмите «Восстановить».", href: "/photos", label: "Открыть галерею" },
    { category: "Фото и истории", q: "Сколько времени доступна история?", a: "История доступна 24 часа. Чтобы добавить её, нажмите «Моя история» в ленте, выберите фото, добавьте подпись и опубликуйте. При просмотре можно поставить историю на паузу или перейти к следующей.", href: "/", label: "Перейти в ленту" },
    { category: "Музыка и видео", q: "Какие аудиофайлы можно загрузить?", a: "Поддерживаются MP3, OGG, FLAC, WAV и M4A размером до 50 МБ и длительностью от 1 секунды до 20 минут. В разделе «Музыка» нажмите кнопку загрузки, выберите файл и заполните название и исполнителя.", href: "/music", label: "Открыть музыку" },
    { category: "Музыка и видео", q: "Почему видео не появляется сразу после загрузки?", a: "После загрузки видео обрабатывается сервером. В видеотеке отображается его состояние. Поддерживаются MP4, MOV, MKV и WEBM до 1 ГБ и 30 минут. При ошибке обработки попробуйте загрузить другой файл.", href: "/videos", label: "Мои видео" },
    { category: "Общение", q: "Как заблокировать пользователя?", a: "В настройках откройте «Чёрный список», нажмите «Добавить в чёрный список», найдите пользователя по имени и нажмите «Заблокировать». В этом же разделе можно снять блокировку.", href: "/settings?tab=blocks", label: "Чёрный список" },
    { category: "Общение", q: "Как изменить уведомления?", a: "В настройках уведомлений выберите нужные типы событий, Push и почтовую сводку. Сохраните изменения. Колокольчик сверху открывает последние события, а раздел «Уведомления» — полный список.", href: "/settings?tab=notifications", label: "Настроить уведомления" },
    { category: "Сообщества", q: "Как создать сообщество?", a: "Откройте «Сообщества», нажмите «Создать сообщество», заполните название и адрес. Выберите открытый доступ или закрытое сообщество со вступлением по заявке.", href: "/communities", label: "Мои сообщества" },
    { category: "Сообщества", q: "Где искать сообщества, которыми я управляю?", a: "В разделе «Сообщества» откройте вкладку «Управляемые». Она показывает сообщества, где вы владелец, администратор или редактор.", href: "/communities", label: "Открыть сообщества" },
    { category: "Походы", q: "Как найти поход и маршрут?", a: "В разделе «Походы» выберите мероприятие и ознакомьтесь с описанием, маршрутом и условиями участия. Для записи своего маршрута откройте «Записать трек» и разрешите доступ к геолокации.", href: "/hikes", label: "Найти поход" },
    { category: "Настройки", q: "Как изменить тему и боковое меню?", a: "В разделе «Аккаунт и внешний вид» выберите тему или откройте настройку пунктов меню. Тема и состав меню сохраняются в этом браузере. Основные разделы остаются доступными.", href: "/settings", label: "Внешний вид" },
    { category: "Настройки", q: "Где посмотреть баланс и подписку?", a: "Баланс и операции доступны в кошельке socialnet. Условия, срок действия и возможности подписки «Плюс» находятся в отдельном разделе подписки.", href: "/settings?tab=wallet", label: "Открыть кошелёк" },
];
export function HelpPage() { const [q, setQ] = useState(""); const [category, setCategory] = useState("Все"); const [mode, setMode] = useState("topics"); const categories = [...new Set(questions.map(x => x.category))]; const found = questions.filter(x => (category === "Все" || x.category === category) && `${x.q} ${x.a} ${x.category}`.toLocaleLowerCase("ru").includes(q.trim().toLocaleLowerCase("ru"))); return h("section", { class: "help-center" },
    h("header", { class: "library-panel" },
        h("h1", null, "\u041F\u043E\u043C\u043E\u0449\u044C socialnet"),
        h("label", { class: "help-search" },
            h(Icon, { name: "search" }),
            h("input", { type: "search", "aria-label": "\u041F\u043E\u0438\u0441\u043A \u043F\u043E \u0441\u043F\u0440\u0430\u0432\u043A\u0435", placeholder: "\u0412\u0432\u0435\u0434\u0438\u0442\u0435 \u0432\u0430\u0448 \u0432\u043E\u043F\u0440\u043E\u0441", value: q, onInput: e => setQ(e.currentTarget.value) })),
        h("nav", { class: "library-tabs", "aria-label": "\u0412\u0438\u0434 \u0441\u043F\u0440\u0430\u0432\u043A\u0438" },
            h("button", { "aria-pressed": mode === "topics", onClick: () => setMode("topics") }, "\u041F\u043E \u0442\u0435\u043C\u0430\u043C"),
            h("button", { "aria-pressed": mode === "list", onClick: () => setMode("list") }, "\u0421\u043F\u0438\u0441\u043E\u043A \u0432\u043E\u043F\u0440\u043E\u0441\u043E\u0432"))),
    h("div", { class: "help-layout" },
        h("div", { class: "help-answers" }, found.length === 0 ? h("div", { class: "library-panel" },
            h(Empty, null, "\u041D\u0438\u0447\u0435\u0433\u043E \u043D\u0435 \u043D\u0430\u0439\u0434\u0435\u043D\u043E. \u041F\u043E\u043F\u0440\u043E\u0431\u0443\u0439\u0442\u0435 \u0434\u0440\u0443\u0433\u043E\u0435 \u0441\u043B\u043E\u0432\u043E \u0438\u043B\u0438 \u0432\u044B\u0431\u0435\u0440\u0438\u0442\u0435 \u0432\u0441\u0435 \u0442\u0435\u043C\u044B.")) : (mode === "list" ? ["Все вопросы"] : categories.filter(c => found.some(x => x.category === c))).map(c => h("section", { class: "library-panel" },
            h("h2", null, c),
            found.filter(x => mode === "list" || x.category === c).map(x => h("details", { class: "help-question", key: x.q },
                h("summary", null, x.q),
                h("p", null, x.a),
                h("a", { href: x.href },
                    x.label,
                    " \u2192")))))),
        h("nav", { class: "settings-navigation", "aria-label": "\u0422\u0435\u043C\u044B \u0441\u043F\u0440\u0430\u0432\u043A\u0438" }, ["Все", ...categories].map(c => h("button", { class: c === category ? "active" : "", onClick: () => setCategory(c) }, c))))); }
export function LegalIndex() { const docs = useLoad(() => get("/legal"), []); return h("section", { class: "library-panel info-page" },
    h("h1", null, "\u041F\u0440\u0430\u0432\u043E\u0432\u0430\u044F \u0438\u043D\u0444\u043E\u0440\u043C\u0430\u0446\u0438\u044F"),
    h("p", null, "\u0414\u043E\u043A\u0443\u043C\u0435\u043D\u0442\u044B socialnet: \u043F\u0440\u0430\u0432\u0438\u043B\u0430 \u0438\u0441\u043F\u043E\u043B\u044C\u0437\u043E\u0432\u0430\u043D\u0438\u044F \u0438 \u043E\u0431\u0440\u0430\u0431\u043E\u0442\u043A\u0430 \u043F\u0435\u0440\u0441\u043E\u043D\u0430\u043B\u044C\u043D\u044B\u0445 \u0434\u0430\u043D\u043D\u044B\u0445."),
    docs.error ? h(ErrorLine, { error: docs.error }) : !docs.data ? h(Loading, null) : h("ul", { class: "info-links" }, docs.data.documents.map(d => h("li", null,
        h("a", { href: d.url, "data-external": true }, d.title),
        h("span", { class: "muted" },
            "\u0420\u0435\u0434\u0430\u043A\u0446\u0438\u044F ",
            d.version))))); }
const pages = {
    about: { title: "Общение начинается с интереса", intro: "socialnet объединяет ленту, сообщества, музыку, фотографии и инструменты для совместных походов.", cards: [["Делитесь моментами", "Публикуйте записи, фотографии и истории.", "/", "Открыть ленту"], ["Находите своих", "Создавайте сообщества вокруг общих интересов.", "/communities", "Сообщества"], ["Отправляйтесь вместе", "Находите походы, маршруты и попутчиков.", "/hikes", "Походы"]] },
    authors: { title: "Всё для авторов socialnet", intro: "Истории, фотографии, видео и сообщества — выберите свой способ рассказать о важном.", cards: [["Соберите сообщество", "Создайте место для читателей и участников.", "/communities", "Создать сообщество"], ["Пишите статьи", "Подготовьте подробный материал и поделитесь опытом.", "/articles/new", "Написать статью"], ["Планируйте публикации", "Просматривайте и управляйте отложенными записями.", "/scheduled", "Отложенные записи"], ["Публикуйте видео", "Загружайте ролики и следите за их обработкой.", "/videos", "Мои видео"]] },
    business: { title: "Ваше сообщество в socialnet", intro: "Рассказывайте о проекте, публикуйте новости и собирайте участников на мероприятия.", cards: [["Страница проекта", "Открытое или закрытое сообщество с ролями участников.", "/communities", "Открыть сообщества"], ["Встречи и походы", "Организуйте мероприятие с описанием и маршрутом.", "/hikes/new", "Создать поход"], ["Полезные материалы", "Публикуйте статьи для участников сообщества.", "/articles", "Читать статьи"]] },
    safety: { title: "Центр цифрового комфорта", intro: "Управляйте общением, видимостью профиля и безопасностью аккаунта.", cards: [["Защитите вход", "Подключите одноразовые коды и проверьте активные сеансы.", "/settings?tab=security", "Безопасность"], ["Выберите аудиторию", "Настройте доступ к профилю и личным сообщениям.", "/settings?tab=privacy", "Приватность"], ["Управляйте общением", "Добавляйте пользователей в чёрный список и снимайте блокировку.", "/settings?tab=blocks", "Чёрный список"], ["Настройте события", "Выберите уведомления, которые хотите получать.", "/settings?tab=notifications", "Уведомления"]] },
};
export function InfoPage({ kind }) { const page = pages[kind] ?? pages.about; return h("section", { class: `info-page info-${kind}` },
    h("header", { class: "info-hero" },
        h("span", { class: "info-eyebrow" }, "socialnet"),
        h("h1", null, page.title),
        h("p", null, page.intro),
        h("a", { class: "info-cta", href: page.cards[0][2] },
            page.cards[0][3],
            " \u2192")),
    h("div", { class: "info-card-grid" }, page.cards.map(([title, description, url, label]) => h("article", { class: "library-panel" },
        h("h2", null, title),
        h("p", null, description),
        h("a", { href: url },
            label,
            " \u2192")))),
    h("div", { class: "library-panel" },
        h("h2", null, "\u041D\u0443\u0436\u043D\u0430 \u043F\u043E\u043C\u043E\u0449\u044C?"),
        h("a", { href: "/help" }, "\u041D\u0430\u0439\u0442\u0438 \u043E\u0442\u0432\u0435\u0442 \u0432 \u0441\u043F\u0440\u0430\u0432\u043E\u0447\u043D\u043E\u043C \u0446\u0435\u043D\u0442\u0440\u0435 \u2192"))); }
export function SidebarFooter() { const ref = useRef(null); const path = usePath(); useEffect(() => { if (ref.current)
    ref.current.open = false; }, [path]); useEffect(() => { const outside = (e) => { if (ref.current && !ref.current.contains(e.target))
    ref.current.open = false; }; const esc = (e) => { if (e.key === "Escape" && ref.current?.open) {
    ref.current.open = false;
    ref.current.querySelector("summary")?.focus();
} }; document.addEventListener("pointerdown", outside); document.addEventListener("keydown", esc); return () => { document.removeEventListener("pointerdown", outside); document.removeEventListener("keydown", esc); }; }, []); return h("footer", { class: "sidebar-footer" },
    h("div", null,
        h("a", { href: "/authors" }, "\u0410\u0432\u0442\u043E\u0440\u0430\u043C"),
        h("a", { href: "/business" }, "\u0414\u043B\u044F \u0441\u043E\u043E\u0431\u0449\u0435\u0441\u0442\u0432"),
        h("a", { href: "/developers" }, "\u0420\u0430\u0437\u0440\u0430\u0431\u043E\u0442\u0447\u0438\u043A\u0430\u043C")),
    h("details", { ref: ref },
        h("summary", null, "\u0415\u0449\u0451 \u2304"),
        h("div", { class: "sidebar-footer-menu" },
            h("a", { href: "/about" }, "\u041E socialnet"),
            h("a", { href: "/legal-info" }, "\u041F\u0440\u0430\u0432\u043E\u0432\u0430\u044F \u0438\u043D\u0444\u043E\u0440\u043C\u0430\u0446\u0438\u044F"),
            h("a", { href: "/legal/privacy", "data-external": true }, "\u0417\u0430\u0449\u0438\u0442\u0430 \u0434\u0430\u043D\u043D\u044B\u0445"),
            h("a", { href: "/safety" }, "\u0426\u0438\u0444\u0440\u043E\u0432\u043E\u0439 \u043A\u043E\u043C\u0444\u043E\u0440\u0442"),
            h("a", { href: "/help" }, "\u041F\u043E\u043C\u043E\u0449\u044C"),
            h("span", null, "\u042F\u0437\u044B\u043A: \u0440\u0443\u0441\u0441\u043A\u0438\u0439")))); }
