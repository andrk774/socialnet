importScripts("core.js");
const core=SocialnetVK;
let writes=Promise.resolve();const transfers=new Set();
function popup(sender){return sender.id===chrome.runtime.id&&sender.url===chrome.runtime.getURL("popup.html");}
async function permitted(sender){
  if(sender.id!==chrome.runtime.id||sender.frameId!==0||!core.destination(sender.url))return false;
  const {allowed}=await chrome.storage.session.get("allowed");
  return allowed?.tabId===sender.tab?.id&&allowed.origin===new URL(sender.url).origin&&allowed.until>Date.now();
}
chrome.runtime.onMessage.addListener((m,sender,reply)=>{
  const run=async()=>{
    if(popup(sender)){
      if(m.op==="clear"){for(const abort of transfers)abort();await chrome.storage.session.clear();return {};}
      if(m.op==="allow"){
        const tab=await chrome.tabs.get(m.tabId);if(!core.destination(tab.url)||new URL(tab.url).origin!==m.origin)throw Error("Неверная вкладка socialnet");
        for(const abort of transfers)abort();await chrome.storage.session.set({allowed:{tabId:m.tabId,origin:m.origin,until:Date.now()+2*60*60*1000}});return {};
      }
      if(m.op==="collect"){
        if(!Array.isArray(m.items)||m.items.length>5000)throw Error("Слишком большой список");
        const items=m.items.filter(t=>/^(-?\d+)_(\d+)$/.test(t.key)).map(t=>({key:t.key,title:core.clean(t.title),artist:core.clean(t.artist),url:core.audioURL(t.url),reason:core.clean(t.reason),playlists:[]})).filter(t=>t.title);
        const old=await chrome.storage.session.get("catalogue");const catalogue=core.merge(old.catalogue||[],items,core.clean(m.label,100));
        await chrome.storage.session.set({catalogue});return {count:catalogue.length,available:catalogue.filter(t=>t.url).length};
      }
    }
    if(m.op==="catalogue"&&await permitted(sender)){
      const {catalogue=[]}=await chrome.storage.session.get("catalogue");
      return {items:catalogue.map(({url,...t})=>({...t,available:!!url}))};
    }
    throw Error("Разрешите перенос через значок расширения в этой вкладке socialnet.");
  };
  // Serialize mutations to avoid dropping tracks when two popup actions overlap.
  const task=writes.then(run);writes=task.catch(()=>{});task.then(reply,e=>reply({error:e.message}));return true;
});
chrome.runtime.onConnect.addListener(port=>{
  if(port.name!=="vk-audio")return;
  let started=false,ack=null;
  const controller=new AbortController();const abort=()=>{controller.abort();if(ack){ack();ack=null;}};transfers.add(abort);
  const timer=setTimeout(abort,120000);
  const clean=()=>{clearTimeout(timer);transfers.delete(abort);controller.abort();if(ack){ack();ack=null;}};
  port.onDisconnect.addListener(clean);
  port.onMessage.addListener(m=>{
    if(m.op==="ack"){ack?.();ack=null;return;}
    if(started||m.op!=="audio")return;started=true;
    void (async()=>{
      if(!await permitted(port.sender))throw Error("Доступ к этой вкладке не разрешён или истёк.");
      const {catalogue=[]}=await chrome.storage.session.get("catalogue");const track=catalogue.find(t=>t.key===m.key);
      const url=core.audioURL(track?.url);if(!url)throw Error("Нет прямого аудиофайла. Обновите список на странице VK.");
      // URL is from the collected catalogue only. No cookies, redirects or arbitrary fetch proxy.
      const res=await fetch(url,{credentials:"omit",redirect:"error",referrerPolicy:"no-referrer",signal:controller.signal});
      if(!res.ok)throw Error(`VK не отдал файл (HTTP ${res.status}). Обновите список в VK.`);
      const mime=(res.headers.get("content-type")||"").split(";")[0].toLowerCase();
      if(!["audio/mpeg","audio/mp3","audio/mp4","audio/x-m4a","audio/ogg","application/ogg","audio/flac","audio/x-flac","audio/wav","audio/x-wav","application/octet-stream"].includes(mime))throw Error("Ответ VK не является поддерживаемым аудиофайлом.");
      if(Number(res.headers.get("content-length"))>core.MAX_BYTES)throw Error("Аудиофайл больше 50 МБ.");
      const reader=res.body.getReader();let bytes=0,index=0;
      while(true){const {done,value}=await reader.read();if(done)break;bytes+=value.length;if(bytes>core.MAX_BYTES)throw Error("Аудиофайл больше 50 МБ.");
        for(let n=0;n<value.length;n+=65536){
          if(controller.signal.aborted)throw Error("Перенос остановлен");
          const part=value.subarray(n,n+65536);let binary="";for(const b of part)binary+=String.fromCharCode(b);
          await new Promise(resolve=>{ack=resolve;port.postMessage({type:"chunk",index:index++,data:btoa(binary)});});
        }
      }
      if(!bytes)throw Error("VK вернул пустой файл.");
      port.postMessage({type:"done",bytes,mime,ext:new URL(url).pathname.split(".").pop().toLowerCase()});
    })().catch(e=>{try{port.postMessage({type:"error",error:e.name==="AbortError"?"Загрузка прервана или истекло время ожидания. Повторите попытку.":e.message});}catch{}}).finally(clean);
  });
});
