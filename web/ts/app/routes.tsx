// Маршрут из GPX: сводка, схема трека и профиль высот. Рисуется в SVG без
// внешних библиотек и без запросов к картографическим серверам.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { Route, saveFile } from "./api.js";
import { ErrorLine } from "./ui.js";

const nf1 = new Intl.NumberFormat("ru", { maximumFractionDigits: 1 });
const nf0 = new Intl.NumberFormat("ru", { maximumFractionDigits: 0 });

export const km = (m: number) => (m < 1000 ? `${nf0.format(m)} м` : `${nf1.format(m / 1000)} км`);

export function hours(sec: number): string {
  const h = Math.floor(sec / 3600), m = Math.round((sec % 3600) / 60);
  return h > 0 ? `${h} ч${m ? ` ${m} мин` : ""}` : `${m} мин`;
}

export function RouteCard({ route }: { route: Route }) {
  const [error, setError] = useState<unknown>(null);
  const stats: [string, string][] = [["Расстояние", km(route.distance_m)]];
  if (route.max_ele !== null) {
    stats.push(["Набор", `↑ ${nf0.format(route.gain_m)} м`], ["Сброс", `↓ ${nf0.format(route.loss_m)} м`]);
    stats.push(["Высота", `${nf0.format(route.min_ele ?? 0)}–${nf0.format(route.max_ele)} м`]);
  }
  if (route.moving_sec > 0) stats.push(["В движении", hours(route.moving_sec)]);
  else if (route.duration_sec > 0) stats.push(["Время", hours(route.duration_sec)]);

  return (
    <section class="route" aria-label={`Маршрут: ${route.title}`}>
      <header class="route-head">
        <strong class="route-title">{route.title}</strong>
        <button class="link" onClick={() => saveFile(route.gpx_url, `${route.title}.gpx`).catch(setError)}>Скачать GPX</button>
      </header>
      <dl class="route-stats">
        {stats.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
      </dl>
      <div class="route-graphics">
        <TrackSketch points={route.preview} />
        {route.profile.length > 1 && <ElevationProfile profile={route.profile} />}
      </div>
      {route.hidden_ends_m > 0 && <p class="muted route-note">Первые и последние {route.hidden_ends_m} м скрыты автором.</p>}
      <ErrorLine error={error} />
    </section>
  );
}

// TrackSketch — линия трека в своём масштабе (равнопромежуточная проекция с
// поправкой на широту, чтобы север был сверху и пропорции не искажались).
export function TrackSketch({ points }: { points: [number, number][] }) {
  if (points.length < 2) return null;
  const W = 240, H = 180, pad = 12;
  const lat0 = (points.reduce((s, p) => s + p[0], 0) / points.length) * Math.PI / 180;
  const xs = points.map((p) => p[1] * Math.cos(lat0)), ys = points.map((p) => p[0]);
  const minX = Math.min(...xs), maxX = Math.max(...xs), minY = Math.min(...ys), maxY = Math.max(...ys);
  const scale = Math.min((W - 2 * pad) / (maxX - minX || 1e-9), (H - 2 * pad) / (maxY - minY || 1e-9));
  const ox = (W - (maxX - minX) * scale) / 2, oy = (H - (maxY - minY) * scale) / 2;
  const X = (x: number) => ox + (x - minX) * scale, Y = (y: number) => H - oy - (y - minY) * scale;
  const d = xs.map((x, i) => `${i ? "L" : "M"}${X(x).toFixed(1)} ${Y(ys[i]).toFixed(1)}`).join("");
  const last = xs.length - 1;
  return (
    <svg class="route-sketch" viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Схема трека">
      <path d={d} fill="none" class="route-line" stroke-width="2.5" stroke-linejoin="round" stroke-linecap="round" />
      <circle cx={X(xs[0])} cy={Y(ys[0])} r="4.5" class="route-start" />
      <circle cx={X(xs[last])} cy={Y(ys[last])} r="4.5" class="route-finish" />
      <text x={W - 6} y={14} text-anchor="end" class="route-north">С ↑</text>
    </svg>
  );
}

// ElevationProfile — высота по пройденному расстоянию.
export function ElevationProfile({ profile }: { profile: { d: number; e: number }[] }) {
  const W = 360, H = 180, L = 40, R = 8, T = 10, B = 24;
  const total = profile[profile.length - 1].d || 1;
  let lo = Math.min(...profile.map((p) => p.e)), hi = Math.max(...profile.map((p) => p.e));
  const step = niceStep((hi - lo) / 3 || 10);
  lo = Math.floor(lo / step) * step;
  hi = Math.max(Math.ceil(hi / step) * step, lo + step);
  const X = (d: number) => L + (d / total) * (W - L - R);
  const Y = (e: number) => T + (1 - (e - lo) / (hi - lo)) * (H - T - B);
  const line = profile.map((p, i) => `${i ? "L" : "M"}${X(p.d).toFixed(1)} ${Y(p.e).toFixed(1)}`).join("");
  const area = `${line}L${X(total).toFixed(1)} ${H - B}L${L} ${H - B}Z`;
  const yTicks: number[] = [];
  for (let e = lo; e <= hi + 1e-9; e += step) yTicks.push(e);
  const kmStep = niceStep(total / 1000 / 4) * 1000;
  const xTicks: number[] = [];
  for (let d = 0; d <= total + 1; d += kmStep) xTicks.push(d);
  return (
    <svg class="route-profile" viewBox={`0 0 ${W} ${H}`} role="img"
      aria-label={`Профиль высот: от ${Math.round(lo)} до ${Math.round(hi)} метров на ${km(total)}`}>
      {yTicks.map((e, i) => (
        <g key={`y${e}`}>
          <line x1={L} x2={W - R} y1={Y(e)} y2={Y(e)} class="route-grid" />
          <text x={L - 6} y={Y(e) + 4} text-anchor="end" class="route-axis">{nf0.format(e)}{i === yTicks.length - 1 ? " м" : ""}</text>
        </g>
      ))}
      {xTicks.map((d, i) => (
        <text key={`x${d}`} x={X(d)} y={H - 6} text-anchor={i === 0 ? "start" : i === xTicks.length - 1 && X(d) > W - R - 20 ? "end" : "middle"} class="route-axis">
          {nf1.format(d / 1000)}{i === xTicks.length - 1 ? " км" : ""}
        </text>
      ))}
      <path d={area} class="route-area" />
      <path d={line} fill="none" class="route-line" stroke-width="2" stroke-linejoin="round" />
    </svg>
  );
}

function niceStep(raw: number): number {
  if (!(raw > 0)) return 1;
  const p = Math.pow(10, Math.floor(Math.log10(raw)));
  const n = raw / p;
  return (n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10) * p;
}
