import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { api, get, post, fullName, UserCard, Page } from "./api.js";
import { me } from "./session.js";
import { on } from "./realtime.js";
import { Avatar, ErrorLine, Loading, Time, useLoad } from "./ui.js";

interface Call {id:number;caller_id:number;callee_id:number;video:boolean;status:string;offer?:string;answer?:string;peer:UserCard;created_at:string;answered_at?:string;ended_at?:string}
const live=(c:Call)=>c.status==="ringing"||c.status==="active";
const labels:Record<string,string>={ringing:"Вызов",active:"Принят",ended:"Завершён",declined:"Отклонён",missed:"Пропущен"};
export function startCall(peer:UserCard,video=false){window.dispatchEvent(new CustomEvent("socialnet-call",{detail:{peer,video}}));}
function StreamVideo({stream,muted=false}:{stream:MediaStream|null;muted?:boolean}){
 const ref=useRef<HTMLVideoElement>(null);
 useEffect(()=>{if(ref.current){ref.current.srcObject=stream;ref.current.play().catch(()=>{});}},[stream]);
 return <video ref={ref} autoPlay playsInline muted={muted} controls={!muted} aria-label={muted?"Ваша камера":"Собеседник"}/>;
}
async function gather(pc:RTCPeerConnection){
 if(pc.iceGatheringState==="complete")return;
 await new Promise<void>((resolve,reject)=>{const done=()=>{clearTimeout(timer);pc.removeEventListener("icegatheringstatechange",change);};const change=()=>{if(pc.iceGatheringState==="complete"){done();resolve();}};const timer=setTimeout(()=>{done();reject(new Error("Не удалось подготовить соединение. Проверьте сеть и TURN-сервер."));},15000);pc.addEventListener("icegatheringstatechange",change);change();});
}

