// Офлайн: сохранённые маршруты с картой — открываются без сети и без входа
// (данные лежат в браузере этого устройства).
import { Fragment, h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { Route } from "./api.js";
import { removeOffline, savedRoutes } from "./offline.js";
import { coords } from "./print.js";
import { ElevationProfile, km, RouteMap } from "./routes.js";
import { Empty, Loading } from "./ui.js";

export function OfflinePage() {
  const [list, setList] = useState<{ route: Route; saved_at: string }[] | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const load = () => savedRoutes().then(setList, () => setList([]));
  useEffect(() => { load(); }, []);
  if (!list) return <Loading />;
  return (
    <section class="offline">
      <h1>Маршруты офлайн</h1>
      {!navigator.onLine && <p class="note">Нет связи. Здесь — маршруты, сохранённые на этом устройстве.</p>}
      {list.length === 0 ? <Empty>Сохранённых маршрутов нет. Откройте маршрут при связи и нажмите «Сохранить офлайн».</Empty> : (
        <ul class="ads">
          {list.map(({ route: r, saved_at }) => (
            <li key={r.id} class="ad">
              <div class="route-head">
                <strong>{r.title}</strong>
                <span class="route-actions">
                  <button class="link" onClick={() => setOpen(open === r.id ? null : r.id)}>{open === r.id ? "Скрыть" : "Открыть"}</button>
                  <button class="link" onClick={() => removeOffline(r.id).then(load)}>Удалить с устройства</button>
                </span>
              </div>
              <p class="muted">{km(r.distance_m)} · ↑ {r.gain_m} м · сохранён {new Date(saved_at).toLocaleDateString("ru")}</p>
              {open === r.id && <>
                <RouteMap route={r} />
                {r.profile.length > 1 && <ElevationProfile profile={r.profile} />}
                {(r.waypoints ?? []).length > 0 && <ul class="plain-list">
                  {r.waypoints!.map((w, i) => <li key={i}><strong>{w.name || `Точка ${i + 1}`}</strong> — {coords(w.lat, w.lon)}</li>)}
                </ul>}
              </>}
            </li>
          ))}
        </ul>
      )}
      <p class="muted">Экстренная связь — 112. Сохранённые маршруты стираются при выходе из аккаунта.</p>
    </section>
  );
}
