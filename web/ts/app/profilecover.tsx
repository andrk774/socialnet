import {h} from '../vendor/preact/index.js';
import {useRef,useState} from '../vendor/preact/hooks.js';
import {get,put,upload,Photo} from './api.js';
import {useLoad,ErrorLine} from './ui.js';
import {Icon} from './shellparts.js';
export function ProfileCover({id,self}:{id:number;self:boolean}){
 const cover=useLoad(()=>get<{url:string}>(`/users/${id}/cover`),[id]);const input=useRef<HTMLInputElement>(null);const [busy,setBusy]=useState(false);const [error,setError]=useState<unknown>(null);
 async function save(file?:File){setBusy(true);setError(null);try{let media_id:string|null=null;if(file){if(!['image/jpeg','image/png','image/gif'].includes(file.type)||file.size>10*1024*1024)throw new Error('Выберите JPEG, PNG или GIF до 10 МБ.');media_id=(await upload<Photo>('/media',file)).id}await put('/users/me/cover',{media_id});cover.reload()}catch(e){setError(e)}finally{setBusy(false);if(input.current)input.current.value=''}}
 return <div class="profile-cover">{cover.data?.url&&<img src={cover.data.url} alt="Обложка профиля"/>}{self&&<details class="cover-menu"><summary><Icon name="edit"/> Изменить обложку</summary><div><button disabled={busy} onClick={()=>input.current?.click()}><Icon name="photo"/>Загрузить изображение</button><button class="danger" disabled={busy||!cover.data?.url} onClick={()=>{if(confirm('Удалить обложку профиля?'))void save()}}><Icon name="trash"/>Удалить</button></div></details>}<input hidden ref={input} type="file" accept="image/jpeg,image/png,image/gif" onChange={e=>{const f=e.currentTarget.files?.[0];if(f)void save(f)}}/><ErrorLine error={error||cover.error}/></div>
}
