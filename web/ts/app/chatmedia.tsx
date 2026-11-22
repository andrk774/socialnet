import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { fetchBlob, get, post, upload } from "./api.js";
import { ErrorLine } from "./ui.js";

export type VideoMessage = {id:string; duration_sec:number; url:string};
export function PrivateVideo({url}:{url:string}) {
  const [src,setSrc]=useState(""); const [error,setError]=useState<unknown>(null); const [busy,setBusy]=useState(false);
  const object=useRef(""); const alive=useRef(true);
  useEffect(()=>()=>{alive.current=false;URL.revokeObjectURL(object.current);},[]);
  async function load(){if(busy)return;setBusy(true);try{const blob=await fetchBlob(url);if(alive.current){object.current=URL.createObjectURL(blob);setSrc(object.current);}}catch(e){if(alive.current)setError(e);}finally{if(alive.current)setBusy(false);}}
  return <div class="chat-video"><ErrorLine error={error}/>{src?<video controls playsInline preload="metadata" src={src}/>:<button disabled={busy} onClick={load}>{busy?"Загрузка…":"▶ Видеосообщение"}</button>}</div>;
}

export function VideoRecorder({done,close}:{done:(v:VideoMessage)=>void;close:()=>void}) {
  const [state,setState]=useState("idle"); const [seconds,setSeconds]=useState(0); const [blob,setBlob]=useState<Blob|null>(null);
  const [src,setSrc]=useState(""); const [error,setError]=useState<unknown>(null);
  const camera=useRef<HTMLVideoElement>(null);const rec=useRef<MediaRecorder|null>(null);const alive=useRef(true);
  const timer=useRef<ReturnType<typeof setInterval>>();const stream=useRef<MediaStream|null>(null);
  useEffect(()=>()=>{alive.current=false;clearInterval(timer.current);if(rec.current){rec.current.onstop=null;if(rec.current.state!=="inactive")rec.current.stop();}stream.current?.getTracks().forEach(t=>t.stop());},[]);
  useEffect(()=>{if(!blob){setSrc("");return;}const u=URL.createObjectURL(blob);setSrc(u);return()=>URL.revokeObjectURL(u);},[blob]);
  async function start(){setError(null);setBlob(null);setState("permission");try{
    const s=await navigator.mediaDevices.getUserMedia({audio:true,video:{width:{ideal:480},height:{ideal:480},facingMode:"user"}});
    if(!alive.current){s.getTracks().forEach(t=>t.stop());return;}stream.current=s;
    const mime=["video/webm;codecs=vp8,opus","video/mp4","video/webm"].find(x=>MediaRecorder.isTypeSupported(x));
    const r=new MediaRecorder(s,mime?{mimeType:mime,videoBitsPerSecond:800000}:undefined);rec.current=r;
    let size=0;const chunks:Blob[]=[];r.ondataavailable=e=>{chunks.push(e.data);size+=e.data.size;if(size>45*1024*1024&&r.state!=="inactive")r.stop();};
    r.onstop=()=>{clearInterval(timer.current);s.getTracks().forEach(t=>t.stop());if(alive.current){setBlob(new Blob(chunks,{type:r.mimeType}));setState("preview");}};
    r.start(500);setSeconds(0);setState("recording");if(camera.current){camera.current.srcObject=s;void camera.current.play().catch(()=>{});}
    const start=Date.now();timer.current=setInterval(()=>{const n=Math.floor((Date.now()-start)/1000);setSeconds(n);if(n>=59&&r.state!=="inactive")r.stop();},250);
  }catch(e){stream.current?.getTracks().forEach(t=>t.stop());setError(e);setState("idle");}}
  async function save(){if(!blob||state==="uploading")return;setState("uploading");setError(null);try{const v=await upload<VideoMessage>("/video-messages",new File([blob],blob.type.includes("mp4")?"message.mp4":"message.webm",{type:blob.type}));if(alive.current)done(v);}catch(e){if(alive.current){setError(e);setState("preview");}}}
  return <section class="music-panel chat-video" aria-label="Запись видеосообщения"><h2>Видеосообщение</h2><p>До 60 секунд и 50 МБ. Перед отправкой запись можно посмотреть.</p><ErrorLine error={error}/>
    <video ref={camera} hidden={state!=="recording"} muted playsInline/>
    {src&&<video src={src} controls playsInline/>}
    {state==="recording"?<button onClick={()=>rec.current?.stop()}>■ Завершить · {seconds} с</button>:<button disabled={state==="permission"||state==="uploading"} onClick={start}>{blob?"Записать заново":"Включить камеру"}</button>}
    <label>Выбрать видео <input type="file" accept="video/mp4,video/webm,video/quicktime" disabled={state==="recording"||state==="permission"||state==="uploading"} onChange={e=>{const f=e.currentTarget.files?.[0];if(f){if(f.size>50*1024*1024){setError(new Error("Максимум 50 МБ"));return;}setBlob(f);setState("preview");}}}/></label>
    {blob&&state!=="recording"&&<button disabled={state==="uploading"||state==="permission"} onClick={save}>{state==="uploading"?"Обработка…":"Прикрепить к сообщению"}</button>}<button onClick={close}>Отмена</button>
  </section>;
}

