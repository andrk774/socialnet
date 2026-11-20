// Сквозной сценарий в настоящем браузере. Запускает Go-тест TestFrontendE2E:
//   node web/e2e/smoke.mjs <base-url> <папка для скриншотов>
import { createRequire } from "node:module";
// playwright ставится глобально; require учитывает NODE_PATH, а import — нет
const { chromium } = createRequire(import.meta.url)("playwright");
import assert from "node:assert/strict";

const [base, shots, clip] = process.argv.slice(2);
process.on("unhandledRejection", async (e) => {
  for (const ctx of browser.contexts()) for (const [i, p] of ctx.pages().entries()) await p.screenshot({ path: `${shots}/failure-${i}.png`, fullPage: true }).catch(() => {});
  console.error(e); process.exit(1);
});
const browser = await chromium.launch({args:['--use-fake-device-for-media-stream','--use-fake-ui-for-media-stream']});
const problems = [];

async function newUser(name, viewport = { width: 1280, height: 860 }) {
  const ctx = await browser.newContext({ viewport, locale: "ru-RU", colorScheme: "light" });
  const page = await ctx.newPage();
  // ответы 4xx — часть сценария (вход без cookie, бонус без почты); всё прочее,
  // включая нарушения CSP («Refused to …»), считается ошибкой
  page.on("console", (m) => { if (m.type() === "error" && !m.text().startsWith("Failed to load resource")) problems.push(`${name}: console: ${m.text()}`); });
  page.on("pageerror", (e) => problems.push(`${name}: pageerror: ${e.message}`));
  // Отменённое браузером — не ошибка: тайлы, которые карта уже не показывает,
  // Leaflet отменяет сам; обновление токена при загрузке обрывается переходом
  // на другую страницу, и сервер на этот случай принимает старый токен
  // ещё минуту (store.RefreshReuseGrace), так что сеанс не теряется.
  const benignAbort = (r) => r.failure()?.errorText === "net::ERR_ABORTED" && (r.url().includes("/tiles/") || r.url().endsWith("/api/v1/auth/refresh") || (r.method()==="GET" && r.url().endsWith("/api/v1/calls")));
  page.on("requestfailed", (r) => { if (!r.url().includes("/api/v1/ws") && !benignAbort(r)) problems.push(`${name}: failed ${r.method()} ${r.url()} ${r.failure()?.errorText}`); });
  await page.goto(base + "/");
  await page.getByRole("button", { name: "Зарегистрироваться" }).click();
  await page.getByLabel("Имя", { exact: true }).fill(name === "anna" ? "Анна" : "Борис");
  await page.getByLabel("Имя пользователя").fill(name);
  await page.getByLabel("Почта").fill(`${name}@example.com`);
  await page.getByLabel("Пароль").fill(`password-${name}`);
  // без отметок форма не отправляется: каждая обязательна и отдельна
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  assert.equal(await page.getByPlaceholder("Что у вас нового?").count(), 0, "registered without consents");
  await page.getByLabel("Мне исполнилось 18 лет").check();
  await page.getByLabel(/Принимаю пользовательское соглашение/).check();
  await page.getByLabel(/Даю согласие на обработку/).check();
  if (name === "anna") await page.screenshot({ path: `${shots}/00-register.png`, fullPage: true });
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page.getByPlaceholder("Что у вас нового?").waitFor();
  return { ctx, page };
}

const anna = await newUser("anna");

// Upload through the visible forms and verify persistence against the real API.
{
  const p = anna.page;
  const picture = await p.locator(".brand-mark").screenshot();
  await p.goto(base + "/u/anna");
  await p.locator('.avatar-upload input[type="file"]').setInputFiles({ name: "avatar.png", mimeType: "image/png", buffer: picture });
  await p.getByRole("button", { name: "Сохранить аватар", exact: true }).click();
  await p.locator('.profile-head img.avatar[src*="/media/"]').waitFor();
  await p.locator('.account-menu > summary img.avatar[src*="/media/"]').waitFor();
  await p.reload();
  await p.locator('.profile-head img.avatar[src*="/media/"]').waitFor();
  await p.getByLabel("Меню фотографии профиля").click();
  await p.getByRole("button", { name: "Открыть фото", exact: true }).click();
  await p.locator('.avatar-photo-dialog[open] img[src$="/full"]').waitFor();
  await p.keyboard.press("Escape");
  await p.getByLabel("Меню фотографии профиля").click();
  p.once("dialog", (d) => d.accept());
  await p.getByRole("button", { name: "Удалить фото", exact: true }).click();
  await p.locator('.profile-head .avatar-letter').waitFor();
  await p.reload();
  await p.locator('.profile-head .avatar-letter').waitFor();
  if (clip) {
    const samples = 44100 * 2;
    const wav = Buffer.alloc(44 + samples * 2);
    wav.write("RIFF", 0); wav.writeUInt32LE(wav.length - 8, 4); wav.write("WAVEfmt ", 8);
    wav.writeUInt32LE(16, 16); wav.writeUInt16LE(1, 20); wav.writeUInt16LE(1, 22);
    wav.writeUInt32LE(44100, 24); wav.writeUInt32LE(88200, 28); wav.writeUInt16LE(2, 32); wav.writeUInt16LE(16, 34);
    wav.write("data", 36); wav.writeUInt32LE(samples * 2, 40);
    for (let i = 0; i < samples; i++) wav.writeInt16LE(Math.round(5000 * Math.sin(i * 440 * 2 * Math.PI / 44100)), 44 + i * 2);
    await p.goto(base + "/music");
    await p.getByRole("button", { name: "Загрузить аудиозапись", exact: true }).click();
    await p.getByLabel("Файл", { exact: true }).setInputFiles({ name: "test.wav", mimeType: "audio/wav", buffer: wav });
    await p.getByLabel("Название", { exact: true }).fill("Проверка музыки");
    await p.getByLabel("Исполнитель", { exact: true }).fill("Анна");
    await p.getByRole("button", { name: "Загрузить", exact: true }).click();
    await p.getByText("Трек добавлен в вашу музыку.", { exact: true }).waitFor();
    await p.locator('.music-dialog[open]').getByLabel("Закрыть окно").click();
    await p.locator('.tracks').getByText("Проверка музыки", { exact: true }).waitFor();
    await p.locator('.track-btn').first().click();
    await p.locator('.player audio').waitFor({ state: 'attached' });
    await p.waitForFunction(() => { const a = document.querySelector('.player audio'); return a && !a.paused && a.currentTime > 0; });
    await p.locator('.music-tracks .track-btn').first().click();
    await p.waitForFunction(() => document.querySelector('.player audio').paused);
    await p.getByRole('button', { name: 'Воспроизвести', exact: true }).click();
    await p.waitForFunction(() => !document.querySelector('.player audio').paused);
    await p.getByRole('button',{name:'Повтор трека',exact:true}).click();
    await p.locator('.header-player-title').filter({hasText:'Проверка музыки'}).waitFor();
    await p.evaluate(()=>{window.__musicElement=document.querySelector('.player audio');});
    await p.locator('.sidenav a[href="/documents"]').click();
    await p.getByRole('heading',{name:/Файлы/}).waitFor();
    await p.waitForFunction(()=>{const a=document.querySelector('.player audio');return a===window.__musicElement&&!a.paused;});
    await p.getByRole('button',{name:'Мини-плеер: пауза',exact:true}).click();
    await p.waitForFunction(()=>window.__musicElement.paused);
    await p.getByRole('button',{name:'Мини-плеер: воспроизвести',exact:true}).click();
    await p.waitForFunction(()=>!window.__musicElement.paused);
    await p.setViewportSize({width:390,height:844});
    assert.ok(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'mini player overflow');
    await p.screenshot({path:`${shots}/mini-player-mobile.png`});
    await p.setViewportSize({width:1280,height:860});
    await p.locator('.header-player-title').click();
    await p.getByRole('heading',{name:'Музыка',exact:true}).waitFor({state:'attached'});
    await p.screenshot({path:`${shots}/mini-player-desktop.png`});
    await p.getByRole("button", { name: "Создать плейлист", exact: true }).click();
    await p.getByLabel("Название плейлиста", { exact: true }).fill("Мой тестовый плейлист");
    await p.getByRole("button", { name: "Создать", exact: true }).click();
    await p.getByRole("heading", { name: "Мой тестовый плейлист", exact: true }).waitFor();
    await p.getByRole("button", { name: "Моя музыка", exact: true }).click();
  await p.getByRole("button", { name: "Добавить в плейлист: Проверка музыки", exact: true }).first().click();
    await p.getByRole("button", { name: "Мой тестовый плейлист", exact: true }).click();
    await p.getByRole("button", { name: "Плейлисты", exact: true }).click();
    await p.locator('.playlist-card').filter({ hasText: "Мой тестовый плейлист" }).click();
    await p.locator('.tracks').getByText("Проверка музыки", { exact: true }).waitFor();
    await p.getByLabel("Поиск музыки", { exact: true }).fill("Проверка");
    await p.getByRole("heading", { name: "Результаты поиска" }).waitFor();
    await p.locator('.tracks').getByText("Проверка музыки", { exact: true }).waitFor();
    await p.getByRole("button", { name: "Главная", exact: true }).click();
    await p.getByRole("button", { name: "☷ Настроить", exact: true }).click();
    await p.getByLabel("Мой тестовый плейлист", { exact: true }).check();
    await p.getByRole("button", { name: "По списку", exact: true }).click();
    await p.getByRole("button", { name: "Применить", exact: true }).click();
    await p.getByRole("button", { name: "Слушать мой микс", exact: true }).click();
    await p.locator('.music-now').getByText("Проверка музыки", { exact: true }).waitFor();
    await p.setViewportSize({ width: 390, height: 844 });
    assert.ok(await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth), "music mobile overflow");
    await p.setViewportSize({ width: 1280, height: 860 });
    await p.goto(base + "/videos");
    await p.getByRole("button", { name: "Загрузить видео", exact: true }).click();
    await p.getByLabel("Файл", { exact: true }).setInputFiles(clip);
    await p.getByLabel("Название", { exact: true }).fill("Проверка загрузки видео");
    await p.getByRole("button", { name: "Загрузить", exact: true }).click();
    await p.getByText("Видео загружено. После обработки здесь появится плеер.", { exact: true }).waitFor();
    await p.getByLabel("Закрыть загрузку видео", { exact: true }).click();
    await p.getByRole("button", { name: "Смотреть: Проверка загрузки видео", exact: true }).click({ timeout: 90000 });
    await p.locator('figure.video video').waitFor({ timeout: 90000 });
    await p.keyboard.press("Escape");
    await p.locator('.video-viewer video').waitFor({ state: 'detached' });
    await p.reload();
    await p.getByLabel("Поиск видео", { exact: true }).fill("несуществующее видео");
    await p.getByText("Нет видео по выбранным условиям.", { exact: true }).waitFor();
    await p.getByLabel("Поиск видео", { exact: true }).fill("Проверка");
    await p.getByRole("button", { name: "Готовые", exact: true }).click();
    await p.getByRole("button", { name: "Смотреть: Проверка загрузки видео", exact: true }).click();
    await p.locator('figure.video video').waitFor();
  }
  await p.goto(base + "/");
  await p.getByPlaceholder("Что у вас нового?").waitFor();
}

