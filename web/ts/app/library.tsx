// Закладки, документы и отложенные записи.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { Doc, Page, Post, Scheduled, del, fileSize, get, patch, saveFile, upload } from "./api.js";
import { PostView, Video } from "./posts.js";
import { Empty, ErrorLine, Loading, Time, duration, useLoad } from "./ui.js";

interface BVideo { id: string; title: string; poster_url?: string; hls_url?: string; mp4_url?: string }
interface BTrack { id: string; title: string; artist: string; duration_sec: number; url: string }
interface Bookmark { id: number; kind: "post" | "video" | "track"; created_at: string; post?: Post; video?: BVideo; track?: BTrack }

const kinds: [Bookmark["kind"], string][] = [["post", "Записи"], ["video", "Видео"], ["track", "Музыка"]];

export function BookmarksPage() {
  const [kind, setKind] = useState<Bookmark["kind"]>("post");
  const list = useLoad(() => get<Page<Bookmark>>(`/bookmarks?kind=${kind}&limit=50`), [kind]);
  const [error, setError] = useState<unknown>(null);
  const unsave = (b: Bookmark, target: string) => del(`/bookmarks/${b.kind}/${target}`).then(list.reload, setError);
  return (
    <section>
      <h1>Закладки</h1>
      <div class="tabs" role="tablist">
        {kinds.map(([k, label]) => <button key={k} role="tab" aria-selected={kind === k} class={kind === k ? "tab on" : "tab"} onClick={() => setKind(k)}>{label}</button>)}
      </div>
      <ErrorLine error={error ?? list.error} />
      {!list.data ? <Loading /> : list.data.items.length === 0 ? <Empty>Здесь появится то, что вы сохраните кнопкой «В закладки».</Empty> :
        list.data.items.map((b) =>
          b.post ? <PostView key={b.id} initial={b.post} /> :
          b.video ? (
            <div class="post" key={b.id}>
              <Video src={b.video.hls_url ?? ""} mp4={b.video.mp4_url} poster={b.video.poster_url ?? ""} title={b.video.title} />
              <button class="link" onClick={() => unsave(b, b.video!.id)}>Убрать из закладок</button>
            </div>
          ) : b.track ? (
            <div class="track" key={b.id}>
              <span>{b.track.artist} — {b.track.title}</span><span class="muted">{duration(b.track.duration_sec)}</span>
              <audio controls preload="none" src={b.track.url} />
              <button class="link" onClick={() => unsave(b, b.track!.id)}>Убрать</button>
            </div>
          ) : null)}
    </section>
  );
}

export function DocumentsPage() {
  const list = useLoad(() => get<Page<Doc>>("/documents?limit=50"), []);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  async function add(files: FileList | null) {
    if (!files) return;
    setBusy(true); setError(null);
    for (const f of Array.from(files)) {
      try { await upload<Doc>("/documents", f); } catch (e) { setError(e); }
    }
    setBusy(false); list.reload();
  }
  async function rename(d: Doc) {
    const title = prompt("Новое название", d.title);
    if (title === null || !title.trim()) return;
    await patch(`/documents/${d.id}`, { title }).then(list.reload, setError);
  }
  async function remove(d: Doc) {
    if (!confirm(`Удалить «${d.title}»? Документ пропадёт из записей и сообщений.`)) return;
    await del(`/documents/${d.id}`).then(list.reload, setError);
  }

  return (
    <section>
      <h1>Документы</h1>
      <p class="muted">PDF, Word, Excel, PowerPoint, OpenDocument, EPUB, DjVu, RTF, TXT, CSV и архивы — до 50 МБ.
        Скачать документ могут только те, кому видна запись или сообщение с ним.</p>
      <label class="ghost file-btn">{busy ? "Загружаем…" : "Загрузить документ"}
        <input type="file" multiple class="sr-only" disabled={busy} onChange={(e) => add((e.target as HTMLInputElement).files)} />
      </label>
      <ErrorLine error={error ?? list.error} />
      {!list.data ? <Loading /> : list.data.items.length === 0 ? <Empty>Документов пока нет.</Empty> : (
        <ul class="doc-list">
          {list.data.items.map((d) => (
            <li key={d.id} class="doc">
              <span class="doc-ext" aria-hidden="true">{d.ext}</span>
              <button class="link doc-name" onClick={() => saveFile(d.url, `${d.title}.${d.ext}`).catch(setError)}>{d.title}.{d.ext}</button>
              <span class="muted">{fileSize(d.size)} · <Time iso={d.created_at} /></span>
              <button class="link" onClick={() => rename(d)}>Переименовать</button>
              <button class="link" onClick={() => remove(d)}>Удалить</button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

const localInput = (iso: string) => { const d = new Date(iso); return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16); };

export function ScheduledPage({ communityId }: { communityId?: number }) {
  const list = useLoad(() => get<{ items: Scheduled[] }>("/scheduled-posts" + (communityId ? `?community_id=${communityId}` : "")), [communityId]);
  const [error, setError] = useState<unknown>(null);
  const [editing, setEditing] = useState<number | null>(null);
  const [when, setWhen] = useState("");

  async function save(s: Scheduled) {
    setError(null);
    try { await patch(`/scheduled-posts/${s.id}`, { publish_at: new Date(when).toISOString() }); setEditing(null); list.reload(); } catch (e) { setError(e); }
  }
  async function cancel(s: Scheduled) {
    if (!confirm("Отменить публикацию?")) return;
    await del(`/scheduled-posts/${s.id}`).then(list.reload, setError);
  }

  const summary = (s: Scheduled) => {
    const parts = [];
    if (s.payload.text) parts.push(s.payload.text.length > 140 ? s.payload.text.slice(0, 140) + "…" : s.payload.text);
    if (s.payload.poll) parts.push(`Опрос: ${s.payload.poll.question}`);
    if (s.payload.photo_ids?.length) parts.push(`фото: ${s.payload.photo_ids.length}`);
    if (s.payload.document_ids?.length) parts.push(`документов: ${s.payload.document_ids.length}`);
    return parts.join(" · ") || "Запись";
  };

  return (
    <section>
      {!communityId && <h1>Отложенные записи</h1>}
      <ErrorLine error={error ?? list.error} />
      {!list.data ? <Loading /> : list.data.items.length === 0 ? <Empty>Нет запланированных записей. В редакторе записи нажмите «Отложить».</Empty> : (
        <ul class="plain-list">
          {list.data.items.map((s) => (
            <li key={s.id} class="scheduled">
              <span>
                <strong>{new Date(s.publish_at).toLocaleString("ru", { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" })}</strong>
                {s.status === "failed" && <span class="error"> · не опубликовано: {s.error}</span>}
                <br /><span class="muted">{summary(s)}</span>
              </span>
              {editing === s.id ? (
                <span class="row tight">
                  <label class="sr-only" for={`when-${s.id}`}>Новое время</label>
                  <input id={`when-${s.id}`} type="datetime-local" value={when} onInput={(e) => setWhen((e.target as HTMLInputElement).value)} />
                  <button class="primary" onClick={() => save(s)}>Сохранить</button>
                  <button class="link" onClick={() => setEditing(null)}>Отмена</button>
                </span>
              ) : (
                <span class="row tight">
                  <button class="ghost" onClick={() => { setEditing(s.id); setWhen(localInput(s.publish_at)); }}>{s.status === "failed" ? "Повторить" : "Перенести"}</button>
                  <button class="link" onClick={() => cancel(s)}>Отменить</button>
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
