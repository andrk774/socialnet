import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { fetchBlob, fileSize } from "./api.js";
import { ErrorLine, duration } from "./ui.js";

interface Track { id:string; title:string; artist:string; url:string; duration_sec:number }
const prefix="sn-offline-music-";
const key=(id:string)=>`/offline/music/${encodeURIComponent(id)}`;
function download(blob:Blob,t:Track){const url=URL.createObjectURL(blob);const a=document.createElement("a");a.href=url;a.download=`${t.artist} - ${t.title}`.replace(/[\\/:*?"<>|]/g,"_")+".mp3";a.click();setTimeout(()=>URL.revokeObjectURL(url),60000);}
export function MusicDownloads({track:t,userId}:{track:Track;userId:number}){
 const [saved,setSaved]=useState(false),[busy,setBusy]=useState(false),[error,setError]=useState<unknown>(null);
 useEffect(()=>{if(typeof caches!=="undefined")void caches.open(prefix+userId).then(c=>c.match(key(t.id))).then(r=>setSaved(!!r)).catch(()=>{});},[t.id,userId]);
 async function run(save:boolean){setBusy(true);setError(null);try{
  if(save&&typeof caches==="undefined")throw new Error("Сохранение офлайн доступно по HTTPS или на localhost.");
  const cache=typeof caches!=="undefined"?await caches.open(prefix+userId):null;
  if(save&&saved){await cache!.delete(key(t.id));setSaved(false);}else{
   const hit=await cache?.match(key(t.id));const blob=hit?await hit.blob():await fetchBlob(t.url);
   if(!blob.size)throw new Error("Аудиофайл пуст.");
   if(save){await cache!.put(key(t.id),new Response(blob,{headers:{"Content-Type":blob.type||"audio/mpeg","X-Track":encodeURIComponent(JSON.stringify(t))}}));setSaved(true);}else download(blob,t);
  }
  window.dispatchEvent(new Event("offline-music-change"));
 }catch(e){setError(e);}finally{setBusy(false);}}
 return <div class="music-download-actions"><button class="link" disabled={busy} onClick={()=>run(false)}>Скачать файл</button><button class="link" disabled={busy} aria-pressed={saved} onClick={()=>run(true)}>{busy?"Загрузка…":saved?"Удалить офлайн":"Сохранить офлайн"}</button><ErrorLine error={error}/></div>;
}
interface Saved {track:Track;cache:string;size:number}
export function OfflineMusic({userId}:{userId?:number}){
 const [items,setItems]=useState<Saved[]>([]),[error,setError]=useState<unknown>(null),[playing,setPlaying]=useState<{url:string;title:string}|null>(null);
 useEffect(()=>{let alive=true;async function load(){try{const result:Saved[]=[];if(typeof caches!=="undefined")for(const name of await caches.keys()){if(!name.startsWith(prefix)||(userId!==undefined&&name!==prefix+userId))continue;const c=await caches.open(name);for(const req of await c.keys()){const r=await c.match(req);if(r){const track=JSON.parse(decodeURIComponent(r.headers.get("X-Track")||""));result.push({track,cache:name,size:(await r.blob()).size});}}}if(alive)setItems(result);}catch(e){if(alive)setError(e);}}void load();window.addEventListener("offline-music-change",load);return()=>{alive=false;window.removeEventListener("offline-music-change",load);};},[userId]);
 useEffect(()=>()=>{if(playing)URL.revokeObjectURL(playing.url);},[playing]);
 async function play(s:Saved){try{const r=await(await caches.open(s.cache)).match(key(s.track.id));if(!r)throw new Error("Копия удалена. Сохраните трек заново при подключении к сети.");setPlaying({url:URL.createObjectURL(await r.blob()),title:s.track.title});}catch(e){setError(e);}}
 return <section class="music-panel"><h2>Музыка офлайн · {items.length}</h2><p class="muted">Доступна на этом устройстве. Копии удаляются при выходе из аккаунта или очистке данных браузера. Браузер также может освободить место автоматически.</p><ErrorLine error={error}/>{playing&&<div><strong>{playing.title}</strong><audio key={playing.url} src={playing.url} controls autoPlay style="width:100%"/></div>}{!items.length&&<p>Нажмите «Сохранить офлайн» рядом с треком при подключении к интернету.</p>}<ul class="plain-list">{items.map(s=><li key={s.cache+s.track.id} class="offline-music-row"><button class="link" onClick={()=>play(s)}>▶ {s.track.artist} — {s.track.title}</button><small class="muted">{duration(s.track.duration_sec)} · {fileSize(s.size)}</small><MusicDownloads track={s.track} userId={Number(s.cache.slice(prefix.length))}/></li>)}</ul></section>;
}