// Каркас ленты: меню аккаунта, сохранение темы и реальные фотоистории.
{
  const p = anna.page;
  await p.getByLabel("Меню аккаунта", { exact: true }).click();
  await p.getByRole("link", { name: "Управление аккаунтом" }).waitFor();
  await p.getByRole("button", { name: "Тема: Системная", exact: true }).click();
  await p.getByRole("button", { name: "Тема: Светлая", exact: true }).click();
  await p.waitForFunction(() => document.documentElement.dataset.theme === "dark");
  assert.equal(await p.evaluate(() => document.documentElement.dataset.theme), "dark");
  await p.reload();
  await p.getByPlaceholder("Что у вас нового?").waitFor();
  await p.waitForFunction(() => document.documentElement.dataset.theme === "dark");
  assert.equal(await p.evaluate(() => document.documentElement.dataset.theme), "dark");
  await p.getByLabel("Меню аккаунта", { exact: true }).click();
  await p.getByRole("button", { name: "Тема: Тёмная", exact: true }).click();
  await p.keyboard.press("Escape");
  assert.equal(await p.locator(".account-menu").evaluate((e) => e.open), false);
  const picture = await p.locator(".brand-mark").screenshot();
  await p.getByRole("button", { name: "Моя история", exact: true }).click();
  await p.getByLabel("Фото для истории").setInputFiles({ name: "story.png", mimeType: "image/png", buffer: picture });
  await p.getByLabel("Подпись", { exact: true }).fill("Новый интерфейс socialnet");
  await p.getByRole("button", { name: "Опубликовать историю" }).click();
  await p.getByRole("button", { name: "Истории: Анна" }).click();
  await p.getByRole("dialog").getByText("Новый интерфейс socialnet", { exact: true }).waitFor();
  await p.getByRole('button',{name:'Приостановить историю',exact:true}).click();
  const progress=await p.locator('.story-progress progress').inputValue().catch(()=>p.locator('.story-progress progress').evaluate(e=>e.value));
  await p.waitForTimeout(250);
  assert.equal(await p.locator('.story-progress progress').evaluate(e=>e.value),Number(progress));
  await p.getByRole('dialog').screenshot({path:`${shots}/story-theater.png`});
  await p.keyboard.press("Escape");
  assert.equal(await p.getByRole("dialog").count(), 0);
  await p.getByLabel("Меню аккаунта", { exact: true }).click();
  await p.screenshot({ path: `${shots}/01a-account-menu.png` });
  await p.locator("h1").first().evaluate((e) => e.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true })));
  assert.equal(await p.locator(".account-menu").evaluate((e) => e.open), false);
  await p.setViewportSize({ width: 390, height: 844 });
  assert.equal(await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "mobile horizontal overflow");
  await p.getByRole("button", { name: "Ещё", exact: true }).click();
  await p.getByRole("dialog", { name: "Все разделы" }).waitFor();
  await p.getByRole("button", { name: "Ещё", exact: true }).click();
  await p.screenshot({ path: `${shots}/01d-mobile-feed.png` });
  await p.setViewportSize({ width: 1280, height: 860 });
}

// вход по QR-коду: новое устройство показывает код, телефон Анны подтверждает
{
  const g = await (await anna.ctx.browser().newContext({ locale: "ru-RU" })).newPage();
  await g.goto(base + "/");
  const started = g.waitForResponse((r) => r.url().endsWith("/api/v1/auth/qr/start"));
  await g.getByRole("button", { name: "Вход по QR-коду с телефона" }).click();
  const { url } = await (await started).json();
  await g.locator(".qr-login img[src^='data:image/svg+xml']").waitFor();
  await g.getByText(/Код действует ещё \d+ с\./).waitFor();
  await g.screenshot({ path: `${shots}/01b-qr-login.png` });
  // «телефон»: Анна открывает ссылку из кода и подтверждает
  await anna.page.goto(url.replace(/^https?:\/\/[^/]+/, base));
  await anna.page.getByRole("heading", { name: "Войти на другом устройстве?" }).waitFor();
  await anna.page.getByText(/это попытка украсть аккаунт/).waitFor();
  await anna.page.screenshot({ path: `${shots}/01c-qr-approve.png` });
  await anna.page.getByRole("button", { name: "Подтвердить вход" }).click();
  await anna.page.getByRole("heading", { name: "Вход подтверждён" }).waitFor();
  // новое устройство входит само
  await g.getByPlaceholder("Что у вас нового?").waitFor();
  await g.context().close();
  await anna.page.goto(base + "/");
}

// «Забыли пароль?» и ссылки из писем работают без входа
{
  const g = await (await anna.ctx.browser().newContext({ locale: "ru-RU" })).newPage();
  await g.goto(base + "/");
  await g.getByRole("link", { name: "Забыли пароль?" }).click();
  await g.getByLabel("Почта").fill("anna@example.com");
  await g.getByRole("button", { name: "Отправить ссылку" }).click();
  await g.getByText("Если такой адрес зарегистрирован").waitFor();
  await g.goto(base + "/reset-password?token=wrong");
  await g.getByLabel("Новый пароль").fill("new-password-123");
  await g.getByRole("button", { name: "Сохранить" }).click();
  await g.locator(".error").first().waitFor();
  await g.goto(base + "/verify-email?token=wrong");
  await g.getByText("Ссылка устарела?").waitFor();
  await g.context().close();
}

// документы открываются без входа, без скриптов
{
  const legal = await anna.ctx.newPage();
  await legal.goto(base + "/legal/privacy");
  await legal.getByRole("heading", { name: "Политика обработки персональных данных" }).waitFor();
  await legal.getByText("152-ФЗ").first().waitFor();
  await legal.screenshot({ path: `${shots}/01-privacy.png`, fullPage: false });
  await legal.close();
}
await anna.page.screenshot({ path: `${shots}/01-feed-empty.png`, fullPage: true });

// публикация с хэштегом, упоминанием и ссылкой
await anna.page.getByPlaceholder("Что у вас нового?").fill("Утро на Таганае. #урал @bob https://example.com/route?a=1.");
await anna.page.getByRole("button", { name: "Опубликовать" }).click();
const post = anna.page.locator("article.post").first();
await post.waitFor();
assert.equal(await post.locator('a[href="/tag/%D1%83%D1%80%D0%B0%D0%BB"]').count(), 1, "hashtag link");
assert.equal(await post.locator('a[href="https://example.com/route?a=1"]').getAttribute("rel"), "noopener noreferrer nofollow ugc");

// XSS через текст записи не исполняется
await anna.page.getByPlaceholder("Что у вас нового?").fill('<img src=x onerror="window.__xss=1"><script>window.__xss=2</script>');
await anna.page.getByRole("button", { name: "Опубликовать" }).click();
await anna.page.locator("article.post").nth(1).waitFor();
assert.equal(await anna.page.evaluate(() => window.__xss), undefined, "xss executed");

