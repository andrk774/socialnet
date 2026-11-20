import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Comment, Doc, Photo, PostTrack, PostVideo, get, saveFile, upload } from "./api.js";
import { me } from "./session.js";
import { ErrorLine, Loading, useLoad } from "./ui.js";
import { MediaUpload } from "./uploads.js";

export const stickers = [["hello","👋","Привет!"],["thanks","🌟","Спасибо!"],["hug","🤗","Обнимаю"],["wow","🚀","Вот это да!"],["yes","🐱","Всё получится!"],["sad","🌧️","Грустно"]];
export type CommentDraft = {photos:Photo[];tracks:PostTrack[];videos:PostVideo[];documents:Doc[];sticker:string};
export const emptyDraft=():CommentDraft=>({photos:[],tracks:[],videos:[],documents:[],sticker:""});
export function Sticker({id}:{id:string}){const s=stickers.find(s=>s[0]===id);return s?<span class={`comment-sticker sticker-${s[0]}`} role="img" aria-label={s[2]}><span aria-hidden="true">{s[1]}</span><strong>{s[2]}</strong></span>:null;}
export function CommentMedia({c}:{c:Comment}){const [error,setError]=useState<unknown>(null);return <div class="comment-media">
 {c.sticker&&<Sticker id={c.sticker}/>}
 {!!c.photos?.length&&<div class="comment-photo-grid">{c.photos.map(p=><a key={p.id} href={p.url} target="_blank" rel="noopener" data-external><img src={p.thumb_url} alt="Фото в комментарии" loading="lazy"/></a>)}</div>}
 {c.tracks?.map(t=><div key={t.id}><p>{t.artist} — {t.title}</p><audio controls preload="none" src={t.url}/></div>)}
 {c.videos?.map(v=><div key={v.id}><p>{v.title}</p><video controls playsInline preload="none" poster={v.poster_url}><source src={v.mp4_url||v.hls_url} type={v.mp4_url?"video/mp4":"application/vnd.apple.mpegurl"}/></video></div>)}
 {c.documents?.map(d=><button key={d.id} class="link" onClick={()=>saveFile(d.url,`${d.title}.${d.ext}`).catch(setError)}>📎 {d.title}.{d.ext}</button>)}<ErrorLine error={error}/>
 </div>;}

