import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Photo, Self, del, fullName, patch, put } from "./api.js";
import { Avatar, ErrorLine, Time } from "./ui.js";
import { setMe, useMe } from "./session.js";
import { PhotoComments } from "./photocomments.js";

export interface DetailedPhoto extends Photo {created_at:string;description:string}
export function LibraryViewer({photo,index,total,album,onStep,onClose,onChanged}:{photo:DetailedPhoto;index:number;total:number;album:string;onStep:(n:number)=>void;onClose:()=>void;onChanged:()=>void}) {
  const me=useMe();const dialog=useRef<HTMLDialogElement>(null);const menu=useRef<HTMLDetailsElement>(null);
  const [editing,setEditing]=useState(false);const [text,setText]=useState(photo.description??"");const [error,setError]=useState<unknown>(null);const [busy,setBusy]=useState(false);const [notice,setNotice]=useState("");
  useEffect(()=>{dialog.current?.showModal();const overflow=document.body.style.overflow;document.body.style.overflow="hidden";return()=>{document.body.style.overflow=overflow;};},[]);
  useEffect(()=>{setEditing(false);setText(photo.description??"");setError(null);setNotice("");if(menu.current)menu.current.open=false;},[photo.id,photo.description]);
  useEffect(()=>{const outside=(e:PointerEvent)=>{if(menu.current&&!menu.current.contains(e.target as Node))menu.current.open=false;};document.addEventListener("pointerdown",outside);return()=>document.removeEventListener("pointerdown",outside);},[]);
  async function act(action:()=>Promise<void>){if(busy)return;setBusy(true);setError(null);try{await action();}catch(e){setError(e);}finally{setBusy(false);}}
  return <dialog ref={dialog} class="library-viewer detailed-viewer" aria-label="Просмотр фотографии" onClose={onClose} onCancel={e=>{if(busy)e.preventDefault();}} onClick={e=>{if(e.target===e.currentTarget&&!busy)dialog.current?.close();}} onKeyDown={e=>{if((e.target as HTMLElement).closest("input,textarea,select")||busy)return;if(e.key==="ArrowRight"){e.preventDefault();onStep(1);}if(e.key==="ArrowLeft"){e.preventDefault();onStep(-1);}}}>
    <button class="photo-view-close" disabled={busy} aria-label="Закрыть фото" onClick={()=>dialog.current?.close()}>×</button>
    <div class="photo-view-media"><div class="photo-view-image"><img src={photo.url} alt={photo.description||"Фотография в полном размере"}/><button class="photo-view-prev" disabled={busy||total<2} aria-label="Предыдущее фото" onClick={()=>onStep(-1)}>‹</button><button class="photo-view-next" disabled={busy||total<2} aria-label="Следующее фото" onClick={()=>onStep(1)}>›</button></div>
      <footer class="photo-view-footer"><span>{album} <small>{index+1} из {total}</small></span><button disabled={busy} onClick={()=>void act(async()=>{await navigator.clipboard.writeText(new URL(photo.url,location.origin).href);setNotice("Ссылка на оригинал скопирована.");})}>Поделиться</button><button disabled={busy} onClick={()=>{if(confirm("Удалить фотографию? Она также исчезнет из записей и сообщений; если это аватар, он будет сброшен."))void act(async()=>{await del(`/media/${photo.id}`);if(me?.avatar_url?.includes(photo.id))setMe({...me,avatar_url:null});onChanged();onClose();});}}>Удалить</button>
      <details ref={menu} class="photo-view-menu"><summary>Ещё ⌄</summary><div><button disabled={busy} onClick={()=>void act(async()=>{setMe(await put<Self>("/users/me/avatar",{media_id:photo.id}));setNotice("Фотография профиля обновлена.");if(menu.current)menu.current.open=false;})}>Сделать фотографией профиля</button><a href={photo.url} target="_blank" rel="noopener noreferrer">Открыть оригинал</a></div></details></footer>
    </div>
    <aside class="photo-view-details">{me&&<header><Avatar user={me} size={48}/><div><a href={`/u/${me.username}`}>{fullName(me)}</a><Time iso={photo.created_at}/></div></header>}
      <div class="photo-view-description">{editing?<form class="stack" onSubmit={e=>{e.preventDefault();void act(async()=>{await patch(`/photos/${photo.id}`,{description:text});setEditing(false);onChanged();});}}><label>Описание фотографии<textarea value={text} maxLength={2000} onInput={e=>setText(e.currentTarget.value)} /></label><div class="library-actions"><button class="primary" disabled={busy}>Сохранить</button><button class="ghost" type="button" disabled={busy} onClick={()=>{setText(photo.description??"");setEditing(false);}}>Отмена</button></div></form>:<><p>{photo.description}</p><button class="link" onClick={()=>setEditing(true)}>Редактировать описание</button></>}<ErrorLine error={error}/><p role="status">{notice}</p></div>
      <PhotoComments key={photo.id} id={photo.id}/>
    </aside>
  </dialog>;
}