export function CallManager(){
 const [call,setCall]=useState<Call|null>(null),[local,setLocal]=useState<MediaStream|null>(null),[remote,setRemote]=useState<MediaStream|null>(null);
 const [busy,setBusy]=useState(false),[error,setError]=useState<unknown>(null),[connection,setConnection]=useState(""),[muted,setMuted]=useState(false),[camera,setCamera]=useState(true);
 const dialog=useRef<HTMLDialogElement>(null);
 const visible=!!call||busy||!!error;
 useEffect(()=>{if(visible&&!dialog.current?.open)dialog.current?.showModal();},[visible]);
 const current=useRef<Call|null>(null),pc=useRef<RTCPeerConnection|null>(null),stream=useRef<MediaStream|null>(null),working=useRef(false),generation=useRef(0),connectTimer=useRef<number>(),lastBeat=useRef(0);
 function update(c:Call|null){current.current=c;setCall(c);}
 function cleanup(){generation.current++;clearTimeout(connectTimer.current);pc.current?.close();pc.current=null;stream.current?.getTracks().forEach(t=>t.stop());stream.current=null;setLocal(null);setRemote(null);setConnection("");setMuted(false);setCamera(true);working.current=false;setBusy(false);}
 async function end(action="end",keepError=false){
  if(!keepError)setError(null);
  const c=current.current;cleanup();update(null);if(c)try{await post(`/calls/${c.id}/action`,{action});}catch(e){setError(e);}window.dispatchEvent(new Event("calls-changed"));
 }
 async function prepare(video:boolean){
  if(!navigator.mediaDevices?.getUserMedia||!window.RTCPeerConnection)throw new Error("Для звонков откройте socialnet по HTTPS или на localhost в современном браузере.");
  const gen=generation.current;
  const config=await get<{ice_servers:RTCIceServer[]}>("/calls/config");
  const media=await navigator.mediaDevices.getUserMedia({audio:true,video});
  if(gen!==generation.current){media.getTracks().forEach(t=>t.stop());throw new Error("Вызов отменён");}
  stream.current=media;setLocal(media);
  const rtc=new RTCPeerConnection({iceServers:config.ice_servers});pc.current=rtc;
  media.getTracks().forEach(t=>rtc.addTrack(t,media));
  rtc.ontrack=e=>{if(pc.current===rtc)setRemote(e.streams[0]||new MediaStream([e.track]));};
  rtc.onconnectionstatechange=()=>{if(pc.current!==rtc)return;setConnection(rtc.connectionState);if(rtc.connectionState==="connected")clearTimeout(connectTimer.current);if(rtc.connectionState==="failed"){setError(new Error("Соединение прервано. Проверьте сеть; для разных сетей может понадобиться TURN."));void end("end",true);}};
  return rtc;
 }
 function connectionDeadline(){clearTimeout(connectTimer.current);connectTimer.current=window.setTimeout(()=>{if(pc.current?.connectionState!=="connected"){setError(new Error("Собеседник принял вызов, но медиасоединение не установлено."));void end("end",true);}},45000);}
 async function initiate(peer:UserCard,video:boolean){
  if(current.current||working.current){setError(new Error("Сначала завершите текущий вызов."));return;}
  working.current=true;setBusy(true);setError(null);
  const gen=generation.current;
  let createdID:number|undefined;
  try{const rtc=await prepare(video);await rtc.setLocalDescription(await rtc.createOffer());await gather(rtc);if(gen!==generation.current)return;
   const created=await post<{id:number}>(`/users/${peer.id}/calls`,{video,offer:rtc.localDescription!.sdp});
   createdID=created.id;
   if(gen!==generation.current){await post(`/calls/${created.id}/action`,{action:"end"});return;}
   update(await get<Call>(`/calls/${created.id}`));
  }catch(e){if(createdID)void post(`/calls/${createdID}/action`,{action:"end"}).catch(()=>{});if(gen===generation.current){setError(e);cleanup();}}finally{if(gen===generation.current){working.current=false;setBusy(false);}}
 }
 async function accept(){
  const c=current.current;if(!c||working.current)return;working.current=true;setBusy(true);setError(null);const gen=generation.current;
  try{const detail=await get<Call>(`/calls/${c.id}`);if(!detail.offer)throw new Error("Вызов уже завершён.");const rtc=await prepare(c.video);await rtc.setRemoteDescription({type:"offer",sdp:detail.offer});await rtc.setLocalDescription(await rtc.createAnswer());await gather(rtc);if(gen!==generation.current)return;await post(`/calls/${c.id}/action`,{action:"answer",answer:rtc.localDescription!.sdp});update({...c,status:"active"});connectionDeadline();}
  catch(e){if(gen===generation.current){setError(e);cleanup();}}finally{if(gen===generation.current){working.current=false;setBusy(false);}}
 }
 useEffect(()=>{
  let stopped=false,polling=false;
  const poll=async()=>{if(stopped||polling)return;polling=true;try{
   const c=current.current;
   if(c){const fresh=await get<Call>(`/calls/${c.id}`);if(stopped||current.current?.id!==c.id)return;
    if(!live(fresh)){cleanup();update(null);window.dispatchEvent(new Event("calls-changed"));return;}
    if(fresh.status==="active"&&!pc.current&&!working.current){cleanup();update(null);return;}
    update(fresh);
    if(fresh.status==="active"&&pc.current){if(fresh.caller_id===me()?.id&&fresh.answer&&!pc.current.remoteDescription){await pc.current.setRemoteDescription({type:"answer",sdp:fresh.answer});connectionDeadline();}
     if(Date.now()-lastBeat.current>10000){lastBeat.current=Date.now();await post(`/calls/${fresh.id}/action`,{action:"heartbeat"});}}
   }else if(!working.current){const list=await get<Page<Call>>("/calls");if(stopped||current.current||working.current)return;const incoming=list.items.find(x=>x.status==="ringing"&&x.callee_id===me()?.id);if(incoming)update(incoming);}
  }catch(e){if(current.current)setError(e);}finally{polling=false;}};
  const handler=(e:Event)=>{const d=(e as CustomEvent).detail;void initiate(d.peer,d.video);};window.addEventListener("socialnet-call",handler);
  const off=on("call",poll);const timer=setInterval(poll,2000);void poll();
  return()=>{stopped=true;clearInterval(timer);off();window.removeEventListener("socialnet-call",handler);const c=current.current,owned=!!pc.current;cleanup();if(c&&owned)void post(`/calls/${c.id}/action`,{action:"end"}).catch(()=>{});};
 },[]);
 if(!call&&!busy&&!error)return null;
 const incoming=!!call&&call.callee_id===me()?.id&&call.status==="ringing";
 return <dialog ref={dialog} class="call-overlay" aria-label="Звонок" onCancel={e=>{e.preventDefault();void end(incoming?"decline":"end");}}><div class="call-window">
  <h2>{call?fullName(call.peer):"Подготовка звонка"}</h2><p role="status">{busy?"Подготовка микрофона и соединения…":incoming?"Входящий "+(call?.video?"видеозвонок":"аудиозвонок"):connection==="connected"?"Соединение установлено":call?.status==="active"?"Соединяем…":call?"Ожидание ответа…":"Звонок завершён"}</p>
  <ErrorLine error={error}/><div class={call?.video?"call-streams":"call-streams audio-call"}><StreamVideo stream={remote}/>{local&&<StreamVideo stream={local} muted/>}</div>
  <div class="call-controls">{incoming&&<button class="primary" disabled={busy} onClick={accept}>Принять</button>}{local&&<><button class="ghost" aria-pressed={muted} onClick={()=>{stream.current?.getAudioTracks().forEach(t=>t.enabled=muted);setMuted(!muted);}}>{muted?"Включить микрофон":"Выключить микрофон"}</button>{call?.video&&<button class="ghost" aria-pressed={!camera} onClick={()=>{stream.current?.getVideoTracks().forEach(t=>t.enabled=!camera);setCamera(!camera);}}>{camera?"Выключить камеру":"Включить камеру"}</button>}</>}
  <button class="call-end" onClick={()=>void end(incoming?"decline":"end")}>{incoming?"Отклонить":call||busy?"Завершить":"Закрыть"}</button></div>
 </div></dialog>;
}