// реакция и ветка комментариев
const first = anna.page.locator("article.post").nth(1); // первая запись теперь вторая
await first.getByRole("button", { name: "Выбрать реакцию" }).click();
await first.getByRole("menuitem", { name: "Люблю" }).click();
await first.getByRole("button", { name: /Ваша реакция: Люблю/ }).waitFor();
await first.locator('.reaction-count').hover();
await first.locator('.reaction-preview .avatar').first().waitFor();
await first.locator('.reaction-count').click();
await anna.page.getByRole('dialog',{name:'Реакции на публикацию'}).getByRole('link',{name:'Анна',exact:true}).waitFor();
await anna.page.getByRole('button',{name:'Закрыть список',exact:true}).click();
await first.getByRole('button',{name:'Поделиться',exact:true}).click();
const share=anna.page.getByRole('dialog',{name:'Поделиться записью'});
assert.match(await share.getByLabel('Ссылка на запись').inputValue(),/\/p\/\d+$/);
await anna.ctx.grantPermissions(['clipboard-read','clipboard-write'],{origin:base});
await share.getByRole('button',{name:'Копировать ссылку',exact:true}).click();
await share.getByRole('status').filter({hasText:'Ссылка скопирована'}).waitFor();
await share.getByRole('button',{name:'Закрыть',exact:true}).click();
await first.getByRole("button", { name: /Комментарии/ }).click();
await first.getByPlaceholder("Написать комментарий").fill("Красота!");
await first.getByRole("button", { name: "Отправить", exact: true }).click();
await first.getByText("Красота!").waitFor();
await first.getByRole("button", { name: "Ответить" }).click();
await first.getByPlaceholder("Написать комментарий").fill("Анна, спасибо");
await first.getByRole("button", { name: "Отправить", exact: true }).click();
await first.getByText("Анна, спасибо").waitFor();
const rootComment=first.locator('.comment').filter({has:anna.page.getByText('Красота!',{exact:true})}).first();
await rootComment.getByRole('button',{name:'Нравится комментарий',exact:true}).first().click();
await rootComment.locator('.reaction-count').first().filter({hasText:'1'}).waitFor();
await rootComment.locator('.reaction-count').first().click();
const commentReactions=anna.page.getByRole('dialog',{name:'Реакции на комментарий',exact:true});
await commentReactions.getByRole('link',{name:'Анна',exact:true}).waitFor();
await commentReactions.getByRole('button',{name:'Закрыть список'}).click();
await first.getByRole('button',{name:'Стикеры',exact:true}).click();
const commentPicker=anna.page.getByRole('dialog',{name:'Вложения к комментарию',exact:true});
await commentPicker.locator('.comment-sticker-grid button').first().click();
await first.getByRole('button',{name:'Файл',exact:true}).click();
await commentPicker.getByLabel('Файл для комментария').setInputFiles({name:'comment-file.txt',mimeType:'text/plain',buffer:Buffer.from('comment attachment')});
await commentPicker.getByText('Выбрано 1 из 10',{exact:true}).waitFor();
await commentPicker.getByRole('button',{name:'Готово',exact:true}).click();
await first.getByRole('button',{name:'Отправить',exact:true}).click();
await first.locator('.comment-media .sticker-hello').waitFor();
await first.locator('.comment-media').getByRole('button',{name:/comment-file/}).waitFor();
await anna.page.screenshot({path:`${shots}/comment-content.png`,fullPage:true});
await anna.page.screenshot({ path: `${shots}/02-feed-post.png`, fullPage: true });

// второй пользователь; Анна пишет ему, он видит сообщение без перезагрузки
const bob = await newUser("bob");
await bob.page.getByRole("link", { name: /^Сообщения/ }).click();
await bob.page.getByText("Диалогов пока нет").waitFor();
await anna.page.goto(base + "/u/bob");
await anna.page.getByRole("button", { name: "Написать" }).click();
await anna.page.getByLabel("Сообщение").fill("Привет! Идёшь в субботу?");
await anna.page.getByRole("button", { name: "Отправить", exact: true }).click();
await anna.page.waitForURL(/\/im\/\d+$/);
await bob.page.getByText("Привет! Идёшь в субботу?").waitFor({ timeout: 5000 }); // список обновился по WebSocket
await bob.page.getByRole("link", { name: /Анна/ }).click();
await bob.page.getByLabel("Сообщение").fill("Иду!");
await bob.page.getByRole("button", { name: "Отправить", exact: true }).click();
await anna.page.locator(".msg").filter({ hasText: "Иду!" }).waitFor({ timeout: 5000 });
await anna.page.getByLabel("Сообщение", { exact: true }).fill("Первая строка");
await anna.page.getByLabel("Сообщение", { exact: true }).press("Shift+Enter");
await anna.page.getByLabel("Сообщение", { exact: true }).pressSequentially("Вторая строка");
assert.equal(await anna.page.getByLabel("Сообщение", { exact: true }).inputValue(), "Первая строка\nВторая строка");
await anna.page.getByLabel("Сообщение", { exact: true }).press("Enter");
await bob.page.locator(".msg").filter({ hasText: "Вторая строка" }).waitFor();
await anna.page.screenshot({ path: `${shots}/03-chat.png` });

// опрос: Анна спрашивает, Борис голосует и видит проценты
await anna.page.goto(base + "/");
await anna.page.getByRole("button", { name: "Опрос" }).click();
await anna.page.getByLabel("Вопрос").fill("Куда в субботу?");
await anna.page.getByLabel("Вариант 1").fill("Тургояк");
await anna.page.getByLabel("Вариант 2").fill("Таганай");
await anna.page.getByRole("button", { name: "Опубликовать" }).click();
await anna.page.locator("fieldset.poll").first().waitFor();
await bob.page.goto(base + "/u/anna");
const pollPost = bob.page.locator("article.post").filter({ has: bob.page.locator("fieldset.poll") }).first();
await pollPost.getByRole("button", { name: "Тургояк" }).click();
await pollPost.getByText("100%").waitFor();
await pollPost.getByText("Проголосовали: 1").waitFor();
// закладка
const saved = bob.page.waitForEvent("requestfinished", (r) => r.url().includes("/bookmarks/post/"), { timeout: 5000 });
await pollPost.getByRole("button", { name: "В закладки" }).click();
await pollPost.getByRole("button", { name: "В закладках" }).waitFor();
await saved; // запрос завершён целиком, и переход на другую страницу его не оборвёт
await bob.page.goto(base + "/bookmarks");
await bob.page.getByText("Куда в субботу?").waitFor();
await bob.page.screenshot({ path: `${shots}/08-poll-bookmarks.png`, fullPage: true });

// документ в записи: Борис скачивает его, файл совпадает
const pdf = "%PDF-1.4\n% маршрут\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n";
await anna.page.goto(base + "/");
await anna.page.locator(".composer input[type=file]:not([accept])").setInputFiles({ name: "Маршрут.pdf", mimeType: "application/pdf", buffer: Buffer.from(pdf) });
await anna.page.locator(".composer .doc").getByText("Маршрут").waitFor();
await anna.page.getByPlaceholder("Что у вас нового?").fill("Маршрут на субботу");
await anna.page.getByRole("button", { name: "Опубликовать" }).click();
await anna.page.locator("article.post .doc").first().waitFor();
await bob.page.goto(base + "/u/anna");
const dl = bob.page.waitForEvent("download");
await bob.page.getByRole("button", { name: "Маршрут.pdf" }).click();
const file = await dl;
assert.equal(file.suggestedFilename(), "Маршрут.pdf");
const fs0 = await import("node:fs");
assert.equal(fs0.readFileSync(await file.path(), "utf8"), pdf, "downloaded document differs");

// аудитория: запись «только мне» Борис больше не видит
const docPost = anna.page.locator("article.post").filter({ hasText: "Маршрут на субботу" });
await docPost.getByLabel("Кто видит запись").selectOption("only_me");
await docPost.getByText("видят: только я").waitFor();
await bob.page.goto(base + "/u/anna");
await bob.page.locator("article.post").first().waitFor();
assert.equal(await bob.page.getByText("Маршрут на субботу").count(), 0, "only_me post visible to bob");

