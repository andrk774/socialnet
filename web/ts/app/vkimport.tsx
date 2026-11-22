import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { ApiError, get, post, upload } from "./api.js";
import { me } from "./session.js";
import { ErrorLine } from "./ui.js";

interface VKTrack {key:string;title:string;artist:string;playlists:string[];available:boolean;reason:string}
interface Progress {tracks:Record<string,string>;playlists:Record<string,number>}
const uuid=/^[a-f0-9-]{36}$/i;

function bridge(op:"catalogue"|"audio",key?:string,signal?:AbortSignal):Promise<any> {
  return new Promise((resolve,reject)=>{
    const id=crypto.randomUUID();const chunks:Uint8Array[]=[];let size=0,index=0;
    const finish=(error?:Error,value?:unknown)=>{clearTimeout(timer);window.removeEventListener("message",receive);signal?.removeEventListener("abort",cancel);if(error){window.postMessage({channel:"socialnet-vk:request",id,op:"cancel"},location.origin);reject(error);}else resolve(value);};
    const cancel=()=>finish(new Error("Импорт остановлен"));
    const receive=(e:MessageEvent)=>{
      const m=e.data;if(e.source!==window||e.origin!==location.origin||m?.channel!=="socialnet-vk:response"||m.id!==id)return;
      try{
        if(m.type==="error")finish(new Error(typeof m.error==="string"?m.error:"Ошибка расширения"));
        else if(op==="catalogue"&&m.type==="catalogue")finish(undefined,m.items);
        else if(op==="audio"&&m.type==="chunk"){
          if(m.index!==index++||typeof m.data!=="string"||m.data.length>90000)throw Error("Повреждённый ответ расширения");
          const decoded=atob(m.data);size+=decoded.length;if(size>50*1024*1024)throw Error("Максимум 50 МБ на трек");
          chunks.push(Uint8Array.from(decoded,c=>c.charCodeAt(0)));
        }else if(op==="audio"&&m.type==="done"){
          if(size!==m.bytes||!size||!["mp3","m4a","ogg","flac","wav"].includes(m.ext))throw Error("Аудиофайл получен не полностью");
          finish(undefined,new File(chunks as BlobPart[],`vk_${key}.${m.ext}`,{type:m.mime}));
        }
      }catch(e){finish(e instanceof Error?e:new Error("Неверный ответ расширения"));}
    };
    const timer=setTimeout(()=>finish(new Error(op==="catalogue"?"Расширение не отвечает. Установите его, разрешите перенос в эту вкладку и повторите.":"Истекло время загрузки трека. Обновите список в VK.")),op==="catalogue"?8000:125000);
    window.addEventListener("message",receive);signal?.addEventListener("abort",cancel,{once:true});
    if(signal?.aborted){cancel();return;}
    window.postMessage({channel:"socialnet-vk:request",id,op,key},location.origin);
  });
}

