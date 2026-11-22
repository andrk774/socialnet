import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, post, del } from "./api.js";

interface Nearby { visible: boolean; people: {id:number;username:string;first_name:string;last_name:string;distance:string}[] }
export function NearbyPage() {
 const [data,setData]=useState<Nearby>({visible:false,people:[]});
 const [busy,setBusy]=useState(false), [error,setError]=useState("");
 async function refresh() { try {setData(await get<Nearby>("/nearby"));}catch {setError("Не удалось загрузить людей рядом.");} }
 useEffect(()=>{void refresh();const timer=setInterval(()=>void refresh(),60000);return ()=>clearInterval(timer);},[]);
 async function enable() {
  setBusy(true);setError("");
  try {
   if (!window.isSecureContext || !navigator.geolocation) throw new Error("Для геолокации откройте socialnet по HTTPS.");
   const fix=await new Promise<GeolocationPosition>((resolve,reject)=>navigator.geolocation.getCurrentPosition(resolve,reject,{enableHighAccuracy:false,timeout:15000,maximumAge:60000}));
   setData(await post<Nearby>("/nearby",{lat:fix.coords.latitude,lon:fix.coords.longitude,consent:true}));
  } catch(e) {setError(e instanceof Error ? e.message : "Разрешите доступ к местоположению и повторите попытку.");}finally{setBusy(false);}
 }
 async function hide() {setBusy(true);setError("");try{await del("/nearby");setData({visible:false,people:[]});}catch{setError("Не удалось скрыться. Повторите попытку.");}finally{setBusy(false);}}
 return <section><h1>Люди рядом</h1><p>Найдите пользователей socialnet в пределах 25 км. После согласия вы видны 15 минут. Мы показываем только примерное расстояние, без точки на карте.</p>
 <p><strong>{data.visible?"Вы видны людям рядом":"Вы скрыты"}</strong></p>
 <button disabled={busy} onClick={enable}>{busy?"Подождите…":data.visible?"Обновить местоположение и видимость":"Разрешить геолокацию и стать видимым"}</button>{data.visible && <button disabled={busy} onClick={hide}>Скрыться</button>}
 {error && <p role="alert">{error}</p>}
 {data.visible && !data.people.length && <p>Пока никого рядом нет. Другому пользователю тоже нужно включить видимость.</p>}
 {data.people.map(p=><article key={p.id}><h3><a href={`/u/${encodeURIComponent(p.username)}`}>{p.first_name} {p.last_name}</a></h3><p>Примерно {p.distance}</p><a href={`/im/u/${p.id}`}>Написать сообщение</a></article>)}
 <h2>Мессенджер по Wi-Fi</h2><p>Подключитесь к общей Wi-Fi-сети и откройте один и тот же локальный сервер socialnet. Сообщения проходят через этот сервер даже без внешнего интернета. Сервер и Wi-Fi должны оставаться включёнными.</p><a href="/im">Открыть мессенджер</a>
 </section>;
}
