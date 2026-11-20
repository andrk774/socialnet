import { SiteSettingsPanel } from "./site-settings.js";
import { AIPanel } from "./ai.js";
import { SMTPPanel } from "./smtp.js";
import { TURNPanel } from "./turn.js";
import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get } from "./api.js";
import { ErrorLine, Loading } from "./ui.js";
import { KubernetesPanel } from "./kubernetes.js";

interface SystemState {
 at: string; started_at: string; uptime_seconds: number; database_ok: boolean; redis: string;
 heap_bytes: number; runtime_bytes: number; goroutines: number; websockets: number;
 requests: number; errors_5xx: number; mean_latency_ms: number;
 pool: Record<string, number>; queues: Record<string, number> | null; backup: Record<string, number> | null;
}
export function SystemTab() {
 const [data, setData] = useState<SystemState>();
 const [error, setError] = useState<unknown>(null);
 const [history, setHistory] = useState<SystemState[]>([]);
 const [refresh, setRefresh] = useState(0);
 useEffect(() => {
   let live = true; let timer: ReturnType<typeof setTimeout>;
   const poll = async () => {
     try { const next = await get<SystemState>("/admin/system"); if (live) { setData(next); setError(null); setHistory((old) => [...(old.at(-1)?.started_at === next.started_at ? old : []), next].slice(-60)); } }
     catch (e) { if (live) setError(e); }
     finally { if (live) timer = setTimeout(poll, 10000); }
   };
   void poll(); return () => { live = false; clearTimeout(timer); };
 }, [refresh]);
 if (!data) return <><ErrorLine error={error} /><Loading /></>;
 const mib = (v: number) => (v / 1048576).toFixed(1) + " МБ";
 const prev = history.at(-2);
 const seconds = prev ? (Date.parse(data.at) - Date.parse(prev.at)) / 1000 : 0;
 const rate = prev && seconds > 0 ? Math.max(0, (data.requests - prev.requests) / seconds).toFixed(1) : "—";
 const tiles = [["PostgreSQL", data.database_ok ? "Доступен" : "Ошибка"], ["Redis", data.redis === "ok" ? "Доступен" : data.redis === "disabled" ? "Не настроен" : "Ошибка"], ["Время работы API", `${Math.floor(data.uptime_seconds / 60)} мин`], ["Память объектов Go", mib(data.heap_bytes)], ["Память среды Go", mib(data.runtime_bytes)], ["Горутины", data.goroutines], ["WebSocket-соединения", data.websockets], ["Запросов с запуска", data.requests], ["Ошибок HTTP 5xx с запуска", data.errors_5xx], ["Запросов в секунду", rate], ["Среднее время с запуска", `${data.mean_latency_ms.toFixed(1)} мс`], ["Соединения БД: занято / максимум", `${data.pool.acquired} / ${data.pool.max}`]];
 const max = Math.max(1, ...history.map((x) => x.heap_bytes));
 const points = history.map((x, i) => `${i * 600 / Math.max(1, history.length - 1)},${110 - x.heap_bytes / max * 95}`).join(" ");
 return <section class="system-monitor"><div class="row"><button class="ghost" onClick={() => setRefresh((n) => n + 1)}>Обновить</button><span class="muted">Обновление каждые 10 секунд · {new Date(data.at).toLocaleTimeString("ru")}</span></div><ErrorLine error={error} />{error && <p class="error">Показаны последние полученные данные.</p>}
 <p class="muted">Показатели текущего процесса API. Счётчики обнуляются при его перезапуске.</p>
 <ul class="tiles">{tiles.map(([name, value]) => <li class="tile" key={name}><strong class="tile-value">{value}</strong><span>{name}</span></li>)}</ul>
 <div class="music-panel"><h2>Память объектов Go</h2><svg viewBox="0 0 600 120" role="img" aria-label="Изменение памяти объектов Go за время наблюдения"><polyline points={points} fill="none" stroke="currentColor" stroke-width="2" /></svg><p class="muted">История последних 60 измерений в этой вкладке. Верхняя граница: {mib(max)}.</p></div>
 <div class="music-panel"><h2>Очереди</h2>{data.queues ? <dl class="system-queues"><dt>Задачи обработки</dt><dd>{data.queues.jobs}</dd><dt>Доставка событий</dt><dd>{data.queues.callbacks}</dd><dt>Открытые тревоги</dt><dd><a href="/admin?tab=alerts">{data.queues.open_alerts}</a></dd></dl> : <p class="error">Не удалось получить очереди.</p>}</div>
 <div class="music-panel"><h2>Резервное копирование</h2><p>{data.backup?.last_run_timestamp ? `Последний запуск: ${new Date(data.backup.last_run_timestamp * 1000).toLocaleString("ru")}` : "Нет данных о запусках."}</p><p>{data.backup?.last_success_timestamp ? `Последняя копия с успешной проверкой восстановления: ${new Date(data.backup.last_success_timestamp * 1000).toLocaleString("ru")}` : "Нет подтверждённой проверкой восстановления копии."}</p></div>
 <SiteSettingsPanel/><AIPanel/><SMTPPanel/><TURNPanel/><KubernetesPanel/></section>;
}
