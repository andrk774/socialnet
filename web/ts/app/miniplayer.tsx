import { h } from "../vendor/preact/index.js";
export interface MiniPlayerState {title:string;artist:string;paused:boolean;previousEnabled:boolean;nextEnabled:boolean;previous:()=>void;toggle:()=>void;next:()=>void}
export function MiniPlayer({player}:{player:MiniPlayerState|null}){
 if(!player)return null;
 return <div class="header-player" role="group" aria-label="Музыкальный мини-плеер">
 <button aria-label="Мини-плеер: предыдущий трек" title="Предыдущий трек" disabled={!player.previousEnabled} onClick={player.previous}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 4h3v16H5zM19 4v16L8 12z"/></svg></button>
 <button aria-label={player.paused?"Мини-плеер: воспроизвести":"Мини-плеер: пауза"} title={player.paused?"Воспроизвести":"Пауза"} onClick={player.toggle}><svg viewBox="0 0 24 24" aria-hidden="true"><path d={player.paused?"M7 3v18l15-9z":"M5 4h5v16H5zM14 4h5v16h-5z"}/></svg></button>
 <button aria-label="Мини-плеер: следующий трек" title="Следующий трек" disabled={!player.nextEnabled} onClick={player.next}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M16 4h3v16h-3zM5 4l11 8-11 8z"/></svg></button>
 <a href="/music" class="header-player-title" title={`${player.artist} — ${player.title}`}>{player.artist} — {player.title}</a>
 </div>;
}