// трек GPX: загрузка в редакторе, карточка со статистикой и профилем, чистый файл на скачивание
{
  const pts = [];
  for (let i = 0; i <= 300; i++) pts.push([55.05 + i * 10 / 111195, 400 + i + 30 * Math.sin(i / 20)]);
  for (let i = 299; i >= 0; i--) pts.push([55.05 + i * 10 / 111195, 400 + i + 30 * Math.sin(i / 20)]);
  const gpxText = `<?xml version="1.0" encoding="UTF-8"?><gpx version="1.1" creator="Garmin"><metadata><author><name>Анна</name></author></metadata>`
    + `<trk><name>Двуглавая сопка</name><trkseg>`
    + pts.map(([lat, ele], i) => `<trkpt lat="${lat.toFixed(6)}" lon="60.1"><ele>${ele.toFixed(1)}</ele><time>${new Date(Date.UTC(2026, 6, 4, 6, 0, i * 10)).toISOString()}</time></trkpt>`).join("")
    + `</trkseg></trk><wpt lat="55.06" lon="60.1"><name>&lt;img src=x onerror=alert(1)&gt;Родник</name></wpt></gpx>`;
  await anna.page.goto(base + "/");
  await anna.page.locator('.composer input[data-kind="gpx"]').setInputFiles({ name: "hike.gpx", mimeType: "application/gpx+xml", buffer: Buffer.from(gpxText) });
  await anna.page.locator(".composer .doc").getByText("Двуглавая сопка").waitFor();
  await anna.page.getByPlaceholder("Что у вас нового?").fill("Сходили на Двуглавую");
  await anna.page.getByRole("button", { name: "Опубликовать" }).click();
  const card = anna.page.locator("article.post section.route").first();
  await card.waitFor();
  await card.locator("dd", { hasText: "6 км" }).waitFor();
  await card.locator("dd", { hasText: "↑ 340 м" }).waitFor(); // набор без шума GPS
  assert.equal(await card.locator("svg.route-profile path").count(), 2, "elevation profile drawn");
  assert.equal(await card.locator("svg.route-sketch circle").count(), 2, "start and finish drawn");
  await card.getByText("Первые и последние 200 м скрыты").waitFor();
  await anna.page.screenshot({ path: `${shots}/12-route.png`, fullPage: true });

  // карта: подробный трек, тайлы через наш прокси, имена точек — только текстом
  await card.getByRole("button", { name: "Показать на карте" }).click();
  const map = card.locator(".route-map");
  await map.locator("path.map-route").waitFor({ state: "attached" }); // трек туда-обратно по меридиану: ширина 0
  await map.locator("img.leaflet-tile-loaded").first().waitFor();
  const tileSrc = await map.locator("img.leaflet-tile").first().getAttribute("src");
  assert.ok(tileSrc.startsWith("/tiles/") || tileSrc.startsWith(base + "/tiles/"), `tiles go through the proxy: ${tileSrc}`);
  await map.getByText("© тестовая карта").waitFor();
  assert.equal(await map.locator(".leaflet-control-attribution a").count(), 0, "no default Leaflet prefix link");
  await map.locator("path.map-waypoint").hover({ force: true });
  await anna.page.locator(".leaflet-tooltip", { hasText: "Родник" }).waitFor();
  assert.equal(await anna.page.locator(".leaflet-tooltip img").count(), 0, "waypoint name rendered as text");
  // точка на маршруте: щелчок по карте рядом с треком
  await card.getByRole("button", { name: "Отметить точку" }).click();
  const mapBox = await map.boundingBox();
  await map.click({ position: { x: mapBox.width / 2 + 8, y: mapBox.height / 2 } });
  await card.getByText(/^Место: 55\./).waitFor();
  await card.locator(".add-mark").getByLabel("Что это").selectOption({ label: "Родник" });
  await card.locator(".add-mark").getByLabel("Название").fill("Родник у тропы");
  await card.locator(".add-mark").getByLabel("Заметка").fill("Течёт и в августе");
  await card.getByRole("button", { name: "Сохранить точку" }).click();
  await card.locator("li.mark-spring", { hasText: "Родник у тропы" }).waitFor();
  await card.locator(".route-map path.mark-spring").waitFor();
  await card.locator("li.mark-spring").getByRole("button", { name: "На месте" }).click();
  await card.locator("li.mark-spring", { hasText: "подтвердили: 1" }).waitFor();
  await anna.page.screenshot({ path: `${shots}/13-map.png`, fullPage: true });
  // офлайн: маршрут с тайлами сохраняется на устройстве и открывается без сети
  await anna.page.evaluate(() => navigator.serviceWorker.ready.then(() => true));
  await card.getByRole("button", { name: "Сохранить офлайн" }).click();
  await card.getByRole("button", { name: "Сохранён офлайн ✓" }).waitFor({ timeout: 60000 });
  {
    const off = await anna.ctx.newPage();
    await off.goto(base + "/offline");
    await off.getByText("Двуглавая сопка").first().waitFor();
    await anna.ctx.setOffline(true);
    await off.reload();
    await off.getByText("Нет связи").waitFor();
    await off.getByRole("button", { name: "Открыть" }).first().click();
    await off.locator("img.leaflet-tile-loaded").first().waitFor();
    await off.screenshot({ path: `${shots}/17-offline.png`, fullPage: true });
    await anna.ctx.setOffline(false);
    await off.close();
  }
  await card.getByRole("button", { name: "Во весь экран" }).click();
  await anna.page.locator(".route-map-full").waitFor();
  await anna.page.waitForFunction(() => document.body.classList.contains("no-scroll"));
  await anna.page.keyboard.press("Escape");
  await anna.page.waitForFunction(() => !document.querySelector(".route-map-full") && !document.body.classList.contains("no-scroll"));
  // открытый каталог: страница без входа и без скриптов
  await card.getByRole("button", { name: "Открыть в каталоге маршрутов…" }).click();
  await card.getByLabel("Описание для каталога").fill("Старт от Белого ключа, вода у родника");
  await card.getByRole("button", { name: "Открыть для всех" }).click();
  await card.getByRole("link", { name: "страница маршрута" }).waitFor();
  {
    // гость без входа и без JavaScript видит маршрут в каталоге
    const guest = await anna.ctx.browser().newContext({ locale: "ru-RU", javaScriptEnabled: false });
    const g = await guest.newPage();
    assert.equal((await g.goto(base + "/trails")).status(), 200, "trails catalog");
    await g.getByRole("link", { name: "Двуглавая сопка" }).first().click();
    await g.getByRole("heading", { name: "Двуглавая сопка" }).waitFor();
    await g.getByText("Старт от Белого ключа").waitFor();
    await g.screenshot({ path: `${shots}/18-trail.png`, fullPage: true });
    await guest.close();
  }
  const dl = anna.page.waitForEvent("download");
  await card.getByRole("button", { name: "Скачать GPX" }).click();
  const gpxFile = await (await import("node:fs")).readFileSync(await (await dl).path(), "utf8");
  assert.ok(gpxFile.includes("<trkpt") && !gpxFile.includes("<time>") && !gpxFile.includes("Анна") && !gpxFile.includes("Garmin"), "downloaded GPX must be clean");
}

// поход: Анна собирает группу по заявкам, Борис просится, Анна принимает
{
  await anna.page.getByRole("link", { name: "Походы", exact: true }).click();
  await anna.page.getByText("Ближайших походов нет").waitFor();
  await anna.page.getByRole("link", { name: "Организовать поход" }).click();
  await anna.page.getByLabel("Название").fill("Двуглавая сопка с ночёвкой");
  const d = new Date(Date.now() + 5 * 864e5);
  const local = (x) => new Date(x.getTime() - x.getTimezoneOffset() * 6e4).toISOString().slice(0, 16);
  await anna.page.getByLabel("Начало").fill(local(d));
  await anna.page.getByLabel("Контрольный срок возвращения").fill(local(new Date(d.getTime() + 30 * 36e5)));
  await anna.page.getByLabel("Сложность").selectOption({ label: "Средний" });
  await anna.page.getByLabel("Мест (с организатором)").fill("2");
  await anna.page.getByLabel("Место сбора").fill("Ж/д вокзал Златоуста, 7:00");
  await anna.page.getByLabel("Маршрут").selectOption({ label: "Двуглавая сопка" });
  await anna.page.getByLabel(/Снаряжение/).fill("Треккинговые ботинки\nДождевик\nКоврик и спальник");
  await anna.page.getByLabel(/Участие по заявке/).check();
  await anna.page.getByRole("button", { name: "Создать поход" }).click();
  await anna.page.getByRole("heading", { name: "Двуглавая сопка с ночёвкой" }).waitFor();
  await anna.page.getByText("1 из 2 · свободно 1").waitFor();
  await anna.page.locator(".gear li", { hasText: "Коврик и спальник" }).waitFor();
  await anna.page.locator("section.route").waitFor(); // маршрут похода
  // погода на дни похода: шестой день — гроза, день похода выделен
  await anna.page.locator(".weather-days li").nth(9).waitFor();
  await anna.page.locator(".weather-days li.hike-day").first().waitFor();
  await anna.page.locator(".weather-days li.warn", { hasText: "гроза" }).waitFor();
  const hikeURL = anna.page.url();
  // QR-код похода: картинка, переход по коду считается, организатор видит счётчик
  {
    const hikeId = new URL(hikeURL).pathname.split("/").pop();
    await anna.page.goto(`${base}/qr/e/${hikeId}`);
    await anna.page.waitForURL(/\/events\/\d+\?from=qr$/);
    await anna.page.locator(".hike-qr summary").click();
    await anna.page.locator(".hike-qr img[src^='data:image/svg+xml']").waitFor();
    await anna.page.locator(".hike-qr [data-stat=scans]", { hasText: "1" }).waitFor();
    await anna.page.locator(".hike-qr").screenshot({ path: `${shots}/14c-hike-qr.png` });
    await anna.page.goto(hikeURL);
  }
  // гость на главной видит поход в витрине под формой входа
  {
    const g = await (await anna.ctx.browser().newContext({ locale: "ru-RU" })).newPage();
    await g.goto(base + "/");
    await g.locator(".showcase-hikes").getByRole("link", { name: "Двуглавая сопка с ночёвкой" }).waitFor();
    await g.screenshot({ path: `${shots}/00-guest.png`, fullPage: true });
    await g.context().close();
  }

  await bob.page.goto(base + "/hikes");
  await bob.page.getByRole("link", { name: "Двуглавая сопка с ночёвкой" }).click();
  await bob.page.getByLabel(/Пара слов организатору/).fill("Был на Таганае трижды, есть палатка");
  await bob.page.getByRole("button", { name: "Подать заявку" }).click();
  await bob.page.getByText("Заявка на рассмотрении").waitFor();

  await anna.page.goto(hikeURL);
  const app = anna.page.locator(".applications li", { hasText: "есть палатка" });
  await app.getByRole("button", { name: "Принять" }).click();
  await anna.page.getByText("2 из 2 · мест нет").waitFor();
  await anna.page.screenshot({ path: `${shots}/14-hike.png`, fullPage: true });
  await bob.page.reload();
  await bob.page.getByText("Вы идёте").waitFor();
  await bob.page.getByRole("heading", { name: "Мои доверенные контакты" }).waitFor();
  // чат похода: Борис в нём, пишет группе, Анна видит
  await bob.page.getByRole("link", { name: "Чат похода" }).click();
  await bob.page.getByText("Чат похода: в нём все, кто идёт.").waitFor();
  await bob.page.locator(".msg-service", { hasText: "в группе похода" }).waitFor();
  await bob.page.getByLabel("Сообщение").fill("Кто берёт котелок?");
  await bob.page.locator(".chat-form").getByRole("button", { name: "Отправить" }).click();
  await anna.page.goto(hikeURL);
  await anna.page.getByRole("link", { name: "Чат похода" }).click();
  await anna.page.locator(".msg", { hasText: "Кто берёт котелок?" }).waitFor();
  await bob.page.goto(hikeURL);
  await anna.page.goto(hikeURL);
  // отзыв о маршруте каталога: звёзды и текст, средняя оценка
  {
    const rc = bob.page.locator("section", { has: bob.page.getByRole("heading", { name: "Маршрут", exact: true }) });
    await rc.getByRole("button", { name: "Показать на карте" }).click();
    await rc.locator(".route-reviews").getByText("отзывов пока нет").waitFor();
    await rc.locator(".star-input label").nth(3).click();
    await rc.locator(".review-form").getByLabel("Отзыв").fill("Курумник после дождя скользкий");
    await rc.getByRole("button", { name: "Оставить отзыв" }).click();
    await rc.locator(".route-reviews .rating-line", { hasText: "4.0 · 1 отзыв" }).waitFor();
    await rc.locator(".reviews li", { hasText: "Курумник после дождя скользкий" }).waitFor();
    await rc.getByRole("button", { name: "Показать на карте" }).or(rc.getByRole("button", { name: "Скрыть карту" })).click();
  }
  // подвоз: Борис предлагает места, Анна просится, Борис берёт
  await bob.page.getByText("Предложить места в машине").click();
  const dep = new Date(Date.now() + 5 * 864e5 - 2 * 36e5);
  await bob.page.locator(".rides").getByLabel("Отправление").fill(new Date(dep.getTime() - dep.getTimezoneOffset() * 6e4).toISOString().slice(0, 16));
  await bob.page.locator(".rides").getByLabel("Свободных мест").fill("2");
  await bob.page.locator(".rides").getByRole("button", { name: "Предложить" }).click();
  await bob.page.locator("li.ride", { hasText: "свободно 2 из 2" }).waitFor();
  await anna.page.reload();
  await anna.page.locator("li.ride").getByRole("button", { name: "Попроситься" }).click();
  await anna.page.locator("li.ride").getByLabel("Пара слов водителю").fill("Заберёшь у вокзала?");
  await anna.page.locator("li.ride").getByRole("button", { name: "Отправить" }).click();
  await anna.page.locator("li.ride").getByText("Заявка у водителя").waitFor();
  await bob.page.reload();
  await bob.page.locator("li.ride").getByRole("button", { name: "Взять" }).click();
  await bob.page.locator("li.ride", { hasText: "свободно 1 из 2" }).waitFor();
  await anna.page.reload();
  await anna.page.locator("li.ride").getByText("Вы едете").waitFor();
  // в календарь: файл .ics с началом похода
  {
    const [dl] = await Promise.all([anna.page.waitForEvent("download"), anna.page.getByRole("button", { name: "В календарь" }).click()]);
    const ics = (await import("node:fs")).readFileSync(await dl.path(), "utf8");
    assert.match(ics, /BEGIN:VEVENT[\s\S]*DTSTART:\d{8}T\d{6}Z/, "ics has the start");
    assert.ok(dl.suggestedFilename().endsWith(".ics"), dl.suggestedFilename());
  }
  // лист для печати: карта, профиль, точки с координатами, снаряжение, 112
  await anna.page.getByRole("link", { name: "Лист для печати" }).click();
  await anna.page.locator(".print-sheet .route-map path.map-route").waitFor({ state: "attached" });
  await anna.page.locator(".print-table td", { hasText: "Финиш" }).waitFor();
  await anna.page.locator(".print-table", { hasText: "с. ш." }).waitFor();
  await anna.page.locator(".checklist li", { hasText: "Коврик и спальник" }).waitFor();
  await anna.page.getByText("единый номер экстренных служб").waitFor();
  await anna.page.locator("img.leaflet-tile-loaded").first().waitFor();
  await anna.page.emulateMedia({ media: "print" });
  assert.equal(await anna.page.locator(".sidenav").isVisible(), false, "navigation hidden when printing");
  await anna.page.pdf({ path: `${shots}/16-route-sheet.pdf`, format: "A4", printBackground: true });
  await anna.page.emulateMedia({ media: "screen" });
  await anna.page.goBack();
  await anna.page.getByRole("button", { name: "Мы вернулись" }).click();
  await anna.page.getByText(/группа вернулась/).waitFor();
  await bob.page.goto(base + "/notifications");
  await bob.page.getByRole("link", { name: "принял(а) вашу заявку в поход" }).waitFor();
}

