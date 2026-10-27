// Предупреждения на территории: баннер на маршруте и походе, управление
// в админке (модераторы).
import { h, Fragment } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { del, get, patch, post } from "./api.js";
import { Empty, ErrorLine, Loading, useLoad } from "./ui.js";

export interface AreaNotice {
  id: number; kind: "fire" | "closure" | "flood" | "other"; title: string; body: string; source_url: string;
  bbox: [number, number, number, number]; starts_on: string; ends_on: string | null; updated_at: string;
}

const kindLabel: Record<AreaNotice["kind"], string> = { fire: "Противопожарный режим", closure: "Тропа закрыта", flood: "Паводок", other: "Внимание" };
const dayFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long" });
const day = (s: string) => dayFmt.format(new Date(s + "T12:00:00"));
const period = (n: AreaNotice) => n.ends_on ? `${day(n.starts_on)} — ${day(n.ends_on)}` : `с ${day(n.starts_on)}, до отмены`;

export function NoticeBanner({ notices }: { notices?: AreaNotice[] }) {
  if (!notices || notices.length === 0) return null;
  return (
    <div class="notices" role="note">
      {notices.map((n) => (
        <div key={n.id} class={`notice notice-${n.kind}`}>
          <strong>{kindLabel[n.kind]}: {n.title}</strong>
          <span class="muted"> · {period(n)}</span>
          {n.body && <p>{n.body}</p>}
          {n.source_url && <> · <a href={n.source_url} target="_blank" rel="noopener noreferrer">источник</a></>}
        </div>
      ))}
    </div>
  );
}

// Прямоугольники основных районов — примерные, их можно поправить вручную.
const presets: [string, [number, number, number, number]][] = [
  ["Таганай", [55.15, 59.75, 55.45, 60.10]],
  ["Зюраткуль", [54.80, 59.05, 55.05, 59.40]],
  ["Иремель", [54.40, 58.70, 54.65, 59.05]],
  ["Уральские Зори / Александровская сопка", [54.95, 59.45, 55.20, 59.75]],
  ["Ильменский заповедник", [54.95, 60.05, 55.25, 60.35]],
  ["Челябинская область целиком", [51.95, 57.10, 56.40, 63.40]],
];

const emptyForm = { kind: "fire", title: "", body: "", source_url: "", bbox: ["", "", "", ""], starts_on: new Date().toISOString().slice(0, 10), ends_on: "" };

export function NoticesTab() {
  const list = useLoad(() => get<{ items: AreaNotice[] }>("/notices?all=1"), []);
  const [f, setF] = useState(emptyForm);
  const [editing, setEditing] = useState<number | null>(null);
  const [error, setError] = useState<unknown>(null);
  const set = (k: keyof typeof emptyForm) => (e: Event) => setF({ ...f, [k]: (e.target as HTMLInputElement).value });
  const setBox = (i: number) => (e: Event) => { const b = [...f.bbox]; b[i] = (e.target as HTMLInputElement).value; setF({ ...f, bbox: b }); };
  async function save(e: Event) {
    e.preventDefault();
    setError(null);
    const body = { ...f, bbox: f.bbox.map(Number), ends_on: f.ends_on || null };
    try {
      if (editing) await patch(`/mod/notices/${editing}`, body); else await post("/mod/notices", body);
      setF(emptyForm); setEditing(null); list.reload();
    } catch (err) { setError(err); }
  }
  const edit = (n: AreaNotice) => {
    setEditing(n.id);
    setF({ kind: n.kind, title: n.title, body: n.body, source_url: n.source_url, bbox: n.bbox.map(String), starts_on: n.starts_on, ends_on: n.ends_on ?? "" });
  };
  const today = new Date().toISOString().slice(0, 10);
  return (
    <>
      <p class="muted">Показываются на маршрутах и походах, которые проходят через прямоугольник в дни действия.</p>
      <form class="stack notice-form" onSubmit={save}>
        <div class="row">
          <label>Что<select value={f.kind} onChange={set("kind")}>
            {(Object.keys(kindLabel) as AreaNotice["kind"][]).map((k) => <option key={k} value={k}>{kindLabel[k]}</option>)}
          </select></label>
          <label class="grow">Заголовок<input required maxLength={200} value={f.title} onInput={set("title")} placeholder="Запрет посещения лесов" /></label>
        </div>
        <label>Подробности<textarea rows={2} maxLength={2000} value={f.body} onInput={set("body")} /></label>
        <label>Ссылка на источник<input type="url" maxLength={500} value={f.source_url} onInput={set("source_url")} placeholder="https://…" /></label>
        <div class="row">
          <label>Район<select onChange={(e) => { const p = presets[+(e.target as HTMLSelectElement).value]; if (p) setF({ ...f, bbox: p[1].map(String) }); }}>
            <option value="">— вручную —</option>
            {presets.map(([name], i) => <option key={name} value={String(i)}>{name}</option>)}
          </select></label>
          {["Широта от", "Долгота от", "Широта до", "Долгота до"].map((label, i) => (
            <label key={label}>{label}<input required inputMode="decimal" size={7} value={f.bbox[i]} onInput={setBox(i)} /></label>
          ))}
        </div>
        <div class="row">
          <label>С<input type="date" required value={f.starts_on} onInput={set("starts_on")} /></label>
          <label>По (пусто — до отмены)<input type="date" value={f.ends_on} onInput={set("ends_on")} /></label>
        </div>
        <ErrorLine error={error} />
        <div class="row">
          <button class="primary">{editing ? "Сохранить" : "Добавить предупреждение"}</button>
          {editing && <button type="button" class="ghost" onClick={() => { setEditing(null); setF(emptyForm); }}>Отмена</button>}
        </div>
      </form>
      <ErrorLine error={list.error} />
      {!list.data ? <Loading /> : list.data.items.length === 0 ? <Empty>Предупреждений нет.</Empty> : (
        <ul class="plain-list notices-admin">
          {list.data.items.map((n) => (
            <li key={n.id} class={n.ends_on && n.ends_on < today ? "past" : ""}>
              <span><strong>{kindLabel[n.kind]}: {n.title}</strong><br /><span class="muted">{period(n)} · {n.bbox.map((v) => v.toFixed(2)).join(", ")}</span></span>
              <span class="row tight">
                <button class="ghost" onClick={() => edit(n)}>Изменить</button>
                <button class="ghost danger" onClick={() => confirm("Удалить предупреждение?") && del(`/mod/notices/${n.id}`).then(list.reload, setError)}>Удалить</button>
              </span>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
