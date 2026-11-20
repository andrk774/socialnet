import {h} from '../vendor/preact/index.js';
import {useState} from '../vendor/preact/hooks.js';
import {get,post} from './api.js';
import {ErrorLine,Loading,Time,useLoad} from './ui.js';
import {Icon} from './shellparts.js';
interface Comment {id:number;username:string;name:string;text:string;created_at:string}
interface Page {items:Comment[];next_cursor:number|null}
export function PhotoComments({id}:{id:string}){
 const data=useLoad(()=>get<Page>(`/photos/${id}/comments`),[id]);
 const [text,setText]=useState(''),[busy,setBusy]=useState(false),[error,setError]=useState<unknown>(null);
 return <section class="photo-comments" aria-label="Комментарии к фотографии">
  <div class="photo-comments-list" aria-live="polite">
   {!data.data&&!data.error&&<Loading/>}<ErrorLine error={data.error}/>
   {data.data?.items.length===0&&<div class="photo-comments-empty"><Icon name="message"/><p>Оставьте первый комментарий к этой фотографии</p></div>}
   {data.data?.items.map(c=><article key={c.id}><a href={`/u/${c.username}`}>{c.name||c.username}</a><p>{c.text}</p><Time iso={c.created_at}/></article>)}
   {data.data?.next_cursor&&<button class="link" disabled={busy} onClick={async()=>{setBusy(true);try{const old=data.data!;const page=await get<Page>(`/photos/${id}/comments?after=${old.next_cursor}`);data.set({...page,items:[...old.items,...page.items]});}catch(e){setError(e)}finally{setBusy(false)}}}>Показать следующие комментарии</button>}
  </div>
  <ErrorLine error={error}/>
  <form onSubmit={async e=>{e.preventDefault();if(busy||!text.trim())return;setBusy(true);setError(null);try{await post(`/photos/${id}/comments`,{text});setText('');data.reload();}catch(e){setError(e)}finally{setBusy(false)}}}>
   <textarea aria-label="Комментарий к фотографии" placeholder="Написать комментарий…" maxLength={2000} rows={2} value={text} disabled={busy} onInput={e=>setText(e.currentTarget.value)}/>
   <button class="primary" disabled={busy||!text.trim()} aria-label="Отправить комментарий">Отправить</button>
  </form>
 </section>;
}