export function CallsPage(){
 const history=useLoad(()=>get<Page<Call>>("/calls"),[]),config=useLoad(()=>get<{relay_configured:boolean}>("/calls/config"),[]);
 const [filter,setFilter]=useState("all"),[q,setQ]=useState(""),[people,setPeople]=useState<UserCard[]>([]),[error,setError]=useState<unknown>(null),[loading,setLoading]=useState(false);
 useEffect(()=>{const fn=()=>history.reload();window.addEventListener("calls-changed",fn);const off=on("call",fn);return()=>{window.removeEventListener("calls-changed",fn);off();};},[]);
 useEffect(()=>{let active=true;setPeople([]);if(q.trim().length<2)return;const timer=setTimeout(()=>get<{items:UserCard[]}>(`/search?type=users&q=${encodeURIComponent(q.trim())}`).then(r=>{if(active)setPeople(r.items.filter(u=>u.id!==me()?.id));},e=>{if(active)setError(e);}),300);return()=>{active=false;clearTimeout(timer);};},[q]);
 async function more(){if(!history.data?.next_cursor||loading)return;setLoading(true);try{const page=await get<Page<Call>>(`/calls?before=${history.data.next_cursor}`);history.set({items:[...history.data.items,...page.items],next_cursor:page.next_cursor});}catch(e){setError(e);}finally{setLoading(false);}}
 return <section class="calls-page"><header class="library-panel"><h1>Звонки</h1><p>Личные аудио- и видеозвонки в socialnet</p><label>Кому позвонить?<input type="search" aria-label="Поиск собеседника" placeholder="Имя или username, от 2 символов" maxLength={64} value={q} onInput={e=>setQ(e.currentTarget.value)}/></label>{people.map(u=><div class="call-history-row"><Avatar user={u}/><a href={`/u/${u.username}`}>{fullName(u)}</a><button class="ghost" onClick={()=>startCall(u)}>Позвонить</button><button class="ghost" onClick={()=>startCall(u,true)}>Видео</button></div>)}{config.data&&!config.data.relay_configured&&<p class="muted">TURN-сервер не настроен. Соединение между разными сетями может быть недоступно.</p>}</header>
 <div class="library-panel"><div class="stats-toolbar"><h2>История звонков</h2><select aria-label="Фильтр звонков" value={filter} onChange={e=>setFilter(e.currentTarget.value)}><option value="all">Все</option><option value="active">Активные</option><option value="missed">Пропущенные</option></select><button class="ghost" onClick={async()=>{if(!confirm("Очистить историю завершённых звонков только у вас?"))return;try{await api("DELETE","/calls/history");history.reload();}catch(e){setError(e);}}}>Очистить историю</button></div>
 <ErrorLine error={error||history.error}/>{!history.data?<Loading/>:<>{history.data.items.filter(c=>filter==="all"||(filter==="active"?live(c):c.status==="missed"&&c.callee_id===me()?.id)).map(c=><div class="call-history-row" key={c.id}><Avatar user={c.peer}/><div><a href={`/u/${c.peer.username}`}>{fullName(c.peer)}</a><p class={c.status==="missed"?"call-missed":"muted"}>{c.caller_id===me()?.id?"↗":"↙"} {labels[c.status]} · <Time iso={c.created_at}/></p></div><button class="ghost" aria-label={`Позвонить ${fullName(c.peer)}`} onClick={()=>startCall(c.peer)}>☎</button><button class="ghost" aria-label={`Видеозвонок ${fullName(c.peer)}`} onClick={()=>startCall(c.peer,true)}>Видео</button></div>)}{!history.data.items.length&&<p class="empty">Звонков пока нет. Найдите собеседника или позвоните из диалога.</p>}{history.data.next_cursor&&<button class="ghost" disabled={loading} onClick={more}>Показать ещё</button>}</>}
 </div></section>;
}
