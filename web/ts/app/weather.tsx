// Прогноз на маршруте: на высоте вершины, с предупреждениями о грозе,
// сильном ветре и осадках.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { get } from "./api.js";
import { ErrorLine, Loading, useLoad } from "./ui.js";

interface Day { date: string; code: number; t_max: number | null; t_min: number | null; precip_mm: number | null; precip_prob: number | null; wind_max: number | null; gust_max: number | null }
interface Report { elevation: number | null; days: Day[]; source: string; fetched_at: string }

// WMO-коды погоды → короткая подпись и значок
function sky(code: number): [string, string] {
  if (code === 0) return ["☀️", "Ясно"];
  if (code <= 2) return ["🌤️", "Переменная облачность"];
  if (code === 3) return ["☁️", "Пасмурно"];
  if (code === 45 || code === 48) return ["🌫️", "Туман"];
  if (code >= 51 && code <= 57) return ["🌦️", "Морось"];
  if (code >= 61 && code <= 67) return ["🌧️", "Дождь"];
  if (code >= 71 && code <= 77) return ["🌨️", "Снег"];
  if (code >= 80 && code <= 82) return ["🌧️", "Ливень"];
  if (code === 85 || code === 86) return ["🌨️", "Снегопад"];
  if (code >= 95) return ["⛈️", "Гроза"];
  return ["·", "—"];
}

export function warnings(d: Day): string[] {
  const w: string[] = [];
  if (d.code >= 95) w.push("гроза — уходите с гребней");
  if ((d.gust_max ?? 0) >= 17) w.push(`порывы до ${Math.round(d.gust_max!)} м/с`);
  if ((d.precip_mm ?? 0) >= 15) w.push(`осадки ${Math.round(d.precip_mm!)} мм — броды поднимутся`);
  if ((d.t_min ?? 99) <= -15) w.push(`мороз до ${Math.round(d.t_min!)}°`);
  return w;
}

const dayFmt = new Intl.DateTimeFormat("ru", { weekday: "short", day: "numeric", month: "short" });
const t = (v: number | null) => (v === null ? "—" : `${v > 0 ? "+" : ""}${Math.round(v)}°`);

export function Weather({ lat, lon, elevation, from, to }: { lat: number; lon: number; elevation?: number | null; from?: string; to?: string }) {
  const q = `/weather?lat=${lat.toFixed(4)}&lon=${lon.toFixed(4)}${elevation != null ? `&elevation=${Math.round(elevation)}` : ""}`;
  const rep = useLoad(() => get<Report>(q), [q]);
  if (rep.error) return <ErrorLine error={rep.error} />;
  if (!rep.data) return <Loading />;
  const r = rep.data;
  return (
    <section class="weather" aria-label="Прогноз погоды">
      <p class="muted">Прогноз{r.elevation != null ? ` на высоте ${Math.round(r.elevation)} м` : ""}. Данные: {r.source}. В горах погода меняется быстрее прогноза.</p>
      <ol class="weather-days">
        {r.days.map((d) => {
          const [icon, text] = sky(d.code), warn = warnings(d), inHike = from && d.date >= from && d.date <= (to ?? from);
          return (
            <li key={d.date} class={`${inHike ? "hike-day" : ""} ${warn.length ? "warn" : ""}`}>
              <span class="wd-date">{dayFmt.format(new Date(d.date + "T12:00:00"))}</span>
              <span class="wd-icon" title={text} aria-label={text}>{icon}</span>
              <span class="wd-temp"><strong>{t(d.t_max)}</strong> / {t(d.t_min)}</span>
              <span class="wd-more">{d.precip_mm ? `${d.precip_mm.toFixed(1)} мм` : "без осадков"}{d.precip_prob != null ? ` · ${Math.round(d.precip_prob)}%` : ""}</span>
              <span class="wd-more">ветер {d.wind_max != null ? Math.round(d.wind_max) : "—"}{d.gust_max != null ? `, порывы ${Math.round(d.gust_max)}` : ""} м/с</span>
              {warn.map((x) => <span key={x} class="wd-warn">⚠ {x}</span>)}
            </li>
          );
        })}
      </ol>
    </section>
  );
}

// WeatherToggle — кнопка «Погода» для карточки маршрута.
export function WeatherToggle(props: { lat: number; lon: number; elevation?: number | null; from?: string; to?: string }) {
  const [open, setOpen] = useState(false);
  return (
    <div class="weather-toggle">
      <button class="link" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? "Скрыть погоду" : "Погода на маршруте"}</button>
      {open && <Weather {...props} />}
    </div>
  );
}