type Choice=(Photo|Doc|PostTrack|PostVideo)&{album_id?:number;status?:string};
type Kind="photos"|"tracks"|"videos"|"documents"|"stickers";
export function CommentExtras({value,onChange,disabled=false}:{value:CommentDraft;onChange:(v:CommentDraft)=>void;disabled?:boolean}){
 const [kind,setKind]=useState<Kind|null>(null),[error,setError]=useState<unknown>(null),[busy,setBusy]=useState(false),[query,setQuery]=useState(""),[album,setAlbum]=useState("");
 const dialog=useRef<HTMLDialogElement>(null);
 const total=value.photos.length+value.tracks.length+value.videos.length+value.documents.length;
 const urls={photos:"/photos?limit=50",tracks:`/users/${me()?.id}/music?limit=50`,videos:`/users/${me()?.id}/videos?limit=50`,documents:"/documents?limit=50"};
 const library=useLoad(async()=>{if(!kind||kind==="stickers")return {items:[] as Choice[],next_cursor:null as string|number|null};const page=await get<{items:Choice[];next_cursor:string|number|null}>(urls[kind]);return {...page,items:page.items??[]};},[kind]);
 const albums=useLoad(()=>get<{items:{id:number;title:string}[]}>("/photo-albums"),[]);
 useEffect(()=>{if(kind){setQuery("");setAlbum("");dialog.current?.showModal();}},[kind]);
 const close=()=>{if(busy)return;dialog.current?.close();setKind(null);setError(null);};
 function select(item:Choice){if(!kind||kind==="stickers"||total>=10)return;const current=value[kind] as Choice[];if(current.some(x=>x.id===item.id))return;onChange({...value,[kind]:[...current,item]});}
 async function more(){if(!kind||kind==="stickers"||!library.data?.next_cursor||busy)return;setBusy(true);try{const old=library.data;const page=await get<{items:Choice[];next_cursor:string|number|null}>(urls[kind]+`&${kind==="photos"?"after":"before"}=${old.next_cursor}`);library.set({...page,items:[...old.items,...page.items]});}catch(e){setError(e)}finally{setBusy(false)}}
 async function add(file:File|undefined){if(!file||busy||!kind||kind==="stickers")return;setBusy(true);setError(null);try{if(total>=10)throw new Error("Можно прикрепить до 10 вложений");const item=await upload<Choice>(kind==="photos"?"/media":"/documents",file);select(item);library.reload();}catch(e){setError(e)}finally{setBusy(false)}}
 return <div class="comment-extras">
 <div class="comment-attachment-toolbar" aria-label="Вложения к комментарию">{([["photos","Фото"],["tracks","Музыка"],["videos","Видео"],["documents","Файл"],["stickers","Стикеры"]] as [Kind,string][]).map(([id,label])=><button type="button" class="chip" disabled={disabled} onClick={()=>setKind(id)}>{label}</button>)}</div>
 <div class="comment-draft">{(["photos","tracks","videos","documents"] as const).flatMap(k=>value[k].map(x=><span class="chip" key={x.id}>{"thumb_url" in x?<img src={x.thumb_url} alt="Выбранное фото"/>:"title" in x?x.title:k}<button type="button" disabled={disabled} aria-label="Убрать вложение" onClick={()=>onChange({...value,[k]:value[k].filter(y=>y.id!==x.id)})}>×</button></span>))}{value.sticker&&<span><Sticker id={value.sticker}/><button type="button" disabled={disabled} onClick={()=>onChange({...value,sticker:""})}>Убрать стикер</button></span>}</div>
 <dialog class="comment-attachment-dialog" ref={dialog} aria-label="Вложения к комментарию" onCancel={e=>{e.preventDefault();close();}}><header><h2>{kind==="stickers"?"Стикеры socialnet":"Прикрепить к комментарию"}</h2><button type="button" disabled={busy} aria-label="Закрыть вложения" onClick={close}>×</button></header>
 <ErrorLine error={error||library.error}/>
 {kind==="stickers"?<div class="comment-sticker-grid">{stickers.map(([id,,label])=><button type="button" key={id} aria-label={label} onClick={()=>{onChange({...value,sticker:id});close();}}><Sticker id={id}/></button>)}</div>:<>
 <input type="search" aria-label="Поиск вложений" placeholder="Поиск по загруженным файлам" value={query} onInput={e=>setQuery(e.currentTarget.value)}/>
 {kind==="photos"&&<label>Альбом<select value={album} onChange={e=>setAlbum(e.currentTarget.value)}><option value="">Все фотографии</option>{albums.data?.items?.map(a=><option value={a.id}>{a.title}</option>)}</select></label>}
 {(kind==="photos"||kind==="documents")&&<label class="file-btn">Загрузить с устройства<input type="file" aria-label="Файл для комментария" disabled={busy||total>=10} accept={kind==="photos"?"image/jpeg,image/png,image/gif":undefined} onChange={e=>{void add(e.currentTarget.files?.[0]);e.currentTarget.value="";}}/></label>}
 {(kind==="tracks"||kind==="videos")&&<details><summary>Загрузить с устройства</summary><MediaUpload kind={kind==="tracks"?"music":"video"} onUploaded={library.reload}/></details>}
 {kind==="videos"&&<p class="muted">Выбрать можно обработанные видео. <button type="button" class="link" onClick={library.reload}>Обновить список</button></p>}
 {!library.data?<Loading/>:<div class="comment-library">{library.data.items.filter(x=>(!("status" in x)||x.status==="ready")&&(!album||x.album_id===Number(album))&&(!query||("title" in x?x.title:"").toLocaleLowerCase().includes(query.toLocaleLowerCase()))).map(x=><button type="button" key={x.id} disabled={total>=10||busy||(kind!==null&&(value[kind] as Choice[]).some(y=>y.id===x.id))} onClick={()=>select(x)}>{"thumb_url" in x?<img src={x.thumb_url} alt="Фотография"/>:"poster_url" in x?<img src={x.poster_url} alt=""/>:null}<span>{"title" in x?x.title:"Фотография"}</span></button>)}{!library.data.items.length&&<p>В этой библиотеке пока ничего нет. Загрузите файл с устройства.</p>}</div>}
 {library.data?.next_cursor&&<button type="button" disabled={busy} onClick={more}>Показать ещё</button>}<p class="muted">Выбрано {total} из 10</p>
 </>}
 <button type="button" class="primary" disabled={busy} onClick={close}>Готово</button></dialog>
 </div>;
}
