import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Photo, get, patch, post, upload } from "./api.js";
import { Empty, ErrorLine, Loading, useLoad } from "./ui.js";
import { Icon } from "./shellparts.js";
import { useMe } from "./session.js";
import { LibraryViewer } from "./libraryviewer.js";

interface LibraryPhoto extends Photo { created_at: string; album_id: number | null; archived: boolean; description: string }
interface Album { id: number; title: string; count: number; cover_url: string | null }
export function PhotosPage() {
  const me = useMe();
  const settingsKey = `sn-photo-albums-${me?.id}`;
  const [hiddenAlbums,setHiddenAlbums] = useState<number[]>(()=>{try{const v=JSON.parse(localStorage.getItem(settingsKey)??"[]");return Array.isArray(v)?v.filter(x=>Number.isSafeInteger(x)):[];}catch{return [];}});
  const [draftHidden,setDraftHidden] = useState<number[]>([]);
  const settings = useRef<HTMLDialogElement>(null);
  const menu = useRef<HTMLDetailsElement>(null);
  useEffect(()=>{
    const outside=(e:PointerEvent)=>{if(menu.current&&!menu.current.contains(e.target as Node))menu.current.open=false;};
    const escape=(e:KeyboardEvent)=>{if(e.key==="Escape"&&menu.current?.open){menu.current.open=false;menu.current.querySelector("summary")?.focus();}};
    document.addEventListener("pointerdown",outside);document.addEventListener("keydown",escape);
    return()=>{document.removeEventListener("pointerdown",outside);document.removeEventListener("keydown",escape);};
  },[]);
  const photos = useLoad(async () => {
    const all: LibraryPhoto[] = []; let cursor: string | null = null;
    do { const page: {items: LibraryPhoto[]; next_cursor: string | null} = await get(`/photos${cursor ? `?after=${cursor}` : ""}`); all.push(...page.items); cursor = page.next_cursor; } while (cursor);
    return all.sort((a,b) => b.created_at.localeCompare(a.created_at));
  }, []);
  const albums = useLoad(() => get<{items: Album[]}>("/photo-albums"), []);
  const [tab,setTab] = useState<"photos"|"albums"|"archive">(new URLSearchParams(location.search).get("tab")==="albums"?"albums":"photos");
  const [album,setAlbum] = useState<number | null>(null);
  const [selecting,setSelecting] = useState(false);
  const [selected,setSelected] = useState<string[]>([]);
  const [current,setCurrent] = useState<string | null>(null);
  const [error,setError] = useState<unknown>(null);
  const [busy,setBusy] = useState(false);
  const [progress,setProgress] = useState("");
  const [title,setTitle] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const creator = useRef<HTMLDialogElement>(null);
  const visible = (photos.data ?? []).filter(p => p.archived === (tab === "archive") && (album === null || p.album_id === album) && (tab!=="photos" || album!==null || !hiddenAlbums.includes(p.album_id??0)));
  const choices = [{id:0,title:"Без альбома",count:(photos.data??[]).filter(p=>!p.album_id&&!p.archived).length,cover_url:(photos.data??[]).find(p=>!p.album_id&&!p.archived)?.thumb_url??null},...(albums.data?.items??[])];
  function saveSettings(){try{localStorage.setItem(settingsKey,JSON.stringify(draftHidden));setHiddenAlbums(draftHidden);setSelected([]);settings.current?.close();}catch{setError(new Error("Не удалось сохранить настройки браузера."));}}
  const photo = visible.find(p => p.id === current);
  const reload = () => { photos.reload(); albums.reload(); };
  function switchTab(t: typeof tab) { setTab(t);setAlbum(null);setSelected([]);setSelecting(false); }
  function step(n: number) { const at=visible.findIndex(p=>p.id===current); if(visible.length) setCurrent(visible[(at+n+visible.length)%visible.length].id); }
  async function organize(action: {album_id?: number; archived?: boolean}) {
    if(!selected.length || busy) return; setBusy(true);setError(null);
    try { await patch("/photos",{ids:selected,...action});setSelected([]);setSelecting(false);reload(); } catch(e){setError(e);} finally {setBusy(false);}
  }
  async function files(files: File[]) {
    if(busy) return;setBusy(true);setError(null); const failures:string[]=[];
    for(let i=0;i<files.length;i++) {
      const file=files[i];setProgress(`Загрузка ${i+1} из ${files.length}`);
      try {
        if(file.size>10*1024*1024 || !/\.(jpe?g|png|gif)$/i.test(file.name)) throw new Error("JPEG, PNG или GIF, не более 10 МБ");
        const p=await upload<Photo>("/media",file);
        if(album!==null) await patch("/photos",{ids:[p.id],album_id:album});
      } catch(e){failures.push(`${file.name}: ${e instanceof Error?e.message:String(e)}`);}
    }
    setBusy(false);setProgress(failures.length?`Не удалось загрузить: ${failures.length}`:`Загружено фотографий: ${files.length}`);
    if(failures.length) setError(new Error(failures.join("; "))); if(input.current) input.current.value="";reload();
  }
  async function create(e: Event) {
    e.preventDefault();if(busy) return;setBusy(true);setError(null);
    try {const a=await post<Album>("/photo-albums",{title});setTitle("");creator.current?.close();setTab("photos");setAlbum(a.id);setSelected([]);reload();}catch(e){setError(e);}finally{setBusy(false);}
  }
  const years=[...new Set(visible.map(p=>new Date(p.created_at).getFullYear()))];
  return <section class="photo-library">
    <header class="library-panel"><h1>Мои фотографии</h1><div class="library-toolbar">
      <nav class="library-tabs" aria-label="Фотографии"><button aria-pressed={tab==="photos"} onClick={()=>switchTab("photos")}>Фото</button><button aria-pressed={tab==="albums"} onClick={()=>switchTab("albums")}>Альбомы</button>{tab==="archive"&&<button aria-pressed="true">Архив</button>}</nav>
      <div class="library-actions photo-actions"><button class="primary" disabled={busy} onClick={()=>input.current?.click()}><Icon name="photo"/>Загрузить фото</button>
        <details class="photo-menu" ref={menu}><summary aria-label="Действия с фотографиями">•••</summary><div class="photo-menu-items"><button onClick={()=>{switchTab("archive");if(menu.current)menu.current.open=false;}}>Архив</button><button onClick={()=>{creator.current?.showModal();if(menu.current)menu.current.open=false;}}>Создать альбом</button><button disabled={tab==="albums"} aria-pressed={selecting} onClick={()=>{setSelecting(!selecting);setSelected([]);if(menu.current)menu.current.open=false;}}>{selecting?"Отменить выбор":"Выбрать несколько"}</button></div></details>
        <button class="photo-settings-button" aria-label="Настройки вкладки Фото" title="Настройки вкладки Фото" onClick={()=>{setDraftHidden([...hiddenAlbums]);setError(null);settings.current?.showModal();}}><Icon name="settings"/></button>
      </div>
      <input ref={input} type="file" hidden multiple accept="image/jpeg,image/png,image/gif" onChange={e=>void files(Array.from((e.target as HTMLInputElement).files??[]))}/>
    </div><p class="muted">JPEG, PNG или GIF до 10 МБ. Можно выбрать несколько файлов.</p><p role="status">{progress}</p><ErrorLine error={error || photos.error || albums.error}/></header>
    {selecting && <div class="library-panel library-toolbar"><strong>Выбрано: {selected.length} / 200</strong><button disabled={busy||!selected.length} onClick={()=>organize({archived:tab!=="archive"})}>{tab==="archive"?"Восстановить":"В архив"}</button><label>В альбом <select disabled={busy||!selected.length} value="" onChange={e=>{const value=(e.target as HTMLSelectElement).value;if(value!=="") void organize({album_id:+value});}}><option value="">Выберите альбом</option><option value="0">Без альбома</option>{albums.data?.items.map(a=><option value={a.id}>{a.title}</option>)}</select></label></div>}
    <div class="library-panel">
      {album!==null && <div class="library-toolbar"><button onClick={()=>switchTab("albums")}>← Альбомы</button><h2>{albums.data?.items.find(a=>a.id===album)?.title}</h2></div>}
      {tab==="archive" && <p class="muted">Архив скрывает фото только из этой галереи. В опубликованных записях и сообщениях оно остаётся.</p>}
      {!photos.data && !photos.error ? <Loading/> : tab==="albums" ? <div class="album-grid">{albums.data?.items.map(a=><button class="album-tile" onClick={()=>{setAlbum(a.id);setTab("photos");}}>{a.cover_url?<img src={a.cover_url} alt="" loading="lazy"/>:<span class="album-placeholder"><Icon name="photo"/></span>}<strong>{a.title}</strong><span class="muted">Фотографий: {a.count}</span></button>)}{albums.data?.items.length===0&&<Empty>Создайте первый альбом, чтобы собрать фотографии вместе.</Empty>}</div> : visible.length===0 ? <Empty>{tab==="archive"?"В архиве пока нет фотографий.":hiddenAlbums.length&&album===null?"Нет фото из выбранных альбомов. Измените настройки вкладки.":"Загрузите фотографии — они появятся здесь."}</Empty> : years.map(year=><section class="photo-year"><h2>{year}</h2><div class="photo-grid">{visible.filter(p=>new Date(p.created_at).getFullYear()===year).map((p,i)=><button key={p.id} class={`photo-tile ${selected.includes(p.id)?"selected":""}`} aria-label={`${selecting?"Выбрать":"Открыть"} фото ${i+1}, ${new Date(p.created_at).toLocaleDateString()}`} aria-pressed={selecting?selected.includes(p.id):undefined} onClick={()=>{if(selecting)setSelected(selected.includes(p.id)?selected.filter(id=>id!==p.id):selected.length<200?[...selected,p.id]:selected);else setCurrent(p.id);}}><img src={p.thumb_url} alt={`Фотография ${i+1}`} loading="lazy"/>{selecting&&<span class="photo-check">{selected.includes(p.id)?"✓":"○"}</span>}</button>)}</div></section>)}
    </div>
    <dialog ref={creator} class="music-dialog" aria-label="Создать альбом"><form class="stack" onSubmit={create}><h2>Новый альбом</h2><label>Название альбома<input required maxLength={100} value={title} onInput={e=>setTitle((e.target as HTMLInputElement).value)}/></label><ErrorLine error={error}/><div class="library-actions"><button class="primary" disabled={busy}>Создать</button><button type="button" onClick={()=>creator.current?.close()}>Отмена</button></div></form></dialog>
    <dialog ref={settings} class="music-dialog photo-settings-dialog" aria-label="Настройки вкладки Фото"><button class="music-dialog-close" aria-label="Закрыть настройки фото" onClick={()=>settings.current?.close()}>×</button><header><h2>Вкладка «Фото»</h2><p class="muted">Выберите альбомы, снимки из которых хотите показывать во вкладке «Фото»</p></header><div class="photo-album-choices">{choices.map(a=><label key={a.id} class="photo-album-choice">{a.cover_url?<img src={a.cover_url} alt=""/>:<span class="photo-album-empty"><Icon name="photo"/></span>}<span><strong>{a.title}</strong><small>{a.count} фото</small></span><input type="checkbox" checked={!draftHidden.includes(a.id)} onChange={e=>setDraftHidden(e.currentTarget.checked?draftHidden.filter(id=>id!==a.id):[...draftHidden,a.id])}/></label>)}</div><ErrorLine error={error}/><footer><button class="ghost" onClick={()=>settings.current?.close()}>Отмена</button><button class="primary" onClick={saveSettings}>Сохранить</button></footer><p class="muted photo-settings-note">Настройка сохраняется в этом браузере. Видимость опубликованных фото не меняется.</p></dialog>
    {photo&&<LibraryViewer photo={photo} index={visible.findIndex(p=>p.id===current)} total={visible.length} album={albums.data?.items.find(a=>a.id===photo.album_id)?.title??"Мои фотографии"} onStep={step} onClose={()=>setCurrent(null)} onChanged={reload}/>}
  </section>;
}
