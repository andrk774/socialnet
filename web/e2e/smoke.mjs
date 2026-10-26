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
const browser = await chromium.launch();
const problems = [];

async function newUser(name, viewport = { width: 1280, height: 860 }) {
  const ctx = await browser.newContext({ viewport, locale: "ru-RU", colorScheme: "light" });
  const page = await ctx.newPage();
  // ответы 4xx — часть сценария (вход без cookie, бонус без почты); всё прочее,
  // включая нарушения CSP («Refused to …»), считается ошибкой
  page.on("console", (m) => { if (m.type() === "error" && !m.text().startsWith("Failed to load resource")) problems.push(`${name}: console: ${m.text()}`); });
  page.on("pageerror", (e) => problems.push(`${name}: pageerror: ${e.message}`));
  page.on("requestfailed", (r) => { if (!r.url().includes("/api/v1/ws")) problems.push(`${name}: failed ${r.url()} ${r.failure()?.errorText}`); });
  await page.goto(base + "/");
  await page.getByRole("button", { name: "Зарегистрироваться" }).click();
  await page.getByLabel("Имя", { exact: true }).fill(name === "anna" ? "Анна" : "Борис");
  await page.getByLabel("Имя пользователя").fill(name);
  await page.getByLabel("Почта").fill(`${name}@example.com`);
  await page.getByLabel("Пароль").fill(`password-${name}`);
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page.getByPlaceholder("Что у вас нового?").waitFor();
  return { ctx, page };
}

const anna = await newUser("anna");
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
await first.getByRole("button", { name: /Комментарии/ }).click();
await first.getByPlaceholder("Написать комментарий").fill("Красота!");
await first.getByRole("button", { name: "Отправить", exact: true }).click();
await first.getByText("Красота!").waitFor();
await first.getByRole("button", { name: "Ответить" }).click();
await first.getByPlaceholder("Написать комментарий").fill("Анна, спасибо");
await first.getByRole("button", { name: "Отправить", exact: true }).click();
await first.getByText("Анна, спасибо").waitFor();
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
await anna.page.screenshot({ path: `${shots}/03-chat.png` });

// профиль, кошелёк, безопасность; после перезагрузки сеанс восстанавливается из cookie
await anna.page.goto(base + "/wallet");
await anna.page.getByText("Операций пока не было").waitFor();
// без подтверждённой почты бонус не дают — интерфейс объясняет почему
await anna.page.getByRole("button", { name: "Забрать ежедневный бонус" }).click();
await anna.page.getByRole("alert").filter({ hasText: "подтвердите почту" }).waitFor();
await anna.page.getByRole("button", { name: "Отправить письмо ещё раз" }).click();
await anna.page.getByText("Письмо отправлено").waitFor();
await anna.page.screenshot({ path: `${shots}/04-wallet.png`, fullPage: true });
await anna.page.goto(base + "/settings");
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
const phone = await anna.ctx.browser().newContext({ viewport: { width: 390, height: 844 }, locale: "ru-RU" });
const mp = await phone.newPage();
await mp.goto(base + "/");
await mp.getByRole("button", { name: "Войти" }).first().waitFor();
await mp.getByLabel("Почта").fill("bob@example.com");
await mp.getByLabel("Пароль").fill("password-bob");
await mp.getByRole("button", { name: "Войти" }).click();
await mp.getByPlaceholder("Что у вас нового?").waitFor();
const overflow = await mp.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1);
assert.equal(overflow, false, "horizontal scroll on phone");
await mp.screenshot({ path: `${shots}/06-phone.png`, fullPage: true });

// тёмная тема
const dark = await browser.newContext({ viewport: { width: 1280, height: 860 }, colorScheme: "dark" });
const dp = await dark.newPage();
await dp.goto(base + "/");
await dp.screenshot({ path: `${shots}/07-login-dark.png` });

await browser.close();
if (problems.length) { console.error(problems.join("\n")); process.exit(1); }
console.log("e2e ok");
