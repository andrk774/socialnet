import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Doc, Page, del, fileSize, get, patch, saveFile, upload } from "./api.js";
import { Empty, ErrorLine, Loading, Time, useLoad } from "./ui.js";

export function DocumentsPage(){
 const [q,setQ]=useState(""),[search,setSearch]=useState(""),[kind,setKind]=useState("all"),[busy,setBusy]=useState(false),[error,setError]=useState<unknown>(null),[loading,setLoading]=useState(false);
 useEffect(()=>{const timer=setTimeout(()=>setSearch(q.trim()),250);return()=>clearTimeout(timer);},[q]);
 const key=`${kind}:${search}`;
 const currentKey=useRef(key);currentKey.current=key;
 const url=`/documents?limit=50&kind=${kind}&q=${encodeURIComponent(search)}`;
 const list=useLoad(async()=>({...await get<Page<Doc>&{total:number}>(url),key}),[key]);
 const data=list.data?.key===key?list.data:undefined;
 async function add(files:FileList|null){if(!files||busy)return;setBusy(true);setError(null);try{for(const f of Array.from(files)){if(f.size>50*1024*1024)throw new Error(`Файл «${f.name}» превышает 50 МБ.`);await upload<Doc>("/documents",f);}}catch(e){setError(e);}finally{setBusy(false);list.reload();}}
 async function more(){if(!data?.next_cursor||loading)return;setLoading(true);try{const page=await get<Page<Doc>&{total:number}>(url+`&before=${data.next_cursor}`);if(currentKey.current===key)list.set({...page,key,items:[...data.items,...page.items]});}catch(e){if(currentKey.current===key)setError(e);}finally{setLoading(false);}}
 async function rename(d:Doc){const title=prompt("Новое название",d.title);if(!title?.trim())return;try{await patch(`/documents/${d.id}`,{title});list.reload();}catch(e){setError(e);}}
 async function remove(d:Doc){if(!confirm(`Удалить «${d.title}»? Файл пропадёт из записей и сообщений.`))return;try{await del(`/documents/${d.id}`);list.reload();}catch(e){setError(e);}}
 return <div class="files-layout"><section class="library-panel files-main"><header class="files-heading"><h1>Файлы {data&&<small class="muted">{data.total}</small>}</h1><label class="primary file-btn">{busy?"Загружаем…":"Загрузить файл"}<input type="file" multiple class="sr-only" disabled={busy} aria-label="Загрузить файл" accept=".pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.odt,.ods,.odp,.epub,.djvu,.rtf,.txt,.md,.csv,.zip,.rar,.7z,.gz,.gif" onChange={e=>{void add(e.currentTarget.files);e.currentTarget.value="";}}/></label></header>
 <input class="files-search" type="search" aria-label="Поиск файлов" placeholder="Поиск файлов" maxLength={120} value={q} onInput={e=>setQ(e.currentTarget.value)}/>
 <ErrorLine error={error||list.error}/>{!data&&!list.error?<Loading/>:data&&!data.items.length?<Empty>{search?"Файлы не найдены. Измените запрос или категорию.":"В этой категории файлов пока нет."}</Empty>:data&&<ul class="files-list">{data.items.map(d=><li key={d.id}><span class={`file-icon ${d.ext==="gif"?"file-gif":""}`} aria-hidden="true">{d.ext.toUpperCase()}</span><div class="file-info"><button class="link file-name" onClick={()=>saveFile(d.url,`${d.title}.${d.ext}`).catch(setError)}>{d.title}.{d.ext}</button><p class="muted">{fileSize(d.size)} · <Time iso={d.created_at}/></p></div><details class="file-menu"><summary aria-label={`Действия с файлом ${d.title}`}>•••</summary><div><button onClick={()=>rename(d)}>Переименовать</button><button onClick={()=>remove(d)}>Удалить</button></div></details></li>)}</ul>}
 {data?.next_cursor&&<button class="ghost" disabled={loading} onClick={more}>Показать ещё</button>}<p class="muted files-note">Документы, GIF и архивы — до 50 МБ. Скачать файл могут только пользователи, которым доступна запись или сообщение с ним.</p></section>
 <nav class="settings-navigation" aria-label="Категории файлов">{[["all","Все файлы"],["text","Текстовые"],["gif","GIF-изображения"],["archive","Архивы"]].map(([id,label])=><button class={kind===id?"active":""} aria-pressed={kind===id} onClick={()=>setKind(id)}>{label}</button>)}</nav></div>;
}
