import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Page, Photo, Self, UserCard, get, put, post, upload } from "./api.js";
import { setMe } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, Time, duration, useLoad } from "./ui.js";
import { Video } from "./posts.js";

export function AvatarUpload({ user, onSaved }: { user: UserCard; onSaved: () => void }) {
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const input = useRef<HTMLInputElement>(null);
  const menu = useRef<HTMLDetailsElement>(null);
  const viewer = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const outside = (e: PointerEvent) => { if (menu.current && !menu.current.contains(e.target as Node)) menu.current.open = false; };
    const escape = (e: KeyboardEvent) => { if (e.key === "Escape" && menu.current?.open) { menu.current.open = false; menu.current.querySelector("summary")?.focus(); } };
    document.addEventListener("pointerdown", outside); document.addEventListener("keydown", escape);
    return () => { document.removeEventListener("pointerdown", outside); document.removeEventListener("keydown", escape); };
  }, []);
  async function remove() {
    if (busy || !confirm("Удалить фото профиля?")) return;
    setBusy(true); setError(null);
    try { setMe(await put<Self>("/users/me/avatar", { media_id: null })); setFile(null); if (input.current) input.current.value = ""; onSaved(); if (menu.current) menu.current.open = false; }
    catch (e) { setError(e); } finally { setBusy(false); }
  }
  useEffect(() => {
    if (!file) { setPreview(""); return; }
    const url = URL.createObjectURL(file); setPreview(url);
    return () => URL.revokeObjectURL(url);
  }, [file]);
  async function save() {
    if (!file || busy) return;
    setBusy(true); setError(null);
    try {
      const photo = await upload<Photo>("/media", file);
      setMe(await put<Self>("/users/me/avatar", { media_id: photo.id }));
      setFile(null); if (input.current) input.current.value = ""; onSaved();
    } catch (e) { setError(e); } finally { setBusy(false); }
  }
  return <div class="avatar-upload">
    <details ref={menu} class="profile-avatar-menu">
      <summary aria-label="Меню фотографии профиля"><Avatar user={user} size={144} /><span class="avatar-plus" aria-hidden="true">+</span></summary>
      <div class="profile-avatar-options">
        {user.avatar_url && <button type="button" onClick={() => { if (menu.current) menu.current.open = false; viewer.current?.showModal(); }}>Открыть фото</button>}
        <button type="button" disabled={busy} onClick={() => { if (menu.current) menu.current.open = false; input.current?.click(); }}>Изменить фото</button>
        {user.avatar_url && <button type="button" class="avatar-delete" disabled={busy} onClick={remove}>Удалить фото</button>}
      </div>
    </details>
      <input ref={input} type="file" class="sr-only" aria-label="Новое фото профиля" accept="image/jpeg,image/png,image/gif" disabled={busy} onChange={(e) => {
        const el = e.currentTarget; const selected = el.files?.[0]; setError(null);
        if (selected && selected.size > 10 * 1024 * 1024) { setError(new Error("Фото должно быть не больше 10 МБ")); el.value = ""; setFile(null); return; }
        setFile(selected ?? null);
      }} />
    <dialog ref={viewer} class="avatar-photo-dialog" aria-label="Фотография профиля" onClick={(e) => { if (e.target === e.currentTarget) viewer.current?.close(); }}>
      <button class="photo-close" aria-label="Закрыть фото" onClick={() => viewer.current?.close()}>×</button>
      {user.avatar_url && <img src={user.avatar_url.replace(/\/thumb$/, "/full")} alt="Фотография профиля" />}
    </dialog>
    {file && <small class="muted">JPG, PNG, GIF · до 10 МБ</small>}
    {file && <div class="avatar-upload-preview">{preview && <img src={preview} alt="Новый аватар" />}<button class="primary" disabled={busy} onClick={save}>{busy ? "Сохраняем…" : "Сохранить аватар"}</button><button class="ghost" disabled={busy} onClick={() => { setFile(null); if (input.current) input.current.value = ""; }}>Отмена</button></div>}
    <ErrorLine error={error} />
  </div>;
}

export function MediaUpload({ kind, onUploaded }: { kind: "music" | "video"; onUploaded: () => void }) {
  const [file, setFile] = useState<File | null>(null);
  const [title, setTitle] = useState("");
  const [artist, setArtist] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [done, setDone] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const music = kind === "music";
  async function send(e: Event) {
    e.preventDefault(); if (!file || busy) return;
    setBusy(true); setError(null); setDone(false);
    try {
      await upload(music ? "/tracks" : "/videos", file, music ? { title, artist } : { title });
      setFile(null); setTitle(""); setArtist(""); if (input.current) input.current.value = "";
      setDone(true); onUploaded();
    } catch (err) { setError(err); } finally { setBusy(false); }
  }
  return <form class="media-upload" onSubmit={send} aria-label={music ? "Загрузка музыки" : "Загрузка видео"}>
    <h2>{music ? "Выберите аудиозапись на вашем компьютере" : "Загрузить видео"}</h2>
    {music ? <div class="upload-restrictions"><p class="muted">Ограничения</p><ul><li>Аудиофайл — до 50 МБ, длительностью от 1 секунды до 20 минут. Форматы: MP3, OGG, FLAC, WAV, M4A.</li><li>Загружайте музыку, которую вы вправе размещать.</li></ul></div> : <p class="muted">MP4, MOV, MKV, WEBM · до 1 ГБ и 30 минут</p>}
    <label class={music ? "sr-only" : ""}>Файл<input ref={input} type="file" required disabled={busy} accept={music ? ".mp3,.ogg,.flac,.wav,.m4a" : ".mp4,.mov,.mkv,.webm"} onChange={(e) => {
      const el = e.currentTarget; const selected = el.files?.[0]; setError(null); setDone(false);
      if (selected && selected.size > (music ? 50 * 1024 * 1024 : 1024 * 1024 * 1024)) { setError(new Error(music ? "Файл больше 50 МБ" : "Файл больше 1 ГБ")); el.value = ""; setFile(null); return; }
      setFile(selected ?? null); setTitle(selected?.name.replace(/\.[^.]+$/, "") ?? "");
    }} /></label>
    {music && <div class="upload-file-choice"><button type="button" class="primary" disabled={busy} onClick={() => input.current?.click()}>{file ? "Выбрать другой файл" : "Выбрать файл"}</button>{file && <p class="muted">{file.name}</p>}</div>}
    {file && <><label>Название<input value={title} maxLength={200} disabled={busy} onInput={(e) => setTitle(e.currentTarget.value)} /></label>{music && <label>Исполнитель<input value={artist} maxLength={200} disabled={busy} onInput={(e) => setArtist(e.currentTarget.value)} /></label>}<button class="primary" disabled={busy}>{busy ? "Загрузка и обработка…" : "Загрузить"}</button></>}
    {busy && <p role="status">Не закрывайте страницу до окончания загрузки.</p>}
    {done && <p role="status">{music ? "Трек добавлен в вашу музыку." : "Видео загружено. После обработки здесь появится плеер."}</p>}
    <ErrorLine error={error} />
  </form>;
}

