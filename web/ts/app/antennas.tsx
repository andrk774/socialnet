// «Антенны» — свои ленты по ключевым словам: например, всё про Таганай и
// Иремель, кроме объявлений «продам».
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { del, get, post, put } from "./api.js";
import { PostList } from "./posts.js";
import { query } from "./router.js";
import { Empty, ErrorLine, Loading, useLoad } from "./ui.js";

interface Antenna { id: number; name: string; keywords: string[]; excludes: string[]; unread: number }

const words = (s: string) => s.split(",").map((w) => w.trim()).filter(Boolean);

function AntennaForm({ initial, onDone }: { initial?: Antenna; onDone: (a?: Antenna) => void }) {
  const [name, setName] = useState(initial?.name ?? "");
  const [kw, setKw] = useState(initial?.keywords.join(", ") ?? "");
  const [ex, setEx] = useState(initial?.excludes.join(", ") ?? "");
  const [error, setError] = useState<unknown>(null);
  const submit = async (e: Event) => {
    e.preventDefault();
    setError(null);
    const body = { name, keywords: words(kw), excludes: words(ex) };
    try { onDone(initial ? await put<Antenna>(`/antennas/${initial.id}`, body) : await post<Antenna>("/antennas", body)); } catch (err) { setError(err); }
  };
  return (
    <form class="stack hike-form antenna-form" onSubmit={submit}>
      <label>Название<input value={name} required maxLength={50} onInput={(e) => setName((e.target as HTMLInputElement).value)} placeholder="Таганай и Иремель" /></label>
      <label>Слова через запятую<input value={kw} required onInput={(e) => setKw((e.target as HTMLInputElement).value)} placeholder="таганай, иремель, зюраткуль" />
        <span class="hint">Словоформы находятся сами: «Таганай» найдёт и «на Таганае».</span></label>
      <label>Кроме слов<input value={ex} onInput={(e) => setEx((e.target as HTMLInputElement).value)} placeholder="продам, куплю" /></label>
      <ErrorLine error={error} />
      <div class="row"><button class="primary">{initial ? "Сохранить" : "Создать ленту"}</button>
        <button type="button" class="ghost" onClick={() => onDone()}>Отмена</button></div>
    </form>
  );
}

export function AntennasPage() {
  const list = useLoad(() => get<{ items: Antenna[] }>("/antennas"), []);
  const [active, setActive] = useState<number | null>(query("id") ? +query("id")! : null);
  const [mode, setMode] = useState<"view" | "new" | "edit">("view");
  const [error, setError] = useState<unknown>(null);
  if (list.error) return <ErrorLine error={list.error} />;
  if (!list.data) return <Loading />;
  const items = list.data.items;
  const cur = items.find((a) => a.id === active) ?? null;
  return (
    <section>
      <div class="page-head">
        <h1>Мои ленты</h1>
        {items.length < 5 && mode === "view" && <button class="primary" onClick={() => setMode("new")}>Новая лента</button>}
      </div>
      {mode === "new" && <AntennaForm onDone={(a) => { setMode("view"); if (a) { setActive(a.id); list.reload(); } }} />}
      {items.length === 0 && mode === "view" ? <Empty>Соберите свою ленту по словам: например, все записи про Таганай и Иремель.</Empty> : (
        <div class="row tight antenna-tabs" role="tablist">
          {items.map((a) => (
            <button key={a.id} role="tab" aria-selected={a.id === active} class={a.id === active ? "chip active" : "chip"} onClick={() => { setActive(a.id); setMode("view"); list.reload(); }}>
              {a.name}{a.unread > 0 && a.id !== active ? <span class="badge" aria-label={`новых: ${a.unread}`}>{a.unread >= 99 ? "99+" : a.unread}</span> : null}
            </button>
          ))}
        </div>
      )}
      {cur && mode === "edit" && <AntennaForm initial={cur} onDone={(a) => { setMode("view"); if (a) list.reload(); }} />}
      {cur && mode === "view" && (
        <div class="antenna">
          <p class="muted">Слова: {cur.keywords.join(", ")}{cur.excludes.length > 0 ? ` · кроме: ${cur.excludes.join(", ")}` : ""} ·{" "}
            <button class="link" onClick={() => setMode("edit")}>Изменить</button> ·{" "}
            <button class="link" onClick={() => confirm("Удалить ленту?") && del(`/antennas/${cur.id}`).then(() => { setActive(null); list.reload(); }, setError)}>Удалить</button></p>
          <ErrorLine error={error} />
          <PostList key={cur.id} path={`/antennas/${cur.id}/posts`} empty="Пока нет записей с этими словами." />
        </div>
      )}
    </section>
  );
}
