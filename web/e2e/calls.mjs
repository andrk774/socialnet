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
  const benignAbort = (r) => r.failure()?.errorText === "net::ERR_ABORTED" && (r.url().includes("/tiles/") || r.url().endsWith("/api/v1/auth/refresh"));
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
  if (name === "anna") await page.screenshot({ path: `${shots}/calls-register.png`, fullPage: true });
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await page.getByPlaceholder("Что у вас нового?").waitFor();
  return { ctx, page };
}

const anna = await newUser("anna");
const bob = await newUser("bob");
try {
  await anna.ctx.grantPermissions(['camera','microphone'],{origin:base});
  await bob.ctx.grantPermissions(['camera','microphone'],{origin:base});
  await anna.page.goto(base+'/calls');
  await anna.page.getByLabel('Поиск собеседника').fill('bob');
  await anna.page.locator('.call-history-row').getByRole('button',{name:'Видео',exact:true}).click();
  const incoming=bob.page.getByRole('dialog',{name:'Звонок',exact:true});
  await incoming.getByRole('button',{name:'Принять',exact:true}).click();
  await anna.page.getByText('Соединение установлено',{exact:true}).waitFor({timeout:45000});
  await bob.page.getByText('Соединение установлено',{exact:true}).waitFor({timeout:45000});
  for(const p of [anna.page,bob.page]) {
    await p.waitForFunction(()=>{const v=document.querySelector('.call-streams video');return v&&v.videoWidth>0&&v.currentTime>0;});
    await p.evaluate(()=>{window.testCallStreams=[...document.querySelectorAll('.call-streams video')].map(v=>v.srcObject);});
  }
  await anna.page.screenshot({path:`${shots}/call-connected.png`});
  await anna.page.getByRole('dialog',{name:'Звонок',exact:true}).getByRole('button',{name:'Выключить микрофон',exact:true}).click();
  await anna.page.getByRole('button',{name:'Включить микрофон',exact:true}).waitFor();
  assert.equal(await anna.page.evaluate(()=>window.testCallStreams[1].getAudioTracks().every(t=>!t.enabled)),true);
  await anna.page.getByRole('button',{name:'Выключить камеру',exact:true}).click();
  await anna.page.getByRole('button',{name:'Включить камеру',exact:true}).waitFor();
  assert.equal(await anna.page.evaluate(()=>window.testCallStreams[1].getVideoTracks().every(t=>!t.enabled)),true);
  await anna.page.setViewportSize({width:390,height:844});
  await anna.page.screenshot({path:`${shots}/call-mobile.png`});
  assert.equal(await anna.page.evaluate(()=>document.querySelector('.call-window').scrollWidth<=document.querySelector('.call-window').clientWidth+1),true);
  await anna.page.setViewportSize({width:1280,height:860});
  await anna.page.getByRole('dialog',{name:'Звонок',exact:true}).getByRole('button',{name:'Завершить',exact:true}).click();
  await incoming.waitFor({state:'detached'});
  for(const p of [anna.page,bob.page]) assert.equal(await p.evaluate(()=>window.testCallStreams[1].getTracks().every(t=>t.readyState==='ended')),true,'capture not released');
  await anna.page.getByLabel('Поиск собеседника').fill('bob');
  await anna.page.locator('header .call-history-row').getByRole('button',{name:'Позвонить',exact:true}).click();
  await incoming.getByRole('button',{name:'Принять',exact:true}).click();
  await anna.page.getByText('Соединение установлено',{exact:true}).waitFor({timeout:45000});
  await bob.page.getByText('Соединение установлено',{exact:true}).waitFor({timeout:45000});
  assert.equal(await anna.page.locator('.call-streams video').first().evaluate(v=>v.srcObject.getAudioTracks().length),1);
  assert.equal(await anna.page.locator('.call-streams video').first().evaluate(v=>v.srcObject.getVideoTracks().length),0);
  await anna.page.getByRole('dialog',{name:'Звонок',exact:true}).getByRole('button',{name:'Завершить',exact:true}).click();
  await incoming.waitFor({state:'detached'});
  await anna.page.locator('header .call-history-row').getByRole('button',{name:'Позвонить',exact:true}).click();
  await incoming.getByRole('button',{name:'Отклонить',exact:true}).click();
  await anna.page.getByRole('dialog',{name:'Звонок',exact:true}).waitFor({state:'detached'});
  await anna.page.getByText(/Отклонён/).first().waitFor();
}
catch(e){for(const [name,u] of [["anna",anna],["bob",bob]]) {await u.page.screenshot({path:`${shots}/calls-failure-${name}.png`}); console.error(name,await u.page.locator(".call-overlay").innerText().catch(()=>"no call overlay"));} console.error(problems);throw e;}
assert.deepEqual(problems,[]);
await browser.close();
console.log("Two-browser video call, mute, camera toggle, hangup and decline passed");
