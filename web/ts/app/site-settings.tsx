import {h} from '../vendor/preact/index.js';
import {useEffect,useState} from '../vendor/preact/hooks.js';
import {get,put} from './api.js';
import {ErrorLine} from './ui.js';
interface Settings {public_url:string;ingress_class:string;tls_secret:string;cookie_secure:boolean}
export function SiteSettingsPanel(){
 const [data,setData]=useState<Settings>();const [error,setError]=useState<unknown>(null);const [busy,setBusy]=useState(false);const [saved,setSaved]=useState(false);
 useEffect(()=>{void get<Settings>('/admin/site').then(setData).catch(setError)},[]);
 function change(k:keyof Settings,v:string){if(data)setData({...data,[k]:v});setSaved(false)}
 function download(){if(!data)return;try{const u=new URL(data.public_url);if(u.protocol!=='https:'||u.port||u.hostname==='localhost'||/^[\d.]+$/.test(u.hostname)||u.hostname.includes(':'))throw new Error('Для Ingress нужен домен HTTPS без номера порта.');const manifest={apiVersion:'networking.k8s.io/v1',kind:'Ingress',metadata:{name:'socialnet',namespace:'socialnet'},spec:{ingressClassName:data.ingress_class,tls:[{hosts:[u.hostname],secretName:data.tls_secret}],rules:[{host:u.hostname,http:{paths:[{path:'/',pathType:'Prefix',backend:{service:{name:'socialnet',port:{number:80}}}}]}}]}};const url=URL.createObjectURL(new Blob([JSON.stringify(manifest,null,2)],{type:'application/json'}));const a=document.createElement('a');a.href=url;a.download='socialnet-ingress.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}catch(e){setError(e)}}
 return <section class="music-panel"><h2>Адрес сайта и HTTPS</h2><ErrorLine error={error}/>{data&&<form onSubmit={async e=>{e.preventDefault();setBusy(true);setError(null);setSaved(false);try{await put('/admin/site',data);setData(await get<Settings>('/admin/site'));setSaved(true)}catch(e){setError(e)}finally{setBusy(false)}}}><fieldset disabled={busy} style="display:grid;gap:14px;border:0;padding:0;min-width:0">
 <label>Адрес socialnet<input type="url" required maxLength={512} placeholder="https://socialnet.example.com" value={data.public_url} onInput={e=>change('public_url',e.currentTarget.value)}/></label>
 <p class="muted">Этот адрес используется в новых QR-кодах входа и письмах аккаунта. Для телефона укажите доступный ему домен или адрес компьютера в Wi-Fi. localhost работает только на текущем устройстве.</p>
 <label>Класс Ingress в Kubernetes<input required value={data.ingress_class} onInput={e=>change('ingress_class',e.currentTarget.value)}/></label>
 <label>Название Secret с TLS-сертификатом<input required value={data.tls_secret} onInput={e=>change('tls_secret',e.currentTarget.value)}/></label>
 <p>Secure-cookie в текущей конфигурации: {data.cookie_secure?'включены':'выключены'}. Меняется параметром COOKIE_SECURE при развёртывании.</p>
 <p class="muted">Сохранение адреса не включает HTTPS и не выпускает сертификат. Для домена установите Ingress-контроллер, направьте DNS на точку входа и создайте указанный TLS Secret в namespace socialnet. Для локального сертификата потребуется доверие на каждом устройстве.</p>
 <div class="row"><button class="primary">Сохранить адрес и настройки</button><button type="button" onClick={download}>Скачать конфигурацию Ingress</button></div>
 </fieldset>{saved&&<p role="status">Настройки сохранены. Новые ссылки используют указанный адрес. Доступность HTTPS ещё не проверена.</p>}</form>}</section>
}
