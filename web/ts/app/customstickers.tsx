import { AISticker } from "./ai.js";
import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, post, upload, Photo } from "./api.js";
import { ErrorLine } from "./ui.js";
interface CustomSticker extends Photo {title:string}
export function CustomStickers({select}:{select:(sticker:CustomSticker)=>void}){
 const [items,setItems]=useState<CustomSticker[]>([]);const [title,setTitle]=useState("");const [file,setFile]=useState<File>();const [busy,setBusy]=useState(false);const [error,setError]=useState<unknown>(null);
 async function load(){setItems((await get<{items:CustomSticker[]}>("/stickers")).items)}
 useEffect(()=>{void load().catch(setError)},[]);
 async function create(){if(!file||!title.trim()||busy)return;setBusy(true);setError(null);try{const photo=await upload<Photo>("/media",file);await post("/stickers",{id:photo.id,title:title.trim()});setTitle("");setFile(undefined);await load()}catch(e){setError(e)}finally{setBusy(false)}}
 return <section class="custom-stickers"><AISticker created={p=>{void post("/stickers",{id:p.id,title:title.trim()||"AI sticker"}).then(load).catch(setError)}}/><h3>Мои стикеры</h3><ErrorLine error={error}/><div class="comment-sticker-grid">{items.map(x=><div key={x.id}><button type="button" disabled={busy} title={`Выбрать: ${x.title}`} onClick={()=>select(x)}><img src={x.thumb_url} alt={x.title} style="width:96px;height:96px;object-fit:contain"/></button><button type="button" class="link" disabled={busy} aria-label={`Удалить стикер ${x.title}`} onClick={async()=>{setBusy(true);try{await post("/stickers/remove",{id:x.id});await load()}catch(e){setError(e)}finally{setBusy(false)}}}>Удалить</button></div>)}</div>
 {!items.length&&<p class="muted">Загрузите картинку и дайте ей название. Она сохранится в вашем наборе.</p>}
 <label>Название стикера<input maxLength={64} value={title} onInput={e=>setTitle(e.currentTarget.value)}/></label>
 <label>Картинка (PNG, JPEG, GIF, до 10 МБ)<input type="file" accept="image/png,image/jpeg,image/gif" disabled={busy} onChange={e=>setFile(e.currentTarget.files?.[0])}/></label>
 <button type="button" disabled={busy||!file||!title.trim()} onClick={create}>{busy?"Загрузка…":"Создать стикер"}</button></section>;
}
