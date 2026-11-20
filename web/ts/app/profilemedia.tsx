import {h,Fragment} from '../vendor/preact/index.js';
import {useState,useRef} from '../vendor/preact/hooks.js';
import {get,post} from './api.js';
import {ErrorLine,Loading,useLoad,duration} from './ui.js';
import {Icon} from './shellparts.js';
import {Video} from './posts.js';
import {LibraryViewer,DetailedPhoto} from './libraryviewer.js';
const tabs=[['music','music','Музыка'],['photos','photo','Фото'],['videos','video','Видео'],['albums','photo','Альбомы'],['clips','video','Клипы'],['articles','file','Статьи']];
interface Item {id:string|number;title?:string;artist?:string;url?:string;thumb_url?:string;cover_url?:string;hls_url?:string;mp4_url?:string;poster_url?:string;status?:string;duration_sec?:number;width?:number;height?:number;count?:number}
function ProfileTrack({item}:{item:Item}) {
 const audio=useRef<HTMLAudioElement>(null);
 const [playing,setPlaying]=useState(false),[elapsed,setElapsed]=useState(0),[error,setError]=useState<unknown>(null);
 const [length,setLength]=useState(item.duration_sec??0);
 async function toggle(){const el=audio.current;if(!el)return;if(!el.paused){el.pause();return;}document.querySelectorAll('audio').forEach(other=>{if(other!==el)other.pause();});try{await el.play();setError(null);}catch(e){setError(e);}}
 return <div class="profile-track"><button class="profile-track-play" aria-label={`${playing?'Пауза':'Слушать'}: ${item.title}`} onClick={toggle}><svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d={playing?'M6 4h4v16H6zM14 4h4v16h-4z':'M7 4v16l14-8z'}/></svg></button><div class="profile-track-info"><strong>{item.title}</strong><span class="muted">{item.artist||'Неизвестный исполнитель'}</span></div><span class="profile-track-duration">{duration(length)}</span><audio ref={audio} preload="metadata" src={item.url} onPlay={()=>setPlaying(true)} onPause={()=>setPlaying(false)} onEnded={()=>setPlaying(false)} onTimeUpdate={e=>setElapsed(e.currentTarget.currentTime)} onLoadedMetadata={e=>{if(Number.isFinite(e.currentTarget.duration))setLength(e.currentTarget.duration);}} onError={()=>setError(new Error('Не удалось загрузить аудио'))}/>{(playing||elapsed>0)&&<div class="profile-track-progress"><span>{duration(elapsed)}</span><input aria-label="Позиция трека" type="range" min="0" max={length||1} step="1" value={elapsed} onInput={e=>{if(audio.current)audio.current.currentTime=Number(e.currentTarget.value);}}/></div>}<ErrorLine error={error}/></div>
}
function ProfileVideo({item}:{item:Item}) {
 const dialog=useRef<HTMLDialogElement>(null);
 const [opened,setOpened]=useState(false);
 const [error,setError]=useState<unknown>(null);
 return <><button class="profile-video-card" onClick={()=>{setOpened(true);dialog.current?.showModal();}}><span class="profile-video-thumb">{item.poster_url?<img loading="lazy" src={item.poster_url} alt=""/>:<Icon name="video"/>}<span class="profile-video-play" aria-hidden="true">▶</span><span class="profile-video-duration">{duration(item.duration_sec??0)}</span></span><strong>{item.title||'Видео'}</strong></button><dialog ref={dialog} class="profile-video-dialog" onClose={()=>setOpened(false)}><header><strong>{item.title||'Видео'}</strong><button class="ghost" aria-label="Закрыть видео" onClick={()=>dialog.current?.close()}>×</button></header>{opened&&<Video src={item.hls_url??''} mp4={item.mp4_url} poster={item.poster_url??''} title={item.title} onPlay={()=>{void post(`/videos/${item.id}/view`).catch(setError);}}/>}<ErrorLine error={error}/></dialog></>;
}
export function ProfileMedia({id}:{id:number}){
 const [tab,setTab]=useState('music');
 return <section class="profile-media"><div class="profile-media-tabs" role="tablist" aria-label="Материалы профиля">{tabs.map(([key,icon,label])=><button id={`profile-tab-${key}`} role="tab" aria-selected={tab===key} aria-controls="profile-media-content" class={tab===key?'active':''} onClick={()=>setTab(key)}><Icon name={icon}/> {label}</button>)}</div><div id="profile-media-content" role="tabpanel" aria-labelledby={`profile-tab-${tab}`}><MediaContent key={`${id}-${tab}`} id={id} tab={tab}/></div></section>
}
function MediaContent({id,tab}:{id:number;tab:string}){
 const [current,setCurrent]=useState<string|null>(null);
 const path=tab==='music'?`/users/${id}/music?limit=6`:tab==='photos'?'/photos?limit=6':tab==='albums'?'/photo-albums':tab==='articles'?'/articles/mine':`/users/${id}/videos`;
 const load=useLoad(()=>get<{items:Item[]}>(path),[path]);
 if(load.error)return <ErrorLine error={load.error}/>;if(!load.data)return <Loading/>;
 const items=load.data.items.filter(x=>tab==='clips'?x.status==='ready'&&!!x.width&&!!x.height&&x.height>x.width&&(x.duration_sec??Infinity)<=60:tab==='videos'?x.status==='ready':true).slice(0,6);
 const link=tab==='music'?'/music':tab==='photos'?'/photos':tab==='albums'?'/photos?tab=albums':tab==='articles'?'/articles/new':'/videos';
 return <div class="profile-media-body">{tab==='clips'&&<p class="muted">Вертикальные видео длительностью до минуты</p>}{!items.length?<div class="profile-media-empty"><p class="muted">{tab==='articles'?'Вы ещё не добавили статью':'Здесь пока нет материалов'}</p><a class="button ghost" href={link}>{tab==='articles'?'Создать статью':tab==='albums'?'Создать альбом':tab==='music'?'Добавить музыку':tab==='photos'?'Загрузить фото':'Загрузить видео'}</a></div>:<><div class={`profile-media-items ${tab}`}>{items.map(x=><article key={x.id}>{tab==='music'?<ProfileTrack item={x}/>:tab==='photos'?<button class="profile-photo-card" onClick={()=>setCurrent(String(x.id))}><img src={x.thumb_url||x.url} alt="Фотография"/></button>:tab==='albums'?<a href="/photos?tab=albums">{x.cover_url&&<img src={x.cover_url} alt=""/>}<strong>{x.title}</strong><p class="muted">{x.count??0} фото</p></a>:tab==='articles'?<a href={`/articles/${x.id}`}><strong>{x.title}</strong></a>:<ProfileVideo item={x}/>}</article>)}</div><a class="profile-media-all" href={tab==='articles'?'/articles':link}>Показать всё</a></>}{tab==='photos'&&current&&items.some(x=>String(x.id)===current)&&<LibraryViewer photo={items.find(x=>String(x.id)===current) as DetailedPhoto} index={items.findIndex(x=>String(x.id)===current)} total={items.length} album="Мои фотографии" onStep={n=>setCurrent(String(items[(items.findIndex(x=>String(x.id)===current)+n+items.length)%items.length].id))} onClose={()=>setCurrent(null)} onChanged={load.reload}/>}</div>
}
