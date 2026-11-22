import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { fetchBlob, fileSize } from "./api.js";
import { ErrorLine } from "./ui.js";
interface Clip {id:string;title:string;mp4_url?:string}
const prefix="sn-offline-video-";
const key=(id:string)=>`/offline/video/${encodeURIComponent(id)}`;
export function VideoDownloads({video,userId}:{video:Clip;userId:number}) {
 const [busy,setBusy]=useState(false),[saved,setSaved]=useState(false),[error,setError]=useState<unknown>(null);
 useEffect(()=>{let alive=true;setSaved(false);if(typeof caches!=="undefined")void caches.open(prefix+userId).then(c=>c.match(key(video.id))).then(r=>{if(alive)setSaved(!!r);}).catch(setError);return()=>{alive=false};},[video.id,userId]);
 async function run(offline:boolean){setBusy(true);setError(null);try{
  const cache=typeof caches!=="undefined"?await caches.open(prefix+userId):null;
  if(offline&&!cache)throw new Error("Для офлайн-сохранения нужен HTTPS или localhost.");
  if(offline&&saved){await cache!.delete(key(video.id));setSaved(false);}else{
   const hit=await cache?.match(key(video.id));
   if(!hit&&!video.mp4_url)throw new Error("MP4 ещё не готов. Дождитесь обработки видео.");
   const blob=hit?await hit.blob():await fetchBlob(video.mp4_url!);
   if(!blob.size)throw new Error("Видео пустое.");
   if(offline){await cache!.put(key(video.id),new Response(blob,{headers:{"Content-Type":"video/mp4","X-Video":encodeURIComponent(JSON.stringify(video))}}));setSaved(true);}else{const url=URL.createObjectURL(blob);const a=document.createElement("a");a.href=url;a.download=video.title.replace(/[\\/:*?"<>|]/g,"_")+".mp4";a.click();setTimeout(()=>URL.revokeObjectURL(url),60000);}
  }
  window.dispatchEvent(new Event("offline-video-change"));
 }catch(e){setError(e);}finally{setBusy(false);}}
 return <div class="music-download-actions"><button disabled={busy} onClick={()=>run(false)}>Скачать MP4</button><button disabled={busy} onClick={()=>run(true)}>{busy?"Подождите…":saved?"Удалить офлайн":"Сохранить офлайн"}</button><ErrorLine error={error}/></div>;
}
export function OfflineVideos(){
 const [items,setItems]=useState<{clip:Clip;user:number;size:number}[]>([]),[playing,setPlaying]=useState<{url:string;title:string}|null>(null),[error,setError]=useState<unknown>(null);
 useEffect(()=>{let alive=true;async function load(){try{const result=[];if(typeof caches!=="undefined")for(const name of await caches.keys()){if(!name.startsWith(prefix))continue;const c=await caches.open(name);for(const req of await c.keys()){const r=await c.match(req);if(r)result.push({clip:JSON.parse(decodeURIComponent(r.headers.get("X-Video")||"")),user:Number(name.slice(prefix.length)),size:(await r.blob()).size});}}if(alive)setItems(result);}catch(e){if(alive)setError(e);}}void load();window.addEventListener("offline-video-change",load);return()=>{alive=false;window.removeEventListener("offline-video-change",load);};},[]);
 useEffect(()=>()=>{if(playing)URL.revokeObjectURL(playing.url);},[playing]);
 async function play(clip:Clip,user:number){try{const r=await(await caches.open(prefix+user)).match(key(clip.id));if(!r)throw new Error("Сохранённая копия удалена.");setPlaying({url:URL.createObjectURL(await r.blob()),title:clip.title});}catch(e){setError(e);}}
 return <section><h2>Видео офлайн · {items.length}</h2><p>Копии хранятся на этом устройстве и удаляются при выходе из аккаунта или очистке браузера. Скачанный MP4 можно открыть отдельно.</p><ErrorLine error={error}/>{playing&&<div><h3>{playing.title}</h3><video key={playing.url} controls autoPlay src={playing.url} style="width:100%;max-height:65vh"/></div>}{items.map(x=><article key={x.user+":"+x.clip.id}><button onClick={()=>play(x.clip,x.user)}>▶ {x.clip.title}</button> <small>{fileSize(x.size)}</small><VideoDownloads video={x.clip} userId={x.user}/></article>)}{!items.length&&<p>Сохраните видео в разделе «Видео» при подключении к сети.</p>}</section>;
}
