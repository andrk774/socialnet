import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, put } from "./api.js";
import { ErrorLine } from "./ui.js";
interface Settings {host:string;port:number;username:string;sender:string;tls_mode:string;password_set:boolean;source:string}
export function SMTPPanel(){
 const [data,setData]=useState<Settings>();const [password,setPassword]=useState("");const [clear,setClear]=useState(false);const [busy,setBusy]=useState(false);const [error,setError]=useState<unknown>(null);const [saved,setSaved]=useState(false);
 useEffect(()=>{void get<Settings>("/admin/smtp").then(setData).catch(setError)},[]);
 function change(key:keyof Settings,value:string|number){if(data)setData({...data,[key]:value});setSaved(false)}
 async function save(e:Event){e.preventDefault();if(!data||busy)return;setBusy(true);setError(null);setSaved(false);try{await put("/admin/smtp",{...data,password,clear_password:clear});setPassword("");setClear(false);setData(await get<Settings>("/admin/smtp"));setSaved(true)}catch(e){setError(e)}finally{setBusy(false)}}
 return <section class="music-panel"><h2>Почта — SMTP</h2><ErrorLine error={error}/>{data&&<form onSubmit={save}><fieldset disabled={busy} style="display:grid;gap:14px;border:0;padding:0;min-width:0">
 <label>SMTP-сервер<input required maxLength={253} placeholder="smtp.example.com" value={data.host} onInput={e=>change("host",e.currentTarget.value)}/></label>
 <label>Порт<input type="number" required min={1} max={65535} value={data.port} onInput={e=>change("port",Number(e.currentTarget.value))}/></label>
 <label>Шифрование<select required value={data.tls_mode} onChange={e=>change("tls_mode",e.currentTarget.value)}>{data.tls_mode==="auto"&&<option value="auto" disabled>Из окружения — выберите режим</option>}<option value="starttls">STARTTLS (обычно порт 587)</option><option value="tls">TLS (обычно порт 465)</option><option value="none">Без TLS — локальный Mailpit, без логина</option></select></label>
 <label>Отправитель<input required maxLength={320} value={data.sender} placeholder="Socialnet &lt;no-reply@example.com&gt;" onInput={e=>change("sender",e.currentTarget.value)}/></label>
 <label>Логин<input autoComplete="off" maxLength={320} value={data.username} onInput={e=>change("username",e.currentTarget.value)}/></label>
 <label>Пароль приложения<input type="password" autoComplete="new-password" maxLength={1024} disabled={clear} value={password} placeholder={data.password_set?"Сохранён; пустое поле сохраняет прежний пароль":"Пароль SMTP"} onInput={e=>{setPassword(e.currentTarget.value);setSaved(false)}}/></label>
 <label><input type="checkbox" checked={clear} onChange={e=>{setClear(e.currentTarget.checked);setSaved(false)}}/> Удалить сохранённый пароль</label>
 <p class="muted">Источник: {data.source==="database"?"админ-панель":"окружение сервера"}. Пароль хранится зашифрованным. Настройки применяются к следующим отправкам без перезапуска. Сохранение не отправляет тестовое письмо.</p>
 <button disabled={data.tls_mode==="auto"} type="submit">{busy?"Сохранение…":"Сохранить настройки SMTP"}</button>
 </fieldset>{saved&&<p role="status">Настройки сохранены. Доставка внешнему получателю пока не проверена.</p>}</form>}</section>
}