// попутчики: Борис ищет компанию, Анна находит объявление фильтром и откликается
{
  await bob.page.getByRole("link", { name: "Попутчики" }).click();
  await bob.page.getByText("Объявлений не нашлось").waitFor();
  await bob.page.getByText("Разместить объявление").click();
  const d = new Date(Date.now() + 7 * 864e5);
  const iso = (x) => new Date(x.getTime() - x.getTimezoneOffset() * 6e4).toISOString().slice(0, 10);
  await bob.page.getByLabel("Заголовок").fill("Ищу компанию на Откликной гребень");
  await bob.page.getByLabel("Район", { exact: true }).last().fill("Таганай");
  await bob.page.locator("details.create").getByLabel("С", { exact: true }).fill(iso(d));
  await bob.page.getByLabel("Сколько человек ищу").fill("2");
  await bob.page.getByRole("button", { name: "Разместить" }).click();
  await bob.page.getByRole("heading", { name: "Мои объявления" }).waitFor();

  await anna.page.getByRole("link", { name: "Попутчики" }).click();
  await anna.page.locator("form.filters").getByLabel("Район").fill("иремель");
  await anna.page.getByRole("button", { name: "Найти" }).click();
  await anna.page.getByText("Объявлений не нашлось").waitFor();
  await anna.page.locator("form.filters").getByLabel("Район").fill("таганай");
  await anna.page.getByRole("button", { name: "Найти" }).click();
  const ad = anna.page.locator("li.ad", { hasText: "Откликной гребень" });
  await ad.getByRole("button", { name: "Откликнуться" }).click();
  await ad.getByLabel("Сообщение автору").fill("Пойду, есть палатка");
  await anna.page.screenshot({ path: `${shots}/15-companions.png`, fullPage: true });
  await ad.getByRole("button", { name: "Отправить" }).click();
  await ad.getByText("Отклик отправлен").waitFor();
  await bob.page.goto(base + "/im");
  await bob.page.getByText(/Отклик на объявление/).first().waitFor();
}

// «Плюс»: что даёт и сколько стоит (покупка — после подтверждения почты, проверена в Go-тестах)
await anna.page.getByRole("link", { name: "Плюс" }).click();
await anna.page.locator(".plus-table td", { hasText: "Своих лент" }).waitFor();
await anna.page.getByRole("button", { name: /Подключить за \d+ монет/ }).waitFor();

// своя лента по словам: «таганай» находит «на Таганае»
{
  await bob.page.getByRole("link", { name: "Мои ленты" }).click();
  await bob.page.getByRole("button", { name: "Новая лента" }).click();
  await bob.page.getByLabel("Название").fill("Таганай");
  await bob.page.getByLabel("Слова через запятую").fill("таганай, иремель");
  await bob.page.getByLabel("Кроме слов").fill("продам");
  await bob.page.getByRole("button", { name: "Создать ленту" }).click();
  await bob.page.locator(".antenna article.post", { hasText: "Утро на Таганае" }).waitFor();
}

// статья: разметка, фото в тексте, предпросмотр; HTML из текста остаётся текстом
{
  const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");
  await anna.page.getByRole("link", { name: "Статьи" }).click();
  await anna.page.getByRole("link", { name: "Написать отчёт" }).click();
  await anna.page.getByLabel("Заголовок").fill("Три дня на Таганае");
  await anna.page.locator(".article-editor textarea").fill("## День первый\n\nВышли в **7 утра** от Белого ключа.\n\n<img src=x onerror=\"window.__xss=3\">\n\n- вода\n- палатка\n");
  await anna.page.locator(".file-chip input").setInputFiles({ name: "вершина.png", mimeType: "image/png", buffer: png });
  await anna.page.waitForFunction(() => document.querySelector(".article-editor textarea")?.value.includes("](media:")); // фото загрузилось и вставлено
  await anna.page.getByRole("button", { name: "Предпросмотр" }).click();
  await anna.page.locator(".preview-box strong", { hasText: "7 утра" }).waitFor();
  await anna.page.locator(".preview-box figure img").waitFor();
  await anna.page.getByRole("button", { name: "Опубликовать" }).click();
  await anna.page.getByRole("heading", { name: "Три дня на Таганае" }).waitFor();
  assert.equal(await anna.page.locator(".article-body img[src='x']").count(), 0, "html in article text is not rendered");
  assert.equal(await anna.page.evaluate(() => window.__xss), undefined, "no script from article text");
  await anna.page.screenshot({ path: `${shots}/19-article.png`, fullPage: true });
  await bob.page.goto(base + "/articles");
  await bob.page.locator(".article-card", { hasText: "Три дня на Таганае" }).waitFor();
  // общий поиск из шапки: поход и статья
  await bob.page.locator("#q").fill("двуглав");
  await bob.page.locator("#q").press("Enter");
  await bob.page.locator(".search-section[data-kind=hikes]").getByRole("link", { name: "Двуглавая сопка с ночёвкой" }).waitFor();
  await bob.page.locator("#q").fill("белого ключа");
  await bob.page.locator("#q").press("Enter");
  await bob.page.locator(".search-section[data-kind=articles]").getByRole("link", { name: "Три дня на Таганае" }).waitFor();
  await bob.page.screenshot({ path: `${shots}/23-search.png` });
  await bob.page.goto(base + "/articles");
  // жалоба на статью — уйдёт в админку
  await bob.page.locator(".article-card", { hasText: "Три дня на Таганае" }).getByRole("link").first().click();
  await bob.page.getByRole("button", { name: "Пожаловаться" }).click();
  await bob.page.getByLabel("Причина").selectOption("Спам или реклама");
  await bob.page.getByRole("button", { name: "Отправить жалобу" }).click();
  await bob.page.getByText("Жалоба отправлена модераторам.").waitFor();
}

// сообщество со своими эмодзи: администратор добавляет, участник реагирует
{
  const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");
  await anna.page.getByRole("link", { name: "Сообщества" }).click();
  await anna.page.getByRole("button", {name:"Создать сообщество",exact:true}).click();
  await anna.page.getByLabel("Название").fill("Клуб Таганая");
  await anna.page.getByLabel("Адрес").fill("taganay_club");
  await anna.page.getByRole("dialog", { name: "Создать сообщество" }).getByRole("button", { name: "Создать", exact: true }).click();
  await anna.page.locator('.community-main .community-description').filter({hasText:"Клуб Таганая"}).click();
  await anna.page.getByText(/^Эмодзи сообщества/).click();
  await anna.page.locator(".emoji-manager").getByLabel("Код").fill("gora");
  await anna.page.locator(".emoji-manager").getByLabel("Картинка").setInputFiles({ name: "gora.png", mimeType: "image/png", buffer: png });
  await anna.page.locator(".emoji-manager").getByRole("button", { name: "Добавить" }).click();
  await anna.page.locator(".emoji-list li", { hasText: ":gora:" }).waitFor();
  await anna.page.getByPlaceholder("Что у вас нового?").fill("Сбор клуба в субботу");
  await anna.page.getByRole("button", { name: "Опубликовать" }).click();
  await bob.page.goto(base + "/c/taganay_club");
  const cpost = bob.page.locator("article.post", { hasText: "Сбор клуба в субботу" });
  await cpost.getByRole("button", { name: "Выбрать реакцию" }).click();
  await cpost.getByRole("menuitem", { name: "gora" }).click();
  await cpost.locator(".react-wrap button.chip.active img.emoji").waitFor();

  // сбор клуба: цель, срок, прогресс (пожертвования — в Go-тестах: нужна подтверждённая почта)
  await anna.page.getByText("Объявить сбор").click();
  await anna.page.getByLabel("На что").fill("Рации для клуба");
  await anna.page.getByLabel("Цель, монет").fill("500");
  const until = new Date(Date.now() + 10 * 864e5);
  await anna.page.locator(".fundraisers").getByLabel("До").fill(new Date(until.getTime() - until.getTimezoneOffset() * 6e4).toISOString().slice(0, 16));
  await anna.page.getByRole("button", { name: "Объявить" }).click();
  await anna.page.locator("li.fund", { hasText: "Собрано 0 из 500" }).waitFor();
  await bob.page.goto(base + "/fundraisers");
  await bob.page.locator("li.fund", { hasText: "Рации для клуба" }).getByRole("button", { name: "Поддержать" }).waitFor();
}

