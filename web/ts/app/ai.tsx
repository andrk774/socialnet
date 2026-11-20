import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get, put, post, Photo } from "./api.js";
import { ErrorLine } from "./ui.js";
interface Settings{enabled:boolean;allow_users:boolean;base_url:string;text_model:string;image_model:string;key_set:boolean}
export function AIPanel(){
 const [data,setData]=useState<Settings>();const [key,setKey]=useState("");const [clear,setClear]=useState(false);const [busy,setBusy]=useState(false);const [error,setError]=useState<unknown>(null);const [saved,setSaved]=useState(false);
 useEffect(()=>{void get<Settings>("/admin/ai").then(setData).catch(setError)},[]);
 function change(k:keyof Settings,v:string|boolean){if(data)setData({...data,[k]:v});setSaved(false)}
 return <section class="music-panel"><h2>Искусственный интеллект</h2><ErrorLine error={error}/>{data&&<form onSubmit={async e=>{e.preventDefault();if(busy)return;setBusy(true);setError(null);setSaved(false);try{await put("/admin/ai",{...data,api_key:key,clear_key:clear});setKey("");setClear(false);setData(await get<Settings>("/admin/ai"));setSaved(true)}catch(e){setError(e)}finally{setBusy(false)}}}><fieldset disabled={busy} style="display:grid;gap:14px;border:0;padding:0;min-width:0">
 <label><input type="checkbox" checked={data.enabled} onChange={e=>change("enabled",e.currentTarget.checked)}/> Включить ИИ</label>
 <label><input type="checkbox" checked={data.allow_users} onChange={e=>change("allow_users",e.currentTarget.checked)}/> Разрешить всем пользователям (иначе только администраторам)</label>
 <label>Провайдер<select value={data.base_url==="https://api.openai.com/v1"?"openai":"custom"} onChange={e=>change("base_url",e.currentTarget.value==="openai"?"https://api.openai.com/v1":"")}><option value="openai">OpenAI</option><option value="custom">Совместимый с OpenAI API</option></select></label>
 <label>Адрес API (HTTPS)<input type="url" required maxLength={512} value={data.base_url} onInput={e=>change("base_url",e.currentTarget.value)}/></label>
 <label>Модель текста<input maxLength={128} value={data.text_model} placeholder="Идентификатор модели у провайдера" onInput={e=>change("text_model",e.currentTarget.value)}/></label>
 <label>Модель изображений<input maxLength={128} value={data.image_model} placeholder="Оставьте пустым, чтобы отключить генерацию стикеров" onInput={e=>change("image_model",e.currentTarget.value)}/></label>
 <label>API-ключ<input type="password" autoComplete="new-password" maxLength={4096} disabled={clear} value={key} placeholder={data.key_set?"Сохранён — пустое поле не меняет ключ":"Введите ключ провайдера"} onInput={e=>{setKey(e.currentTarget.value);setSaved(false)}}/></label>
 <label><input type="checkbox" checked={clear} onChange={e=>{setClear(e.currentTarget.checked);setSaved(false)}}/> Удалить ключ</label>
 <p class="muted">Лимит на пользователя в сутки: 30 запросов текста и 5 изображений, включая неудачные попытки. Провайдер может списывать оплату за запросы. Отправляется только текст, выбранный пользователем; история чата автоматически не передаётся.</p>
 <button>Сохранить настройки ИИ</button></fieldset>{saved&&<p role="status">Сохранено. Запросы провайдеру при сохранении не отправляются.</p>}</form>}</section>
}
export function AIText({text,apply}:{text:string;apply:(v:string)=>void}){
 const [enabled,setEnabled]=useState(false);const [busy,setBusy]=useState(false);const [result,setResult]=useState("");const [error,setError]=useState<unknown>(null);
 useEffect(()=>{void get<{text:boolean}>("/ai/status").then(x=>setEnabled(x.text)).catch(()=>{})},[]);
 if(!enabled)return null;
 return <aside><p class="muted">ИИ получит только текст вашего черновика.</p><button type="button" disabled={busy||!text.trim()} onClick={async()=>{setBusy(true);setError(null);setResult("");try{setResult((await post<{text:string}>("/ai/generate",{kind:"text",prompt:text})).text)}catch(e){setError(e)}finally{setBusy(false)}}}>{busy?"ИИ работает…":"Улучшить текст с ИИ"}</button><ErrorLine error={error}/>{result&&<div><p style="white-space:pre-wrap">{result}</p><button type="button" onClick={()=>{apply(result);setResult("")}}>Вставить в черновик</button><button type="button" onClick={()=>setResult("")}>Отмена</button></div>}</aside>
}
export function AISticker({created}:{created:(p:Photo)=>void}){
 const [enabled,setEnabled]=useState(false);const [prompt,setPrompt]=useState("");const [busy,setBusy]=useState(false);const [error,setError]=useState<unknown>(null);
 useEffect(()=>{void get<{images:boolean}>("/ai/status").then(x=>setEnabled(x.images)).catch(()=>{})},[]);if(!enabled)return null;
 return <div><label>Стикер с ИИ: опишите картинку<textarea maxLength={1000} value={prompt} onInput={e=>setPrompt(e.currentTarget.value)}/></label><p class="muted">Описание будет отправлено провайдеру ИИ. Готовый стикер не отправится в чат автоматически.</p><button type="button" disabled={busy||!prompt.trim()} onClick={async()=>{setBusy(true);setError(null);try{created(await post<Photo>("/ai/generate",{kind:"image",prompt}));setPrompt("")}catch(e){setError(e)}finally{setBusy(false)}}}>{busy?"Создаём картинку…":"Создать с ИИ"}</button><ErrorLine error={error}/></div>
}
