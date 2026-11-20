import { h, Fragment } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { get } from "./api.js";
import { ErrorLine, Loading, useLoad } from "./ui.js";

interface Row {id:number; kind:string; text:string; created_at:string; archived:boolean; likes:number; comments:number; viewers:number}
const date = (d:Date) => d.toISOString().slice(0,10);
export function StatisticsPage() {
 const [start,setStart]=useState(date(new Date(Date.now()-6*86400000)));
 const [end,setEnd]=useState(date(new Date()));
 const [period,setPeriod]=useState({start,end});
 const [kind,setKind]=useState("post");
 const [tab,setTab]=useState("overview");
 const [status,setStatus]=useState("all");
 const stats=useLoad(async()=>({...await get<{items:Row[];truncated:boolean}>(`/users/me/statistics?start=${period.start}&end=${period.end}`),period}),[period.start,period.end]);
 const ready=stats.data?.period===period;
 const rows=(ready?stats.data?.items||[]:[]).filter(r=>r.kind===kind && (kind!=="story"||status==="all"||(status==="archive"?r.archived:!r.archived)));
 const likes=rows.reduce((n,r)=>n+r.likes,0), comments=rows.reduce((n,r)=>n+r.comments,0), viewers=rows.reduce((n,r)=>n+r.viewers,0);
 function download(){
  const cell=(s:unknown)=>'"'+String(s).replace(/^[=+@\-\t\r]/,"'$&").replace(/"/g,'""')+'"';
  const lines=[["Тип","ID","Описание","Дата публикации UTC","Реакции за период","Комментарии за период","Уникальные просмотры истории за период"],...rows.map(r=>[r.kind,r.id,r.text,r.created_at,r.kind==="post"?r.likes:"",r.kind==="post"?r.comments:"",r.kind==="story"?r.viewers:""])];
  const url=URL.createObjectURL(new Blob(["\uFEFF"+lines.map(row=>row.map(cell).join(";")).join("\r\n")],{type:"text/csv;charset=utf-8"}));
  const a=document.createElement("a"); a.href=url; a.download=`socialnet-${kind}-${period.start}-${period.end}.csv`; a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
 }
 return <section class="personal-statistics">
  <header class="card"><h1>Моя статистика</h1><p class="muted">Только ваши публикации. Даты и границы периода — UTC.</p>
   <form class="stats-toolbar" onSubmit={e=>{e.preventDefault();setPeriod({start,end});}}>
    <label>С <input aria-label="Начало периода" type="date" required value={start} max={end} onInput={e=>setStart(e.currentTarget.value)}/></label>
    <label>По <input aria-label="Конец периода" type="date" required value={end} min={start} onInput={e=>setEnd(e.currentTarget.value)}/></label>
    <button class="primary">Применить</button><button type="button" class="ghost" disabled={!ready||!!stats.error} onClick={download}>Экспорт CSV</button>
   </form>
   <nav class="stats-toolbar" aria-label="Тип статистики">{[["post","Посты"],["story","Истории"]].map(([value,label])=><button class={kind===value?"primary":"ghost"} aria-pressed={kind===value} onClick={()=>setKind(value)}>{label}</button>)}</nav>
  </header>
  <ErrorLine error={stats.error}/>{!ready&&!stats.error&&<Loading/>}
  {ready&&stats.data&&!stats.error&&<div class="card">
   <nav class="stats-toolbar" aria-label="Раздел статистики">{[["overview","Общее"],["content","Контент"],["audience","Аудитория"]].map(([value,label])=><button class={tab===value?"primary":"ghost"} aria-pressed={tab===value} onClick={()=>setTab(value)}>{label}</button>)}
   {kind==="story"&&<select aria-label="Состояние историй" value={status} onChange={e=>setStatus(e.currentTarget.value)}><option value="all">Все истории</option><option value="active">Активные сейчас</option><option value="archive">Архивные</option></select>}</nav>
   {stats.data.truncated&&<p role="status">Показаны последние 1000 публикаций. Итоги и экспорт относятся только к ним.</p>}
   {tab==="audience"?<p class="empty">Демография и устройства зрителей пока не собираются. Данных для диаграмм аудитории нет.</p>:<>
    {tab==="overview"&&<div class="stats-metrics"><article><span>Публикаций до конца периода</span><strong>{rows.length}</strong></article>{kind==="post"?<><article><span>Реакции за период</span><strong>{likes}</strong></article><article><span>Комментарии за период</span><strong>{comments}</strong></article></>:<article><span>Сумма уникальных просмотров историй</span><strong>{viewers}</strong></article>}</div>}
    <p class="muted">{kind==="post"?"Сохранённые реакции и комментарии, добавленные за выбранный период, включая старые посты. Просмотры и охват постов пока не учитываются.":"Каждый зритель учитывается один раз на историю по дате первого просмотра. Собственные просмотры исключены. Один человек может быть учтён в нескольких историях."}</p>
    {tab==="overview"&&<div class="stats-ranking"><h2>{kind==="post"?"Публикации по числу взаимодействий":"Истории по числу зрителей"}</h2>{[...rows].sort((a,b)=>(b.likes+b.comments+b.viewers)-(a.likes+a.comments+a.viewers)).slice(0,10).map(r=>{const value=r.likes+r.comments+r.viewers;return <div class="stats-bar-row"><span>{r.text||`№${r.id}`}</span><meter min={0} max={Math.max(1,...rows.map(x=>x.likes+x.comments+x.viewers))} value={value} aria-label={r.text||`Публикация ${r.id}`}/><strong>{value}</strong></div>;})}{!rows.length&&<p class="muted">Нет данных для диаграммы.</p>}</div>}
    {rows.length?<div class="stats-table"><table><thead><tr><th>Публикация</th><th>Дата</th>{kind==="post"?<><th>Реакции</th><th>Комментарии</th></>:<><th>Зрители</th><th>Состояние</th></>}</tr></thead><tbody>{rows.map(r=><tr key={r.id}><td>{kind==="post"?<a href={`/p/${r.id}`}>{r.text||`Запись №${r.id}`}</a>:r.text||`История №${r.id}`}</td><td>{new Date(r.created_at).toLocaleDateString("ru-RU",{timeZone:"UTC"})}</td><td>{kind==="post"?r.likes:r.viewers}</td><td>{kind==="post"?r.comments:r.archived?"Архивная":"Активная"}</td></tr>)}</tbody></table></div>:<p class="empty">Нет публикаций для выбранного периода и фильтра.</p>}
   </>}
  </div>}
 </section>;
}
