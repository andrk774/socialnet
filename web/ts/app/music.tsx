import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Page, del, get, post } from "./api.js";
import { Empty, ErrorLine, Loading, duration, useLoad } from "./ui.js";
import { MediaUpload } from "./uploads.js";
import { Icon } from "./shellparts.js";
import type { MiniPlayerState } from "./miniplayer.js";

import { MusicDownloads, OfflineMusic } from "./offlinemusic.js";

interface Track { id: string; title: string; artist: string; duration_sec: number; url: string; in_my_music: boolean }
interface Playlist { id: number; title: string; track_count: number; tracks?: Track[] }
type Tab = "home" | "mine" | "playlists" | "offline";

export function MusicPage({ userId, active=true, onPlayerChange }: { userId: number; active?:boolean; onPlayerChange?:(state:MiniPlayerState|null)=>void }) {
  const [tab, setTab] = useState<Tab>("home");
  const [tracks, setTracks] = useState<Track[]>([]);
  const [cursor, setCursor] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const playlists = useLoad(() => get<{ items: Playlist[] }>(`/users/${userId}/playlists`), [userId]);
  const [selected, setSelected] = useState<Playlist | null>(null);
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const results = useLoad(() => search.length >= 2 ? get<{ items: Track[] }>(`/search?type=tracks&q=${encodeURIComponent(search)}`) : Promise.resolve(null), [search]);
  const uploadDialog = useRef<HTMLDialogElement>(null);
  const createDialog = useRef<HTMLDialogElement>(null);
  const mixDialog = useRef<HTMLDialogElement>(null);
  const addDialog = useRef<HTMLDialogElement>(null);
  const [title, setTitle] = useState("");
  const [saving, setSaving] = useState(false);
  const [dialogError, setDialogError] = useState<unknown>(null);
  const [addTrack, setAddTrack] = useState<Track | null>(null);
  const [queue, setQueue] = useState<Track[]>([]);
  const [position, setPosition] = useState(0);
  const [source, setSource] = useState("mine");
  const [order, setOrder] = useState("shuffle");
  const [draftSource, setDraftSource] = useState("mine");
  const [draftOrder, setDraftOrder] = useState("shuffle");
  const audio = useRef<HTMLAudioElement>(null);
  const playing = queue[position];
  const [paused, setPaused] = useState(true);
  const [elapsed, setElapsed] = useState(0);
  const [length, setLength] = useState(0);
  const [volume, setVolume] = useState(1);
  useEffect(() => { setElapsed(0); setLength(playing?.duration_sec ?? 0); }, [playing?.id]);
  const [repeat,setRepeat] = useState(false);
  const [shuffle,setShuffle] = useState(false);
  const [recent,setRecent] = useState<Track[]>(()=>{try{const data=JSON.parse(localStorage.getItem(`sn-listening-${userId}`)??"[]");return Array.isArray(data)?data.filter(t=>typeof t?.id==="string"&&typeof t?.title==="string"&&typeof t?.url==="string").slice(0,24):[];}catch{return [];}});
  function heard(t:Track) {setRecent(old=>{const next=[t,...old.filter(x=>x.id!==t.id)].slice(0,24);try{localStorage.setItem(`sn-listening-${userId}`,JSON.stringify(next));}catch{}return next;});}
  function nextTrack(){if(shuffle&&queue.length>1){setPosition((position+1+Math.floor(Math.random()*(queue.length-1)))%queue.length);}else if(position+1<queue.length)setPosition(position+1);}
  async function removeCurrent(){if(!playing)return;await act(async()=>{await del(`/tracks/${playing.id}/my-music`);await load();});}
  useEffect(() => { if (playing) void audio.current?.play().catch(() => {}); }, [queue, position]);
  useEffect(()=>{const element=audio.current;return()=>element?.pause();},[playing?.id]);
  async function load(after?: number) {
    setBusy(true); setError(null);
    try {
      const page = await get<Page<Track>>(`/users/${userId}/music?limit=50${after ? `&before=${after}` : ""}`);
      setTracks((old) => after ? [...old, ...page.items] : page.items); setCursor(page.next_cursor);
    } catch (e) { setError(e); } finally { setBusy(false); }
  }
  useEffect(() => { void load(); }, [userId]);
  useEffect(() => { const timer = setTimeout(() => setSearch(query.trim()), 300); return () => clearTimeout(timer); }, [query]);
  const act = async (fn: () => Promise<void>) => { setError(null); try { await fn(); } catch (e) { setError(e); } };
  const play = (items: Track[], index = 0) => { setQueue(items); setPosition(index); };
  const toggleTrack = (items: Track[], index: number) => {
    const el = audio.current;
    if (playing?.id !== items[index].id || !el) { setPaused(true); play(items, index); return; }
    if (el.paused) void el.play().catch(() => setError(new Error("Нажмите воспроизведение в плеере.")));
    else el.pause();
  };
  useEffect(()=>{onPlayerChange?.(playing?{title:playing.title,artist:playing.artist,paused,previousEnabled:position>0,nextEnabled:shuffle?queue.length>1:position+1<queue.length,previous:()=>setPosition(p=>Math.max(0,p-1)),toggle:()=>toggleTrack(queue,position),next:nextTrack}:null);},[queue,position,paused,shuffle,onPlayerChange]);
  useEffect(()=>()=>onPlayerChange?.(null),[onPlayerChange]);
  useEffect(()=>{if(!active){for(const ref of [uploadDialog,createDialog,mixDialog,addDialog])ref.current?.close();}},[active]);
  async function mix() {
    setBusy(true); setError(null);
    try {
      let items: Track[];
      if (source === "mine") {
        items = []; let before: number | null = null;
        do {
          const page: Page<Track> = await get(`/users/${userId}/music?limit=50${before ? `&before=${before}` : ""}`);
          items.push(...page.items); before = page.next_cursor;
        } while (before !== null);
      } else items = (await get<Playlist>(`/playlists/${source}`)).tracks ?? [];
      if (order === "shuffle") for (let i = items.length - 1; i > 0; i--) { const j = Math.floor(Math.random() * (i + 1)); [items[i], items[j]] = [items[j], items[i]]; }
      if (!items.length) throw new Error("В выбранной коллекции пока нет треков. Добавьте музыку и попробуйте снова.");
      play(items);
    } catch (e) { setError(e); } finally { setBusy(false); }
  }
  async function create(e: Event) {
    e.preventDefault(); if (saving) return; setSaving(true); setDialogError(null);
    try { const pl = await post<Playlist>("/playlists", { title }); playlists.reload(); setSelected(pl); setTab("playlists"); setQuery(""); createDialog.current?.close(); setTitle(""); }
    catch (err) { setDialogError(err); } finally { setSaving(false); }
  }
  const openPlaylist = (id: number) => act(async () => { setSelected(await get<Playlist>(`/playlists/${id}`)); setTab("playlists"); setQuery(""); });
  const available = selected && tab === "playlists" ? selected.tracks ?? [] : tracks;
  const visible = search.length >= 2 ? results.data?.items ?? [] : available;
  const rows = (items: Track[]) => <ol class="tracks music-tracks">{items.map((t, i) => <li key={t.id}>
    <button class={playing?.id === t.id ? "track-btn on" : "track-btn"} aria-label={`${playing?.id === t.id && !paused ? "Пауза" : "Слушать"}: ${t.artist} — ${t.title}`} onClick={() => toggleTrack(items, i)}>{playing?.id === t.id && !paused ? "Ⅱ" : "▶"}</button>
    <span class="music-track-title"><strong>{t.title}</strong><small>{t.artist}</small><span class="sr-only">{t.artist} — {t.title}</span></span><span class="muted">{duration(t.duration_sec)}</span>
    <div class="music-track-actions"><button class="icon-btn" title="Добавить в плейлист" aria-label={`Добавить в плейлист: ${t.title}`} onClick={() => { setAddTrack(t); setDialogError(null); addDialog.current?.showModal(); }}><Icon name="plus" /></button>
    {search.length >= 2 && !t.in_my_music && !tracks.some((x) => x.id === t.id) && <button class="link" onClick={() => act(async () => { await post(`/tracks/${t.id}/my-music`); await load(); results.reload(); })}>Себе</button>}
    {selected && tab === "playlists" && search.length < 2 && <button class="icon-btn" aria-label={`Убрать из плейлиста: ${t.title}`} onClick={() => act(async () => { await del(`/playlists/${selected.id}/tracks/${t.id}`); setSelected(await get(`/playlists/${selected.id}`)); playlists.reload(); })}>×</button>}
    </div><MusicDownloads track={t} userId={userId}/></li>)}</ol>;
  const cards = <div class="music-playlists">{playlists.data?.items.map((pl, i) => <button key={pl.id} class={`playlist-card tone-${i % 4}`} onClick={() => openPlaylist(pl.id)}><span class="playlist-art">♫</span><strong>{pl.title}</strong><small>Треков: {pl.track_count}</small></button>)}</div>;
  const dialogClose = (ref: typeof mixDialog) => <button type="button" class="music-dialog-close" aria-label="Закрыть окно" onClick={() => ref.current?.close()}>×</button>;
  return <section class="music-page">
    <h1 class="sr-only">Музыка</h1>
    {playing && <div class="music-player-dock"><div class="player music-player"><div class="music-now"><button class="icon-btn" aria-label="Предыдущий трек" disabled={position === 0} onClick={() => setPosition(position - 1)}>‹</button><span><strong>{playing.title}</strong><small>{playing.artist} · {position + 1} / {queue.length}</small></span><button class="icon-btn" aria-label="Следующий трек" disabled={!shuffle && position + 1 >= queue.length} onClick={nextTrack}>›</button></div><audio ref={audio} key={playing.id} autoPlay loop={repeat} src={playing.url} onDurationChange={(e) => { if (Number.isFinite(e.currentTarget.duration) && e.currentTarget.duration > 0) setLength(e.currentTarget.duration); }} onTimeUpdate={(e) => setElapsed(e.currentTarget.currentTime)} onLoadedMetadata={(e) => { setLength(Number.isFinite(e.currentTarget.duration) && e.currentTarget.duration > 0 ? e.currentTarget.duration : playing.duration_sec); e.currentTarget.volume = volume; setElapsed(0); }} onPlay={() => {setPaused(false);heard(playing);}} onPause={() => setPaused(true)} onEnded={() => { setPaused(true); nextTrack(); }} onError={() => setError(new Error("Не удалось воспроизвести трек. Попробуйте следующий."))} /></div>
    <div class="music-playback-controls">
      <button class="primary playback-toggle" aria-label={paused ? "Воспроизвести" : "Пауза"} onClick={() => toggleTrack(queue, position)}>{paused ? "▶" : "Ⅱ"}</button>
      <button class="icon-btn" title="Вперемешку" aria-label="Вперемешку" aria-pressed={shuffle} onClick={()=>setShuffle(!shuffle)}>⇄</button><button class="icon-btn" title="Повтор трека" aria-label="Повтор трека" aria-pressed={repeat} onClick={()=>setRepeat(!repeat)}>↻</button>
      <span class="playback-time">{duration(elapsed)} / {duration(length || playing.duration_sec)}</span>
      <input type="range" min={0} max={length || 1} step={0.1} value={Math.min(elapsed, length || 1)} disabled={!length} aria-label="Позиция воспроизведения" onInput={(e) => { const value = Number(e.currentTarget.value); if (audio.current) audio.current.currentTime = value; setElapsed(value); }} />
      <label class="playback-volume">Громкость<input type="range" min={0} max={1} step={0.05} value={volume} aria-label="Громкость" onInput={(e) => { const value = Number(e.currentTarget.value); setVolume(value); if (audio.current) audio.current.volume = value; }} /></label>
      {tracks.some(t=>t.id===playing.id)&&<button class="icon-btn" title="Удалить из моей музыки" aria-label="Удалить из моей музыки" onClick={removeCurrent}>✓</button>}
    </div></div>}
    <div class="music-panel music-top">
      <div class="music-navigation"><nav aria-label="Разделы музыки">{([["offline", "\u041e\u0444\u043b\u0430\u0439\u043d"], ["home", "Главная"], ["mine", "Моя музыка"], ["playlists", "Плейлисты"]] as [Tab, string][]).map(([id, name]) => <button class={tab === id ? "music-tab active" : "music-tab"} aria-pressed={tab === id} onClick={() => { if(id === "offline") audio.current?.pause(); setTab(id); setSelected(null); setQuery(""); }}>{name}</button>)}</nav>
        <div class="music-tools"><button class="icon-btn" title="Загрузить аудиозапись" aria-label="Загрузить аудиозапись" onClick={() => uploadDialog.current?.showModal()}><Icon name="upload" /></button><button class="icon-btn" title="Создать плейлист" aria-label="Создать плейлист" onClick={() => { setDialogError(null); createDialog.current?.showModal(); }}><Icon name="plus" /></button></div>
      </div>
      <label class="music-search"><Icon name="search" /><input type="search" placeholder="Поиск музыки" aria-label="Поиск музыки" value={query} onInput={(e) => setQuery(e.currentTarget.value)} /></label>
      {tab === "home" && !query && <div class="music-mix"><button class="mix-play" aria-label="Слушать мой микс" disabled={busy} onClick={mix}>▶</button><h2>Слушать мой микс</h2><p>Ваша музыка в любимом порядке</p><button class="mix-settings" onClick={() => { setDraftSource(source); setDraftOrder(order); mixDialog.current?.showModal(); }}>☷ Настроить</button></div>}
    </div>
    {tab !== "offline" && <ErrorLine error={error ?? results.error ?? playlists.error} />}
    {tab === "mine" && !query && recent.length>0 && <div class="music-panel music-history"><div class="music-section-head"><h2>Недавно прослушанные</h2><button class="link" onClick={()=>{setRecent([]);try{localStorage.removeItem(`sn-listening-${userId}`);}catch{}}}>Очистить историю</button></div>{rows(recent)}</div>}
    {tab === "offline" ? <OfflineMusic userId={userId}/> : search.length >= 2 ? <div class="music-panel"><h2>Результаты поиска</h2>{!results.data ? <Loading /> : visible.length ? rows(visible) : <Empty>Ничего не найдено.</Empty>}</div> : <>
      {tab !== "playlists" && <div class="music-panel"><div class="music-section-head"><h2>Моя музыка</h2>{tab === "home" && <button class="link" onClick={() => setTab("mine")}>Показать все</button>}</div>{busy && !tracks.length ? <Loading /> : tracks.length ? rows(tab === "home" ? tracks.slice(0, 6) : tracks) : <Empty>Добавьте первые треки через кнопку загрузки или поиск.</Empty>}{tab === "mine" && cursor !== null && <button class="ghost" disabled={busy} onClick={() => load(cursor)}>Показать ещё</button>}</div>}
      {(tab === "home" || tab === "playlists") && <div class="music-panel"><div class="music-section-head"><h2>{selected && tab === "playlists" ? selected.title : "Мои плейлисты"}</h2>{selected && <button class="link" onClick={() => setSelected(null)}>Все плейлисты</button>}</div>{selected && tab === "playlists" ? (available.length ? rows(available) : <Empty>Плейлист пуст. Нажмите «+» рядом с треком, чтобы добавить его.</Empty>) : playlists.data?.items.length ? cards : <Empty>Создайте плейлист кнопкой «+» вверху страницы.</Empty>}</div>}
    </>}
    <dialog ref={uploadDialog} class="music-dialog music-upload-dialog" aria-label="Загрузка музыки">{dialogClose(uploadDialog)}<MediaUpload kind="music" onUploaded={() => { void load(); setTab("mine"); setQuery(""); }} /><footer class="upload-dialog-footer"><button class="primary" onClick={() => uploadDialog.current?.close()}>Закрыть</button></footer></dialog>
    <dialog ref={createDialog} class="music-dialog" aria-label="Создание плейлиста">{dialogClose(createDialog)}<form onSubmit={create}><h2>Создать плейлист</h2><label>Название плейлиста<input required maxLength={100} value={title} onInput={(e) => setTitle(e.currentTarget.value)} /></label><ErrorLine error={dialogError} /><button class="primary" disabled={saving || !title.trim()}>Создать</button></form></dialog>
    <dialog ref={addDialog} class="music-dialog" aria-label="Добавление в плейлист">{dialogClose(addDialog)}<h2>Добавить в плейлист</h2><p>{addTrack?.title}</p><ErrorLine error={dialogError} />{playlists.data?.items.length ? playlists.data.items.map((pl) => <button class="playlist-choice" disabled={saving} onClick={async () => { if (!addTrack || saving) return; setSaving(true); setDialogError(null); try { await post(`/playlists/${pl.id}/tracks`, { track_id: addTrack.id }); playlists.reload(); if (selected?.id === pl.id) setSelected(await get(`/playlists/${pl.id}`)); addDialog.current?.close(); } catch (e) { setDialogError(e); } finally { setSaving(false); } }}>{pl.title}</button>) : <Empty>Сначала создайте плейлист кнопкой «+» вверху страницы.</Empty>}</dialog>
    <dialog ref={mixDialog} class="music-dialog" aria-label="Настроить микс">{dialogClose(mixDialog)}<h2>Настроить мой микс</h2><fieldset><legend>Коллекция</legend><label><input type="radio" name="mix-source" checked={draftSource === "mine"} onChange={() => setDraftSource("mine")} /> Моя музыка</label>{playlists.data?.items.map((pl) => <label key={pl.id}><input type="radio" name="mix-source" checked={draftSource === String(pl.id)} onChange={() => setDraftSource(String(pl.id))} /> {pl.title}</label>)}</fieldset><fieldset><legend>Порядок воспроизведения</legend><button class={draftOrder === "shuffle" ? "chip active" : "chip"} aria-pressed={draftOrder === "shuffle"} onClick={() => setDraftOrder("shuffle")}>Вперемешку</button> <button class={draftOrder === "list" ? "chip active" : "chip"} aria-pressed={draftOrder === "list"} onClick={() => setDraftOrder("list")}>По списку</button></fieldset><footer><button class="ghost" onClick={() => { setDraftSource("mine"); setDraftOrder("shuffle"); }}>Сбросить</button><button class="primary" onClick={() => { setSource(draftSource); setOrder(draftOrder); mixDialog.current?.close(); }}>Применить</button></footer></dialog>
  </section>;
}