// отложенная запись
await anna.page.goto(base + "/");
await anna.page.getByPlaceholder("Что у вас нового?").fill("Это выйдет через час");
await anna.page.getByRole("button", { name: "Отложить" }).click();
await anna.page.getByRole("button", { name: "Запланировать" }).click();
await anna.page.getByText(/Запись выйдет/).waitFor();
await anna.page.goto(base + "/scheduled");
await anna.page.getByText("Это выйдет через час").waitFor();
await anna.page.screenshot({ path: `${shots}/09-scheduled.png`, fullPage: true });

// админка (Анну сделал администратором Go-тест): сводка, поиск, карточка, сброс пароля письмом
anna.page.on("dialog", (d) => d.accept());
await anna.page.goto(base + "/admin");
await anna.page.getByText("Пользователей").waitFor();
await anna.page.screenshot({ path: `${shots}/10-admin-stats.png`, fullPage: true });
// жалоба на статью: видно, что обжаловано; скрыть и вернуть
await anna.page.getByRole("tab", { name: "Жалобы" }).click();
{
  const rep = anna.page.locator("li[data-type=article]");
  await rep.getByText("Три дня на Таганае").waitFor();
  await rep.getByText(/Спам или реклама/).waitFor();
  await anna.page.screenshot({ path: `${shots}/10b-admin-reports.png`, fullPage: true });
  await rep.getByRole("button", { name: "Скрыть" }).click();
  await anna.page.getByText("Жалоб нет.").waitFor();
  await anna.page.locator("#rep-status").selectOption("resolved");
  await rep.getByText("скрыто").waitFor();
  await rep.getByRole("button", { name: "Вернуть" }).click();
  await rep.getByRole("button", { name: "Скрыть" }).waitFor();
  assert.equal(await rep.getByText("скрыто").count(), 0, "article is back");
}
// предупреждение на территории: модератор добавляет — баннер на походе
{
  await anna.page.getByRole("tab", { name: "Предупреждения" }).click();
  const form = anna.page.locator(".notice-form");
  await form.getByLabel("Что").selectOption({ label: "Противопожарный режим" });
  await form.getByLabel("Заголовок").fill("Запрет посещения лесов");
  await form.getByLabel("Ссылка на источник").fill("https://example.gov.ru/fire");
  for (const [label, v] of [["Широта от", "54.9"], ["Долгота от", "59.9"], ["Широта до", "55.2"], ["Долгота до", "60.3"]]) await form.getByLabel(label).fill(v);
  await form.getByRole("button", { name: "Добавить предупреждение" }).click();
  await anna.page.locator(".notices-admin li", { hasText: "Запрет посещения лесов" }).waitFor();
  await anna.page.goto(base + "/hikes");
  await anna.page.getByRole("link", { name: "Двуглавая сопка с ночёвкой" }).click();
  await anna.page.locator(".notice-fire", { hasText: "Запрет посещения лесов" }).waitFor();
  await anna.page.screenshot({ path: `${shots}/14b-hike-notice.png` });
  await anna.page.goto(base + "/admin");
}
await anna.page.getByRole("tab", { name: "Пользователи" }).click();
await anna.page.getByPlaceholder(/id, почта/).fill("bob");
await anna.page.getByRole("button", { name: "Найти" }).click();
await anna.page.getByRole("link", { name: "Борис" }).click();
await anna.page.getByText("Журнал безопасности").waitFor();
assert.equal(await anna.page.locator(".admin-card").getByText(/хэш/).count(), 1, "explains why there is no password");
await anna.page.getByRole("button", { name: "Отправить ссылку для сброса пароля" }).click();
await anna.page.getByText(/Письмо со ссылкой отправлено/).waitFor();
await anna.page.screenshot({ path: `${shots}/11-admin-user.png`, fullPage: true });
// Борис выкинут из всех сеансов: следующий запрос вернёт его на вход
await bob.page.goto(base + "/");
await bob.page.getByRole("button", { name: "Войти" }).first().waitFor();

// профиль, кошелёк, безопасность; после перезагрузки сеанс восстанавливается из cookie
await anna.page.goto(base + "/wallet");
await anna.page.getByText("Операций пока не было").waitFor();
// без подтверждённой почты бонус не дают — интерфейс объясняет почему
await anna.page.getByRole("button", { name: "Забрать ежедневный бонус" }).click();
await anna.page.getByRole("alert").filter({ hasText: "подтвердите почту" }).waitFor();
await anna.page.getByRole("button", { name: "Отправить письмо ещё раз" }).click();
await anna.page.getByText("Письмо отправлено").waitFor();
await anna.page.screenshot({ path: `${shots}/04-wallet.png`, fullPage: true });
await anna.page.goto(base + "/settings?tab=security");
await anna.page.getByText("это устройство").waitFor();
await anna.page.goto(base + "/u/anna");
await anna.page.getByRole("heading", { name: "Анна" }).waitFor();
await anna.page.screenshot({ path: `${shots}/05-profile.png`, fullPage: true });

