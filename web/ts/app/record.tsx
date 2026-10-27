// /record — запись трека телефоном: старт, пауза, финиш, сохранение как
// маршрут (тот же путь, что загрузка GPX) и публикация в ленте.
import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Route, post, upload } from "./api.js";
import { RouteCard, hours } from "./routes.js";
import {
  Draft, Fix, addFix, backgroundCapable, distance, elapsed, loadDraft, newDraft, pause, pointCount,
  positionSource, resume, saveDraft, toGPX,
} from "./track.js";
import { ErrorLine } from "./ui.js";

interface WakeLockSentinel { release(): Promise<void> }
interface WakeLockNav { wakeLock?: { request(type: "screen"): Promise<WakeLockSentinel> } }

const dayName = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long" });
const km = (m: number) => (m < 1000 ? `${Math.round(m)} м` : `${(m / 1000).toFixed(m < 10000 ? 2 : 1)} км`);
const clock = (ms: number) => {
  const s = Math.floor(ms / 1000);
  return `${Math.floor(s / 3600)}:${String(Math.floor(s / 60) % 60).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
};

export function RecordPage() {
  const [draft, setDraftState] = useState<Draft | null>(() => loadDraft());
  const [recording, setRecording] = useState(false); // идёт ли приём точек в этой вкладке
  const [finishing, setFinishing] = useState(false);
  const [last, setLast] = useState<Fix | null>(null);
  const [gpsError, setGpsError] = useState("");
  const [, tick] = useState(0);
  const [saved, setSaved] = useState<Route | null>(null);
  const draftRef = useRef(draft);

  const setDraft = (d: Draft | null) => { draftRef.current = d; saveDraft(d); setDraftState(d); };

  // приём точек, пока запись идёт и не на паузе
  useEffect(() => {
    if (!recording) return;
    const stop = positionSource()((f) => {
      setGpsError(""); setLast(f);
      const cur = draftRef.current;
      if (cur) { const next = addFix(cur, f); if (next !== cur) setDraft(next); }
    }, setGpsError);
    const timer = setInterval(() => tick((n) => n + 1), 1000);
    // экран не гаснет — иначе браузер перестаёт получать координаты
    let lock: WakeLockSentinel | null = null;
    const wl = (navigator as unknown as WakeLockNav).wakeLock;
    const grab = () => { if (wl && document.visibilityState === "visible") wl.request("screen").then((l) => { lock = l; }, () => {}); };
    grab();
    document.addEventListener("visibilitychange", grab);
    return () => { stop(); clearInterval(timer); document.removeEventListener("visibilitychange", grab); lock?.release().catch(() => {}); };
  }, [recording]);

  const start = () => { setSaved(null); setDraft(newDraft()); setRecording(true); };
  const togglePause = () => { if (draft) setDraft(draft.paused ? resume(draft) : pause(draft)); };
  const continueDraft = () => { if (draft) { setDraft(resume(draft)); setRecording(true); } };
  const finish = () => { if (draft) setDraft(pause(draft)); setRecording(false); setFinishing(true); };
  const discard = () => { if (confirm("Удалить запись без сохранения?")) { setDraft(null); setFinishing(false); setRecording(false); } };

  if (saved) return <Saved route={saved} onNew={() => setSaved(null)} />;
  if (finishing && draft) return <FinishForm draft={draft} onBack={() => { setFinishing(false); continueDraft(); }} onDiscard={discard}
    onSaved={(r) => { setDraft(null); setFinishing(false); setSaved(r); }} />;

  const dist = draft ? distance(draft.segments) : 0;
  return (
    <section class="record">
      <h1>Записать трек</h1>
      {!draft && <>
        <p>Телефон запишет путь по GPS. Потом трек станет маршрутом: с профилем высот, для похода, статьи или записи в ленте.</p>
        {!backgroundCapable && <p class="note">Держите экран включённым и страницу открытой: браузер не пишет трек в фоне. С выключенным экраном пишет приложение для Android.</p>}
        <button class="primary big" onClick={start}>Начать запись</button>
      </>}
      {draft && !recording && <>
        <p class="note">Есть незавершённая запись от {dayName.format(new Date(draft.startedAt))}: {km(dist)}, точек {pointCount(draft)}.</p>
        <div class="row">
          <button class="primary" onClick={continueDraft}>Продолжить</button>
          <button class="ghost" onClick={() => setFinishing(true)} disabled={pointCount(draft) < 2}>Завершить</button>
          <button class="ghost danger" onClick={discard}>Удалить</button>
        </div>
      </>}
      {draft && recording && <>
        <div class={draft.paused ? "rec-stats paused" : "rec-stats"} aria-live="polite">
          <div><span class="rec-num" data-stat="distance">{km(dist)}</span><span class="muted">пройдено</span></div>
          <div><span class="rec-num">{clock(elapsed(draft))}</span><span class="muted">в пути</span></div>
          <div><span class="rec-num" data-stat="points">{pointCount(draft)}</span><span class="muted">точек</span></div>
          <div><span class="rec-num">{last ? `±${Math.round(last.acc)} м` : "…"}</span><span class="muted">точность</span></div>
        </div>
        <p class="muted" role="status">{draft.paused ? "Пауза — точки не пишутся." : last ? (last.ele !== null ? `Высота ${Math.round(last.ele)} м.` : "Запись идёт.") : "Ищем спутники…"}</p>
        {gpsError && <p class="error" role="alert">{gpsError}</p>}
        <div class="row">
          <button class="ghost big" onClick={togglePause}>{draft.paused ? "Продолжить" : "Пауза"}</button>
          <button class="primary big" onClick={finish} disabled={pointCount(draft) < 2}>Завершить</button>
        </div>
      </>}
    </section>
  );
}

function FinishForm({ draft, onBack, onDiscard, onSaved }: { draft: Draft; onBack: () => void; onDiscard: () => void; onSaved: (r: Route) => void }) {
  const [name, setName] = useState(`Трек ${dayName.format(new Date(draft.startedAt))}`);
  const dist = distance(draft.segments);
  // скрыть можно только меньше половины пути — иначе от трека ничего не останется
  const fits = (m: number) => m === 0 || dist > 2 * m + 50;
  const [hideEnds, setHideEnds] = useState(fits(200) ? "200" : "0");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  async function save(e: Event) {
    e.preventDefault();
    setBusy(true); setError(null);
    try {
      const file = new File([toGPX(draft, name.trim() || "Трек")], "track.gpx", { type: "application/gpx+xml" });
      onSaved(await upload<Route>("/routes", file, { hidden_ends_m: hideEnds }));
    } catch (err) { setError(err); } finally { setBusy(false); }
  }
  return (
    <form class="stack record" onSubmit={save}>
      <h1>Сохранить трек</h1>
      <p>{km(dist)} · {clock(elapsed(draft))} · точек {pointCount(draft)}</p>
      <label>Название<input required maxLength={200} value={name} onInput={(e) => setName((e.target as HTMLInputElement).value)} /></label>
      <label>Начало и конец
        <select value={hideEnds} onChange={(e) => setHideEnds((e.target as HTMLSelectElement).value)}>
          {([[0, "видны всем"], [200, "скрыть 200 м"], [500, "скрыть 500 м"], [1000, "скрыть 1 км"]] as const).map(([m, label]) =>
            <option key={m} value={String(m)} disabled={!fits(m)}>{label}</option>)}
        </select>
      </label>
      <p class="muted">Скрытые концы не видны другим, чтобы по треку нельзя было найти ваш дом.{!fits(200) && " Трек короткий — скрыть концы не получится."}</p>
      <ErrorLine error={error} />
      <div class="row">
        <button class="primary" disabled={busy}>{busy ? "Сохраняем…" : "Сохранить маршрут"}</button>
        <button type="button" class="ghost" onClick={onBack}>Продолжить запись</button>
        <button type="button" class="ghost danger" onClick={onDiscard}>Удалить</button>
      </div>
    </form>
  );
}

function Saved({ route, onNew }: { route: Route; onNew: () => void }) {
  const [text, setText] = useState("");
  const [audience, setAudience] = useState("friends");
  const [posted, setPosted] = useState<number | null>(null);
  const [error, setError] = useState<unknown>(null);
  async function publish(e: Event) {
    e.preventDefault();
    setError(null);
    try { setPosted((await post<{ id: number }>("/posts", { text, route_ids: [route.id], audience })).id); } catch (err) { setError(err); }
  }
  return (
    <section class="record">
      <h1>Маршрут сохранён</h1>
      <p class="muted">{route.title} · в пути {hours(route.duration_sec)}</p>
      <RouteCard route={route} />
      {posted ? <p>Опубликовано — <a href={`/p/${posted}`}>открыть запись</a>.</p> : (
        <form class="stack" onSubmit={publish}>
          <label>Рассказать в ленте<textarea rows={3} maxLength={5000} value={text} placeholder="Как прошли, что видели" onInput={(e) => setText((e.target as HTMLTextAreaElement).value)} /></label>
          <label>Кто видит
            <select value={audience} onChange={(e) => setAudience((e.target as HTMLSelectElement).value)}>
              <option value="everyone">все</option><option value="friends">друзья</option><option value="only_me">только я</option>
            </select>
          </label>
          <ErrorLine error={error} />
          <div class="row"><button class="primary">Опубликовать</button><button type="button" class="ghost" onClick={onNew}>Записать ещё</button></div>
        </form>
      )}
    </section>
  );
}
