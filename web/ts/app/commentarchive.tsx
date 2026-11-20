import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { get, Page } from "./api.js";
import { ErrorLine } from "./ui.js";

interface OwnComment {id:number;post_id:number;text:string;created_at:string}
export function CommentArchivePage(){
 const [busy,setBusy]=useState(false),[count,setCount]=useState(0),[done,setDone]=useState(false),[error,setError]=useState<unknown>(null);
 const cancel=useRef(false);
 useEffect(()=>()=>{cancel.current=true;},[]);
 async function download(){
  cancel.current=false;setBusy(true);setCount(0);setDone(false);setError(null);
  try{
   const parts:BlobPart[]=["\uFEFFID;Post ID;Date UTC;Comment\r\n"];let after=0,total=0;
   const cell=(s:unknown)=>'"'+String(s).replace(/^[=+@\-\t\r]/,"'$&").replace(/"/g,'""')+'"';
   do{
    const page=await get<Page<OwnComment>>(`/users/me/comments?after=${after}`);
    if(cancel.current)return;
    parts.push(page.items.map(c=>[c.id,c.post_id,c.created_at,c.text].map(cell).join(";")).join("\r\n")+(page.items.length?"\r\n":""));
    total+=page.items.length;setCount(total);after=page.next_cursor||0;
   }while(after);
   const url=URL.createObjectURL(new Blob(parts,{type:"text/csv;charset=utf-8"}));const a=document.createElement("a");a.href=url;a.download="socialnet-my-comments.csv";a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);setDone(true);
  }catch(e){if(!cancel.current)setError(e);}finally{setBusy(false);}
 }
 return <article class="library-panel info-page"><a href="/help">← Помощь</a><h1>Как найти все мои комментарии?</h1><p>Скачайте сохранённые комментарии своего аккаунта socialnet в CSV. Файл открывается в Excel и других табличных редакторах.</p><ol><li>Нажмите «Скачать мои комментарии».</li><li>Дождитесь окончания выгрузки, не закрывая страницу.</li><li>Откройте файл и найдите нужную фразу поиском.</li></ol><p class="muted">В файл входят текст, дата, ID комментария и записи. Удалённые комментарии не восстанавливаются. Это выгрузка комментариев, а не полный архив аккаунта.</p><ErrorLine error={error}/><button class="primary" disabled={busy} onClick={download}>Скачать мои комментарии</button>{busy&&<button class="ghost" onClick={()=>{cancel.current=true;}}>Отменить</button>}<p role="status">{busy?`Загружено комментариев: ${count}`:done?`Файл подготовлен. Комментариев: ${count}`:""}</p></article>;
}