// видео: загрузка через API от имени Анны, обработка воркером, воспроизведение
if (clip) {
  const fs = await import("node:fs");
  const bytes = fs.readFileSync(clip).toString("base64");
  const vid = await anna.page.evaluate(async (b64) => {
    const { access_token } = await (await fetch("/api/v1/auth/refresh", { method: "POST" })).json();
    const fd = new FormData();
    fd.append("file", new Blob([Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))], { type: "video/mp4" }), "Ильмень.mp4");
    const v = await (await fetch("/api/v1/videos", { method: "POST", headers: { Authorization: `Bearer ${access_token}` }, body: fd })).json();
    for (let i = 0; i < 100; i++) {
      const r = await (await fetch(`/api/v1/videos/${v.id}`, { headers: { Authorization: `Bearer ${access_token}` } })).json();
      if (r.status === "ready") break;
      await new Promise((res) => setTimeout(res, 300));
    }
    await fetch("/api/v1/posts", { method: "POST", headers: { Authorization: `Bearer ${access_token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ text: "Видео с Ильменя", video_ids: [v.id] }) });
    return v.id;
  }, bytes);
  console.log("uploaded video", vid);

  // играет ли видео в браузере: hls=false — браузер без встроенного HLS
  async function plays(hls) {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 860 } });
    if (!hls) await ctx.addInitScript(() => {
      const orig = HTMLMediaElement.prototype.canPlayType;
      HTMLMediaElement.prototype.canPlayType = function (t) { return /mpegurl/i.test(t) ? "" : orig.call(this, t); };
    });
    const p = await ctx.newPage();
    p.on("pageerror", (e) => problems.push(`video(${hls}): ${e.message}`));
    await p.goto(base + "/");
    await p.getByLabel("Почта").fill("anna@example.com");
    await p.getByLabel("Пароль").fill("password-anna");
    await p.getByRole("button", { name: "Войти" }).click();
    await p.getByPlaceholder("Что у вас нового?").waitFor();
    await p.goto(base + "/u/anna");
    const video = p.locator("figure.video video").first();
    await video.waitFor({ timeout: 10000 }).catch(async (e) => {
      await p.screenshot({ path: `${shots}/failure-video-${hls}.png`, fullPage: true });
      throw e;
    });
    const sources = await video.locator("source").evaluateAll((els) => els.map((e) => e.getAttribute("type")));
    const played = await video.evaluate(async (v) => {
      v.muted = true;
      await v.play();
      await new Promise((res) => setTimeout(res, 1500));
      return { t: v.currentTime, src: v.currentSrc, err: v.error && v.error.code };
    });
    await ctx.close();
    return { sources, ...played };
  }
  const noHls = await plays(false);
  assert.deepEqual(noHls.sources, ["video/mp4"], "without HLS only MP4 source is offered");
  assert.ok(noHls.t > 0.5 && noHls.src.endsWith(`/video/${vid}/720p.mp4`), `MP4 did not play: ${JSON.stringify(noHls)}`);
  const withHls = await plays(true);
  assert.ok(withHls.t > 0.5 && withHls.src.endsWith("master.m3u8"), `HLS did not play: ${JSON.stringify(withHls)}`);
  console.log("video ok", JSON.stringify({ noHls, withHls }));
}

// телефон
const phone = await anna.ctx.browser().newContext({ viewport: { width: 390, height: 844 }, locale: "ru-RU",
  permissions: ["geolocation"], geolocation: { latitude: 55.06, longitude: 60.1, accuracy: 8 } });
const mp = await phone.newPage();
await mp.goto(base + "/");
await mp.getByRole("button", { name: "Войти" }).first().waitFor();
await mp.getByLabel("Почта").fill("bob@example.com");
await mp.getByLabel("Пароль").fill("password-bob");
await mp.getByRole("button", { name: "Войти" }).click();
await mp.getByPlaceholder("Что у вас нового?").waitFor();
// что вылезает за экран: так причина видна сразу, даже если шрифты на машине другие
const wide = async (page) => page.evaluate(() => [...document.querySelectorAll("body *")]
  .filter((el) => el.getBoundingClientRect().right > innerWidth + 1 && getComputedStyle(el).position !== "fixed")
  .slice(0, 8).map((el) => `${el.tagName.toLowerCase()}.${el.className} → ${Math.round(el.getBoundingClientRect().right)}px`));
for (const width of [390, 320]) { // 320 — самые узкие телефоны и запас на более широкие шрифты
  await mp.setViewportSize({ width, height: 844 });
  const overflow = await mp.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1);
  assert.equal(overflow, false, `horizontal scroll on a ${width}px phone: ${(await wide(mp)).join("; ")}`);
}
await mp.setViewportSize({ width: 390, height: 844 });
await mp.screenshot({ path: `${shots}/06-phone.png`, fullPage: true });
// телефон: нижняя панель из пяти разделов, остальное — в «Ещё» по группам
assert.equal(await mp.locator(".tabbar > *").count(), 5, "five tabs on a phone");
await mp.locator(".tabbar").getByRole("button", { name: "Ещё" }).click();
await mp.locator(".more-sheet").getByRole("heading", { name: "Походы" }).waitFor();
await mp.screenshot({ path: `${shots}/06b-phone-more.png` });
await mp.locator(".more-sheet").getByRole("link", { name: "Попутчики" }).click();
await mp.getByRole("heading", { name: "Ищу попутчиков" }).waitFor();
assert.equal(await mp.locator(".more-sheet").count(), 0, "sheet closes after navigation");

// запись трека: GPS подменяет браузер; пауза, сохранение маршрутом, публикация
{
  await mp.locator(".tabbar").getByRole("button", { name: "Ещё" }).click();
  await mp.locator(".more-sheet").getByRole("link", { name: "Записать трек" }).click();
  await mp.getByRole("button", { name: "Начать запись" }).click();
  for (let i = 1; i <= 6; i++) {
    await mp.waitForTimeout(400); // не быстрее 60 м/с — иначе это скачок GPS
    await phone.setGeolocation({ latitude: 55.06 + i * 0.0001, longitude: 60.1, accuracy: 8 });
  }
  await mp.waitForFunction(() => +document.querySelector("[data-stat=points]")?.textContent >= 5);
  await phone.setGeolocation({ latitude: 55.07, longitude: 60.1, accuracy: 300 }); // плохая точность — мимо
  await mp.getByRole("button", { name: "Пауза" }).click();
  await mp.getByText("Пауза — точки не пишутся.").waitFor();
  await mp.screenshot({ path: `${shots}/22-record.png` });
  // черновик переживает перезагрузку страницы
  await mp.reload();
  await mp.getByText(/Есть незавершённая запись/).waitFor();
  await mp.getByRole("button", { name: "Завершить" }).click();
  await mp.getByLabel("Название").fill("Утро на Таганае");
  await mp.getByRole("button", { name: "Сохранить маршрут" }).click();
  await mp.getByRole("heading", { name: "Маршрут сохранён" }).waitFor();
  await mp.getByLabel("Рассказать в ленте").fill("Прогулка до родника");
  await mp.getByRole("button", { name: "Опубликовать" }).click();
  await mp.getByRole("link", { name: "открыть запись" }).click();
  await mp.locator("article.post", { hasText: "Прогулка до родника" }).getByText("Утро на Таганае").waitFor();
  assert.equal(await mp.evaluate(() => localStorage.getItem("sn-track-draft")), null, "draft cleared after save");
}

// тёмная тема
const dark = await browser.newContext({ viewport: { width: 1280, height: 860 }, colorScheme: "dark" });
const dp = await dark.newPage();
await dp.goto(base + "/");
await dp.screenshot({ path: `${shots}/07-login-dark.png` });

// Layout audit of every top-level section at desktop and mobile widths.
{
 const p=anna.page;
 await p.goto(base+'/documents');
 await p.getByLabel('Загрузить файл',{exact:true}).setInputFiles([{name:'Reference.txt',mimeType:'text/plain',buffer:Buffer.from('Document search test')},{name:'Pixel.gif',mimeType:'image/gif',buffer:Buffer.from('R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==','base64')}]);
 await p.getByRole('button',{name:'Reference.txt',exact:true}).waitFor();
 await p.getByRole('button',{name:'GIF-изображения',exact:true}).click();
 await p.getByRole('button',{name:'Pixel.gif',exact:true}).waitFor();
 assert.equal(await p.getByRole('button',{name:'Reference.txt',exact:true}).count(),0);
 await p.getByRole('button',{name:'Все файлы',exact:true}).click();
 await p.getByLabel('Поиск файлов').fill('Reference');
 await p.getByRole('button',{name:'Reference.txt',exact:true}).waitFor();
 await p.getByRole('button',{name:'Pixel.gif',exact:true}).waitFor({state:'detached'});
 const downloaded=p.waitForEvent('download');
 await p.getByRole('button',{name:'Reference.txt',exact:true}).click();
 assert.equal((await downloaded).suggestedFilename(),'Reference.txt');
 await p.goto(base+'/developers?tab=api');
 await p.getByLabel('Поиск API').fill('/calls/config');
 await p.locator('.api-method').first().waitFor();
 assert.equal(await p.locator('.api-method').count(),1);
 await p.goto(base+'/developers?tab=docs');
 await p.getByRole('button',{name:'Авторизация OAuth',exact:true}).click();
 await p.getByRole('heading',{name:'Авторизация OAuth',exact:true}).waitFor();
 await p.goto(base+'/developers?tab=apps');
 await p.getByRole('button',{name:'Создать приложение',exact:true}).click();
 const form=p.getByRole('dialog',{name:'Создать приложение',exact:true});
 await form.getByLabel('Название приложения').fill('Browser test application');
 await form.getByLabel('Redirect URI, по одному на строку').fill('https://example.org/callback');
 await form.getByRole('button',{name:'Создать',exact:true}).click();
 await p.getByRole('heading',{name:'Browser test application',exact:true}).waitFor();
 await p.reload();
 await p.getByRole('heading',{name:'Browser test application',exact:true}).waitFor();
 await p.locator('.developer-app').filter({hasText:'Browser test application'}).getByRole('button',{name:'Удалить приложение',exact:true}).click();
 await p.getByRole('heading',{name:'Browser test application',exact:true}).waitFor({state:'detached'});
}

{
  const p=anna.page;
  await p.goto(base+'/help');
  await p.goto(base+'/statistics');
  await p.getByRole('heading',{name:'Моя статистика',exact:true}).waitFor();
  await p.locator('.stats-table').waitFor();
  const statsDownload=p.waitForEvent('download');
  await p.getByRole('button',{name:'Экспорт CSV',exact:true}).click();
  assert.match((await statsDownload).suggestedFilename(),/socialnet-post.*csv/);
  await p.getByRole('button',{name:'Истории',exact:true}).click();
  await p.getByLabel('Состояние историй').selectOption('archive');
  await p.getByRole('button',{name:'Аудитория',exact:true}).click();
  await p.getByText('Демография и устройства зрителей пока не собираются. Данных для диаграмм аудитории нет.').waitFor();
  await p.goto(base+'/help/comments');
  const commentsDownload=p.waitForEvent('download');
  await p.getByRole('button',{name:'Скачать мои комментарии',exact:true}).click();
  assert.equal((await commentsDownload).suggestedFilename(),'socialnet-my-comments.csv');
  await p.goto(base+'/help');
  await p.getByLabel('Поиск по справке').fill('аудиофайлы');
  await p.getByText('Какие аудиофайлы можно загрузить?',{exact:true}).click();
  await p.locator('.help-question[open]').getByRole('link',{name:'Открыть музыку'}).waitFor();
  assert.equal(await p.locator('.help-question').count(),1);
  await p.getByLabel('Поиск по справке').fill('ничегонет12345');
  await p.getByText('Ничего не найдено. Попробуйте другое слово или выберите все темы.').waitFor();
  await p.goto(base+'/legal-info');
  await p.locator('.info-links a').first().waitFor();
  for(const a of await p.locator('.info-links a').all()) {
    const response=await p.request.get(new URL(await a.getAttribute('href'),base).href);
    assert.equal(response.status(),200,'legal document link');
  }
  const footer=p.locator('.sidenav .sidebar-footer');
  await footer.locator('summary').click();
  await footer.getByRole('link',{name:'О socialnet',exact:true}).click();
  await p.getByRole('heading',{name:'Общение начинается с интереса',exact:true}).waitFor();
}
{
  const p=anna.page;
  await p.goto(base+'/settings');
  await p.getByRole('button',{name:'Настроить пункты меню'}).click();
  const menu=p.getByRole('dialog',{name:'Настройка пунктов меню'});
  await menu.getByLabel('Музыка',{exact:true}).uncheck();
  await menu.getByRole('button',{name:'Сохранить',exact:true}).click();
  assert.equal(await p.locator('.sidenav a[href="/music"]').count(),0);
  await p.reload();
  await p.getByRole('button',{name:'Настроить пункты меню'}).click();
  assert.equal(await menu.getByLabel('Музыка',{exact:true}).isChecked(),false);
  await menu.getByLabel('Музыка',{exact:true}).check();
  await menu.getByRole('button',{name:'Сохранить',exact:true}).click();
  await p.locator('.sidenav a[href="/music"]').waitFor();
  await p.goto(base+'/settings?tab=privacy');
  await p.getByLabel('Кто видит моих друзей').selectOption('friends');
  await p.getByRole('button',{name:'Сохранить',exact:true}).click();
  await p.getByRole('status').filter({hasText:'Настройки сохранены.'}).waitFor();
  await p.reload();
  await p.getByLabel('Кто видит моих друзей').waitFor();
  assert.equal(await p.getByLabel('Кто видит моих друзей').inputValue(),'friends');
  await p.goto(base+'/settings?tab=notifications');
  await p.getByLabel('Сводка на электронную почту').uncheck();
  await p.getByRole('button',{name:'Сохранить',exact:true}).click();
  await p.getByRole('status').filter({hasText:'Настройки сохранены.'}).waitFor();
  await p.reload();
  await p.getByLabel('Сводка на электронную почту').waitFor();
  assert.equal(await p.getByLabel('Сводка на электронную почту').isChecked(),false);
  await p.goto(base+'/settings?tab=blocks');
  await p.getByRole('button',{name:'Добавить в чёрный список',exact:true}).click();
  const blocks=p.getByRole('dialog',{name:'Добавление в чёрный список'});
  await blocks.getByLabel('Имя или имя пользователя').fill('bob');
  await blocks.getByRole('button',{name:'Заблокировать',exact:true}).click();
  await p.getByRole('button',{name:'Разблокировать',exact:true}).click();
  await p.getByText('Ваш чёрный список пуст.').waitFor();
  await p.goto(base+'/settings?tab=security');
  await p.locator('.security-factor').getByRole('button',{name:'Настроить',exact:true}).click();
  const factor=p.getByRole('dialog',{name:'Двухфакторная аутентификация'});
  await factor.getByLabel('Текущий пароль').fill('password-anna');
  await factor.getByRole('button',{name:'Продолжить',exact:true}).click();
  await factor.locator('.totp-secret').waitFor();
  const secret=await factor.locator('.totp-secret').textContent();
  const alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';let bits='';for(const c of secret)bits+=alphabet.indexOf(c).toString(2).padStart(5,'0');
  const key=Buffer.from((bits.match(/.{8}/g)||[]).map(b=>parseInt(b,2)));const counter=Buffer.alloc(8);counter.writeBigUInt64BE(BigInt(Math.floor(Date.now()/30000)));
  const {createHmac}=await import('node:crypto');const digest=createHmac('sha1',key).update(counter).digest();const offset=digest[digest.length-1]&15;const otp=String((digest.readUInt32BE(offset)&0x7fffffff)%1000000).padStart(6,'0');
  await factor.getByLabel('Одноразовый код').fill(otp);
  await factor.getByRole('button',{name:'Включить',exact:true}).click();
  await factor.locator('.backup-codes').waitFor();
  const backup=(await factor.locator('.backup-codes').textContent()).split('\n')[0];
  await factor.getByRole('button',{name:'Я сохранил коды'}).click();
  await p.getByText('Двухфакторная защита включена',{exact:true}).waitFor();
  await p.evaluate(async backup=>{const api=await import('/static/app/api.js');await api.post('/users/me/2fa/disable',{password:'password-anna',backup_code:backup});},backup);
  await p.reload();await p.getByText('Защиту можно усилить',{exact:true}).waitFor();
}
{
  await anna.page.getByRole('button',{name:'Открыть уведомления',exact:true}).click();
  const panel=anna.page.getByRole('region',{name:'Панель уведомлений'});
  await panel.getByText('Показать все',{exact:true}).waitFor();
  await panel.getByLabel('Источники уведомлений').selectOption('social');
  await panel.screenshot({path:`${shots}/notifications-panel.png`});
  await anna.page.keyboard.press('Escape');
  await panel.waitFor({state:'detached'});
}
{
  const p=anna.page;
  await p.goto(base+"/photos");
  const image=await p.locator('.brand-mark').screenshot();
  await p.locator('.photo-library input[type=file]').setInputFiles([{name:'one.png',mimeType:'image/png',buffer:image},{name:'two.png',mimeType:'image/png',buffer:image}]);
  await p.getByRole('status').filter({hasText:'Загружено фотографий: 2'}).waitFor();
  await p.locator('.photo-menu summary').click();
  await p.getByRole('button',{name:'Создать альбом',exact:true}).click();
  await p.getByLabel('Название альбома').fill('Поход 2026');
  await p.getByRole('dialog',{name:'Создать альбом'}).getByRole('button',{name:'Создать',exact:true}).click();
  await p.getByRole('heading',{name:'Поход 2026'}).waitFor();
  await p.getByRole('button',{name:'Фото',exact:true}).click();
  await p.locator('.photo-tile').first().waitFor();
  await p.locator('.photo-menu summary').click();
  await p.getByRole('button',{name:'Выбрать несколько'}).click();
  await p.locator('.photo-tile').first().click();
  await p.getByLabel('В альбом').selectOption({label:'Поход 2026'});
  await p.getByRole('button',{name:'Альбомы',exact:true}).click();
  await p.locator('.album-tile').filter({hasText:'Поход 2026'}).click();
  await p.locator('.photo-tile').click();
  await p.getByRole('dialog',{name:'Просмотр фотографии'}).waitFor();
  const viewer=p.getByRole('dialog',{name:'Просмотр фотографии'});
  await viewer.getByRole('button',{name:'Редактировать описание'}).click();
  await viewer.getByLabel('Описание фотографии').fill('Снимок из похода');
  await viewer.getByRole('button',{name:'Сохранить',exact:true}).click();
  await viewer.getByText('Снимок из похода',{exact:true}).waitFor();
  await viewer.screenshot({path:`${shots}/photo-viewer-desktop.png`});
  await p.setViewportSize({width:390,height:844});
  assert.ok(await viewer.evaluate(el=>el.scrollWidth<=el.clientWidth+1),'photo viewer mobile overflow');
  await viewer.screenshot({path:`${shots}/photo-viewer-mobile.png`});
  await p.setViewportSize({width:1280,height:860});
  await p.keyboard.press('Escape');
  await p.locator('.photo-menu summary').click();
  await p.getByRole('button',{name:'Выбрать несколько'}).click();
  await p.locator('.photo-tile').click();
  await p.getByRole('button',{name:'В архив',exact:true}).click();
  await p.getByText('Загрузите фотографии — они появятся здесь.').waitFor();
  await p.locator('.photo-menu summary').click();
  await p.getByRole('button',{name:'Архив',exact:true}).click();
  await p.locator('.photo-tile').waitFor();
  await p.reload();
  await p.locator('.photo-menu summary').click();
  await p.getByRole('button',{name:'Архив',exact:true}).click();
  await p.locator('.photo-menu summary').click();
  await p.getByRole('button',{name:'Выбрать несколько'}).click();
  await p.locator('.photo-tile').click();
  await p.getByRole('button',{name:'Восстановить',exact:true}).click();
  await p.getByText('В архиве пока нет фотографий.').waitFor();
  await p.screenshot({path:`${shots}/gallery-archive.png`});
  await p.getByRole('button',{name:'Фото',exact:true}).click();
  await p.getByRole('button',{name:'Настройки вкладки Фото',exact:true}).click();
  const settings=p.getByRole('dialog',{name:'Настройки вкладки Фото'});
  await settings.screenshot({path:`${shots}/gallery-settings.png`});
  for(const checkbox of await settings.getByRole('checkbox').all()) await checkbox.uncheck();
  await settings.getByRole('button',{name:'Сохранить',exact:true}).click();
  await p.getByText('Нет фото из выбранных альбомов. Измените настройки вкладки.').waitFor();
  await p.reload();
  await p.getByText('Нет фото из выбранных альбомов. Измените настройки вкладки.').waitFor();
  await p.getByRole('button',{name:'Настройки вкладки Фото',exact:true}).click();
  await settings.getByRole('checkbox').first().check();
  await settings.getByRole('button',{name:'Отмена',exact:true}).click();
  assert.equal(await p.locator('.photo-tile').count(),0,'cancel must not change visible albums');
  await p.getByRole('button',{name:'Настройки вкладки Фото',exact:true}).click();
  for(const checkbox of await settings.getByRole('checkbox').all()) await checkbox.check();
  await settings.getByRole('button',{name:'Сохранить',exact:true}).click();
  await p.locator('.photo-tile').first().waitFor();
}
const auditPaths = ["/developers", "/developers?tab=docs", "/developers?tab=api", "/developers?tab=apps", "/developers?tab=rules", "/calls", "/statistics", "/help/comments", "/help", "/about", "/authors", "/business", "/safety", "/legal-info", "/settings?tab=privacy", "/settings?tab=notifications", "/settings?tab=blocks", "/settings?tab=security", "/settings?tab=wallet", "/settings?tab=plus", "/photos", "/", "/u/anna", "/im", "/notifications", "/communities", "/hikes", "/hikes/new", "/companions", "/rides", "/articles", "/articles/new", "/feeds", "/plus", "/fundraisers", "/offline", "/music", "/videos", "/wallet", "/settings", "/search?q=test", "/bookmarks", "/documents", "/scheduled", "/admin", "/admin?tab=roles", "/admin?tab=system", "/admin?tab=users", "/admin?tab=audit"];
const layoutProblems = [];
for (const width of [1280, 390]) {
  await anna.page.setViewportSize({ width, height: 860 });
  for (const [i, route] of auditPaths.entries()) {
    await anna.page.goto(base + route);
    await anna.page.locator('#main').waitFor();
    await anna.page.waitForFunction(() => !document.querySelector('#main .loading'), { timeout: 10000 }).catch(() => {});
    const issues = await anna.page.evaluate(() => ({
      overflow: document.documentElement.scrollWidth > innerWidth + 1,
      wide: [...document.querySelectorAll('#main *')].filter(e => { const r = e.getBoundingClientRect(); return r.width > 0 && r.right > innerWidth + 1 && getComputedStyle(e).position !== 'absolute'; }).slice(0, 5).map(e => e.className || e.tagName),
    }));
    if (issues.overflow) layoutProblems.push(`${width} ${route}: ${JSON.stringify(issues.wide)}`);
    await anna.page.screenshot({ path: `${shots}/audit-${width}-${i}.png` });
  }
}
assert.deepEqual(layoutProblems, [], 'page overflow');
console.log(`layout audit: ${auditPaths.length * 2} screens`);
await browser.close();
if (problems.length) { console.error(problems.join("\n")); process.exit(1); }
console.log("e2e ok");