type Gift={id:number;name:string;price:number};
export function ChatGifts({recipient,close}:{recipient:number;close:()=>void}) {
  const [items,setItems]=useState<Gift[]>([]);const [gift,setGift]=useState<Gift|null>(null);const [text,setText]=useState("");const [busy,setBusy]=useState(false);const [sent,setSent]=useState(false);const [error,setError]=useState<unknown>(null);
  const key=useRef(crypto.randomUUID());const locked=useRef(false);const payload=useRef<{gift_id:number;message:string;anonymous:boolean;idempotency_key:string}|null>(null);
  useEffect(()=>{void get<{items:Gift[]}>("/gifts/catalog").then(r=>setItems(r.items||[]),setError);},[]);
  async function send(){if(!gift||locked.current)return;locked.current=true;setBusy(true);setError(null);payload.current??={gift_id:gift.id,message:text,anonymous:false,idempotency_key:key.current};try{await post(`/users/${recipient}/gifts`,payload.current);setSent(true);}catch(e){setError(e);}finally{locked.current=false;setBusy(false);}}
  return <section class="music-panel" aria-label="Отправить подарок"><h2>Подарок собеседнику</h2><ErrorLine error={error}/>{sent?<p role="status">Подарок отправлен. Он появится в подарках профиля получателя; получатель получит уведомление.</p>:<div>{items.map(g=><button disabled={busy||!!gift} onClick={()=>setGift(g)}>🎁 {g.name} · {g.price} монет</button>)}{gift&&<div><p>{gift.name}. С баланса будет списано {gift.price} монет.</p><textarea aria-label="Поздравление" maxLength={200} value={text} disabled={busy||!!payload.current} onInput={e=>setText(e.currentTarget.value)}/><button disabled={busy} onClick={send}>{busy?"Отправка…":`Подтвердить за ${gift.price} монет`}</button></div>}</div>}<button disabled={busy} onClick={close}>Закрыть</button></section>;
}

export const chatServices=[{name:"Музыка",path:"/music"},{name:"Видео",path:"/videos"},{name:"Фотографии",path:"/photos"},{name:"Люди рядом",path:"/nearby"}];
export function ServiceCard({body}:{body:string}) {
  const service=chatServices.find(s=>body.endsWith(`Сервис socialnet: ${s.name}\n${s.path}`));
  const prefix=service?body.slice(0,-`Сервис socialnet: ${service.name}\n${service.path}`.length).trim():"";
  return service?<div>{prefix&&<p>{prefix}</p>}<a class="chat-service-card" href={service.path}>▦ {service.name} →</a></div>:<p>{body}</p>;
}