import { VideoDownloads } from "./offlinevideo.js";
interface UploadedVideo { views: number; id: string; title: string; status: string; poster_url?: string; hls_url?: string; mp4_url?: string; duration_sec?: number; created_at: string }
export function VideosPage({ userId }: { userId: number }) {
  const list = useLoad(() => get<Page<UploadedVideo>>(`/users/${userId}/videos?limit=50`), [userId]);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("all");
  const [selected, setSelected] = useState<UploadedVideo | null>(null);
  const [viewError,setViewError] = useState<unknown>(null);
  const viewer = useRef<HTMLDialogElement>(null);
  const uploader = useRef<HTMLDialogElement>(null);
  useEffect(() => { if (selected) viewer.current?.showModal(); }, [selected]);
  useEffect(() => {
    if (!list.data?.items.some((v) => v.status === "processing")) return;
    const timer = window.setTimeout(list.reload, 4000);
    return () => clearTimeout(timer);
  }, [list.data]);
  const videos = list.data?.items.filter((v) => (filter === "all" || v.status === filter) && v.title.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())) ?? [];
  return <section class="video-library">
    <header class="video-library-header"><div><h1>Видео</h1><p class="muted">Ваша видеотека</p></div><button class="primary" onClick={() => uploader.current?.showModal()}>Загрузить видео</button></header>
    <label class="video-search"><span class="sr-only">Поиск видео</span><input type="search" placeholder="Поиск по названию" value={query} onInput={(e) => setQuery(e.currentTarget.value)} /></label>
    <nav class="video-filters" aria-label="Фильтры видео">{[["all", "Все"], ["ready", "Готовые"], ["processing", "Обрабатываются"], ["failed", "С ошибкой"]].map(([id, label]) => <button class={filter === id ? "chip active" : "chip"} aria-pressed={filter === id} onClick={() => setFilter(id)}>{label}</button>)}</nav>
    <ErrorLine error={list.error} />
    {!list.data ? <Loading /> : !videos.length ? <Empty>{list.data.items.length ? "Нет видео по выбранным условиям." : "Загрузите своё первое видео."}</Empty> : <div class="video-grid">{videos.map((v) => <article class="video-card" key={v.id}>
      <button class="video-thumbnail" disabled={v.status !== "ready"} aria-label={`Смотреть: ${v.title}`} onClick={() => {setViewError(null);setSelected(v)}}>
        {v.poster_url ? <img src={v.poster_url} alt="" loading="lazy" /> : <span class="video-placeholder" aria-hidden="true">▷</span>}
        {v.status === "ready" && <><span class="video-play" aria-hidden="true">▶</span>{v.duration_sec !== undefined && <span class="video-duration">{duration(v.duration_sec)}</span>}</>}
        {v.status !== "ready" && <span class="video-state">{v.status === "failed" ? "Ошибка обработки" : "Обрабатывается…"}</span>}
      </button>
      <h2>{v.title}</h2><p class="muted">{v.views ?? 0} просмотров</p><p class="muted"><Time iso={v.created_at} /></p>
      {v.status==="ready"&&<VideoDownloads video={v} userId={userId}/>}
      {v.status === "failed" && <p class="error">Попробуйте загрузить другой файл.</p>}
    </article>)}</div>}
    <dialog ref={uploader} class="music-dialog" aria-label="Загрузка видео"><button class="music-dialog-close" aria-label="Закрыть загрузку видео" onClick={() => uploader.current?.close()}>×</button><MediaUpload kind="video" onUploaded={() => { list.reload(); setFilter("all"); setQuery(""); }} /></dialog>
    <dialog ref={viewer} class="video-viewer" aria-label="Просмотр видео" onClose={() => setSelected(null)} onClick={(e) => { if (e.target === e.currentTarget) viewer.current?.close(); }}><button class="music-dialog-close" aria-label="Закрыть видео" onClick={() => viewer.current?.close()}>×</button>{selected && <><Video onPlay={() => { void post<{views:number}>(`/videos/${selected.id}/view`).then(x => { setSelected(old => old?.id === selected.id ? {...old,views:x.views} : old); list.reload(); }).catch(setViewError); }} key={selected.id} src={selected.hls_url ?? ""} mp4={selected.mp4_url} poster={selected.poster_url ?? ""} title={selected.title} /><ErrorLine error={viewError}/><p class="muted">{selected.views ?? 0} просмотров</p></>}</dialog>
  </section>;
}