export function VKMusicImport({done}:{done:()=>void}) {
  const [items,setItems]=useState<VKTrack[]>([]);const [selected,setSelected]=useState<string[]>([]);
  const [busy,setBusy]=useState(false);const [reading,setReading]=useState(false);const [error,setError]=useState<unknown>(null);
  const [results,setResults]=useState<string[]>([]);const [status,setStatus]=useState("");
  const control=useRef<AbortController|null>(null);const running=useRef(false);const mounted=useRef(true);
  useEffect(()=>()=>{mounted.current=false;control.current?.abort();},[]);
  async function load(){if(reading||running.current)return;setReading(true);setError(null);try{
    const raw=await bridge("catalogue");
    if(!Array.isArray(raw)||raw.length>5000)throw Error("Неверный список расширения");
    const parsed=raw.map((t:any)=>{
      if(typeof t.key!=="string"||! /^-?\d+_\d+$/.test(t.key)||typeof t.title!=="string"||!t.title||t.title.length>200||typeof t.artist!=="string"||t.artist.length>200||!Array.isArray(t.playlists)||t.playlists.length>20||t.playlists.some((p:unknown)=>typeof p!=="string"||!p||p.length>100))throw Error("Некорректные данные трека");
      return {key:t.key,title:t.title,artist:t.artist,playlists:t.playlists,available:t.available===true,reason:typeof t.reason==="string"?t.reason:"Нет доступного файла"} as VKTrack;
    });
    if(mounted.current){setItems(parsed);setSelected([]);setResults([]);setStatus(parsed.length?"Выберите записи для переноса.":"Список пуст. Сначала добавьте записи из VK через значок расширения.");}
  }catch(e){if(mounted.current)setError(e);}finally{if(mounted.current)setReading(false);}}
  function exportList(){const data=items.map(t=>({file:`vk_${t.key}.mp3`,title:t.title,artist:t.artist,playlists:t.playlists,vk_id:t.key}));const url=URL.createObjectURL(new Blob([JSON.stringify(data,null,2)],{type:"application/json"}));const a=document.createElement("a");a.href=url;a.download="vk-music-library.json";a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}
  async function run(){
    const user=me()?.id;if(running.current||!user||!selected.length)return;
    running.current=true;setBusy(true);setError(null);setResults([]);control.current=new AbortController();
    const lines:string[]=[];let success=0;const storageKey=`sn-vk-import-${user}`;
    try{
      const saved=JSON.parse(localStorage.getItem(storageKey)||'{"tracks":{},"playlists":{}}');
      const progress:Progress={tracks:{},playlists:{}};
      for(const [key,value] of Object.entries(saved.tracks||{}))if(typeof value==="string"&&uuid.test(value))progress.tracks[key]=value;
      for(const [key,value] of Object.entries(saved.playlists||{}))if(typeof value==="number"&&Number.isSafeInteger(value)&&value>0)progress.playlists[key]=value;
      const checkpoint=()=>localStorage.setItem(storageKey,JSON.stringify(progress));checkpoint();
      const check=()=>{if(control.current?.signal.aborted||me()?.id!==user)throw Error("Импорт остановлен");};
      for(const t of items.filter(t=>selected.includes(t.key)&&t.available)){
        if(control.current.signal.aborted||me()?.id!==user)break;
        try{
          setStatus(`Перенос: ${t.artist} — ${t.title}`);let tid=progress.tracks[t.key];
          if(tid){try{await get(`/tracks/${tid}`);}catch(e){if(e instanceof ApiError&&e.status===404){delete progress.tracks[t.key];tid="";}else throw e;}}
          if(!tid){const file=await bridge("audio",t.key,control.current.signal) as File;check();const uploaded=await upload<{id:string}>("/tracks",file,{title:t.title,artist:t.artist});tid=uploaded.id;progress.tracks[t.key]=tid;checkpoint();}
          for(const name of t.playlists){
            check();let pid=progress.playlists[name];
            if(pid){try{await get(`/playlists/${pid}`);}catch(e){if(e instanceof ApiError&&e.status===404){delete progress.playlists[name];pid=0;}else throw e;}}
            if(!pid){pid=(await post<{id:number}>("/playlists",{title:name})).id;progress.playlists[name]=pid;checkpoint();}
            check();await post(`/playlists/${pid}/tracks`,{track_id:tid});
          }
          success++;lines.push(`${t.artist} — ${t.title}: перенесён`);
        }catch(e){lines.push(`${t.title}: ${e instanceof Error?e.message:"ошибка"}`);}
        if(mounted.current)setResults([...lines]);
      }
      if(mounted.current)setStatus(`${control.current.signal.aborted?"Остановлено":"Готово"}. Перенесено: ${success}. Проверьте отчёт ниже.`);
      if(me()?.id===user)done();
    }catch(e){if(mounted.current)setError(e);}finally{running.current=false;if(mounted.current)setBusy(false);}
  }
  return <details class="music-panel vk-import"><summary>Импорт из открытого VK</summary>
    <p>Перенос через расширение в вашем браузере. Пароль и cookies VK в socialnet не передаются.</p>
    <ol><li><a href="/static/downloads/socialnet-vk-import.zip" download>Скачать расширение</a>, распаковать. В Chrome: chrome://extensions → режим разработчика → «Загрузить распакованное расширение».</li><li>Открыть свою музыку в VK, прокрутить список и нажать значок расширения → «Добавить записи». Для плейлистов повторить на их страницах, указав название.</li><li>Вернуться в эту вкладку, в расширении нажать «Разрешить перенос», затем получить список ниже.</li></ol>
    <p class="muted">Поддерживаются localhost:8080 и 127.0.0.1:8080. Собираются только загруженные на странице записи. Аудиофайлы HLS, защищённые и недоступные ссылки пока не переносятся. Названия таких треков можно сохранить в JSON.</p>
    <button disabled={busy||reading} onClick={load}>{reading?"Получаем список…":"Получить список из VK"}</button>
    <ErrorLine error={error}/><p role="status">{status}</p>
    {!!items.length&&<div><p>Всего: {items.length}. Доступен прямой файл: {items.filter(t=>t.available).length}. Выбрано: {selected.length}.</p>
      <button disabled={busy} onClick={()=>setSelected(items.filter(t=>t.available).map(t=>t.key))}>Выбрать доступные</button><button disabled={busy} onClick={()=>setSelected([])}>Снять выбор</button><button onClick={exportList}>Сохранить названия и плейлисты в JSON</button>
      <div class="vk-import-list">{items.map(t=><label key={t.key}><input type="checkbox" disabled={busy||!t.available} checked={selected.includes(t.key)} onChange={e=>setSelected(old=>e.currentTarget.checked?[...old,t.key]:old.filter(k=>k!==t.key))}/><span><strong>{t.artist} — {t.title}</strong><small>{t.playlists.join(" · ")}{!t.available&&` · ${t.reason}`}</small></span></label>)}</div>
      <p class="muted">До 50 МБ на трек. Прогресс сохраняется в этом браузере для текущего аккаунта. При потере ответа сервера повтор может создать дубликат — проверьте библиотеку. Остановка завершает уже начатую загрузку в socialnet.</p>
      <button disabled={busy||!selected.length} onClick={run}>{busy?"Перенос…":"Импортировать выбранные"}</button>{busy&&<button onClick={()=>{control.current?.abort();setStatus("Останавливаем перенос…");}}>Остановить</button>}
    </div>}<ul>{results.map((line,i)=><li key={i}>{line}</li>)}</ul>
  </details>;
}
