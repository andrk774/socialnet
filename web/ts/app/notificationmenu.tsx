import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Page, get, post } from "./api.js";
import { Notification, notifText } from "./pages.js";
import { Avatar, Empty, ErrorLine, Loading, Time, UserLink } from "./ui.js";
import { Icon } from "./shellparts.js";
import { usePath } from "./router.js";

export function NotificationMenu({unread,onRead}:{unread:number;onRead:()=>void}) {
  const root=useRef<HTMLDivElement>(null);const trigger=useRef<HTMLButtonElement>(null);const path=usePath();
  const [open,setOpen]=useState(false);const [items,setItems]=useState<Notification[]|null>(null);const [error,setError]=useState<unknown>(null);const [filter,setFilter]=useState("all");const [busy,setBusy]=useState(false);
  useEffect(()=>setOpen(false),[path]);
  useEffect(()=>{if(!open)return;let live=true;setError(null);get<Page<Notification>>("/notifications?limit=50").then(r=>{if(live)setItems(r.items);},e=>{if(live)setError(e);});return()=>{live=false;};},[open,unread]);
  useEffect(()=>{const close=(e:PointerEvent)=>{if(root.current&&!root.current.contains(e.target as Node))setOpen(false);};const esc=(e:KeyboardEvent)=>{if(e.key==="Escape"&&open){setOpen(false);trigger.current?.focus();}};document.addEventListener("pointerdown",close);document.addEventListener("keydown",esc);return()=>{document.removeEventListener("pointerdown",close);document.removeEventListener("keydown",esc);};},[open]);
  async function read(){if(!items||busy)return;const ids=items.filter(n=>!n.read).map(n=>n.id);if(!ids.length)return;setBusy(true);try{await post("/notifications/read",{ids});setItems(items.map(n=>({...n,read:true})));onRead();}catch(e){setError(e);}finally{setBusy(false);}}
  const visible=(items??[]).filter(n=>filter==="all"||filter==="social"&&["like","comment","reply","mention","friend_request","friend_accept"].includes(n.type)||filter==="other"&&!["like","comment","reply","mention","friend_request","friend_accept"].includes(n.type));
  const href=(n:Notification)=>n.post_id?`/p/${n.post_id}`:n.event_id?`/events/${n.event_id}`:n.ride_id?`/rides#ride-${n.ride_id}`:n.type==="video_ready"?"/videos":n.actor?`/u/${n.actor.username}`:"/notifications";
  return <div class="notification-menu" ref={root}><button ref={trigger} class="header-action" aria-label="Открыть уведомления" aria-expanded={open} aria-controls="notification-popover" onClick={()=>setOpen(!open)}><Icon name="bell"/>{unread>0&&<span class="badge">{unread}</span>}</button>
    {open&&<section class="notification-popover" id="notification-popover" aria-label="Панель уведомлений"><header><strong>Уведомления</strong><select aria-label="Источники уведомлений" value={filter} onChange={e=>setFilter(e.currentTarget.value)}><option value="all">Все источники</option><option value="social">Общение</option><option value="other">Другие события</option></select><a href="/settings?tab=notifications" aria-label="Настройки уведомлений"><Icon name="settings"/></a></header><div class="notification-scroll"><ErrorLine error={error}/>{!items&&!error?<Loading/>:visible.length===0?<Empty>Нет уведомлений по выбранному фильтру.</Empty>:[false,true].map(read=><section>{visible.some(n=>n.read===read)&&<h2>{read?"Просмотренные":"Новые"}</h2>}<ul>{visible.filter(n=>n.read===read).map(n=><li key={n.id} class={n.read?"":"unread"}>{n.actor?<Avatar user={n.actor} size={44}/>:<span class="notification-system"><Icon name="bell"/></span>}<div>{n.actor&&<UserLink user={n.actor}/>}<a class="notification-event" href={href(n)}>{notifText[n.type]??"Новое событие"}</a><Time iso={n.created_at}/></div>{!n.read&&<span class="notification-dot" aria-label="Не прочитано"/>}</li>)}</ul></section>)}</div><footer><button class="link" disabled={busy||!items?.some(n=>!n.read)} onClick={read}>Отметить прочитанными</button><a href="/notifications" onClick={()=>setOpen(false)}>Показать все</a></footer></section>}
  </div>;
}
