import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, put } from "./api.js";
import { ErrorLine } from "./ui.js";

interface Settings { enabled: boolean; stun_url: string; turn_url: string; secret_set: boolean; source: string }
export function TURNPanel() {
 const [data,setData]=useState<Settings>();
 const [secret,setSecret]=useState("");
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState<unknown>(null);
 const [saved,setSaved]=useState(false);
 useEffect(()=>{void get<Settings>("/admin/turn").then(setData).catch(setError)},[]);
 async function save(e: Event) {
  e.preventDefault(); if(!data||busy)return;
  setBusy(true);setError(null);setSaved(false);
  try {await put("/admin/turn",{...data,secret});setSecret("");setData(await get<Settings>("/admin/turn"));setSaved(true)}
  catch(e){setError(e)}finally{setBusy(false)}
 }
 return <section class="music-panel"><h2>TURN-сервер для звонков</h2><ErrorLine error={error}/>{data&&<form onSubmit={save}>
 <fieldset disabled={busy} style="border:0;padding:0;display:grid;gap:14px;min-width:0">
 <label><input type="checkbox" checked={data.enabled} onChange={e=>{setData({...data,enabled:e.currentTarget.checked});setSaved(false)}}/> Использовать TURN</label>
 <label>Адрес STUN<input value={data.stun_url} maxLength={512} placeholder="stun:turn.example.org:3478" onInput={e=>{setData({...data,stun_url:e.currentTarget.value});setSaved(false)}}/></label>
 <label>Адрес TURN<input value={data.turn_url} maxLength={512} required={data.enabled} placeholder="turns:turn.example.org:5349?transport=tcp" onInput={e=>{setData({...data,turn_url:e.currentTarget.value});setSaved(false)}}/></label>
 <label>Общий секрет coturn<input type="password" autoComplete="new-password" value={secret} maxLength={512} placeholder={data.secret_set?"Секрет сохранён — оставьте пустым, чтобы не менять":"Введите static-auth-secret"} onInput={e=>{setSecret(e.currentTarget.value);setSaved(false)}}/></label>
 <p class="muted">Секрет должен совпадать со static-auth-secret на сервере coturn (use-auth-secret). Сохранённый секрет не показывается. Настройки применяются к новым звонкам без перезапуска API.</p>
 <p class="muted">Источник: {data.source==="database"?"админ-панель":"переменные окружения"}. Эта форма подключает уже установленный TURN-сервер; она не устанавливает coturn и не открывает порты.</p>
 <button type="submit">{busy?"Сохранение…":"Сохранить настройки TURN"}</button>
 </fieldset>{saved&&<p role="status">Сохранено. Проверьте звонок между устройствами в разных сетях.</p>}
 </form>}</section>;
}
