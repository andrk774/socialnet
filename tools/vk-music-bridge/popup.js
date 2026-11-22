const status=document.getElementById("status");
async function action(fn){document.querySelectorAll("button").forEach(b=>b.disabled=true);try{status.textContent=await fn();}catch(e){status.textContent=e.message;}finally{document.querySelectorAll("button").forEach(b=>b.disabled=false);}}
document.getElementById("collect").onclick=()=>action(async()=>{
  const [tab]=await chrome.tabs.query({active:true,currentWindow:true});if(!SocialnetVK.vkPage(tab?.url))throw Error("Откройте свою музыку или плейлист на vk.ru.");
  await chrome.scripting.executeScript({target:{tabId:tab.id},files:["core.js"]});
  const [scan]=await chrome.scripting.executeScript({target:{tabId:tab.id},files:["collect.js"]});
  if(!scan?.result?.items?.length)throw Error("Записи не найдены. Откройте список треков и прокрутите его. Если список виден, формат страницы пока не поддерживается.");
  const r=await chrome.runtime.sendMessage({op:"collect",items:scan.result.items,label:document.getElementById("playlist").value});if(r.error)throw Error(r.error);
  return `Добавлено/обновлено: ${scan.result.items.length}. Всего: ${r.count}. Прямой аудиофайл доступен у ${r.available}.`;
});
document.getElementById("allow").onclick=()=>action(async()=>{
  const [tab]=await chrome.tabs.query({active:true,currentWindow:true});if(!SocialnetVK.destination(tab?.url))throw Error("Откройте socialnet: http://127.0.0.1:8080/music или http://localhost:8080/music.");
  const r=await chrome.runtime.sendMessage({op:"allow",tabId:tab.id,origin:new URL(tab.url).origin});if(r.error)throw Error(r.error);
  await chrome.scripting.executeScript({target:{tabId:tab.id},files:["bridge.js"]});
  return "Разрешено для этой вкладки на 2 часа. В socialnet откройте «Импорт из открытого VK» → «Получить список».";
});
document.getElementById("clear").onclick=()=>action(async()=>{const r=await chrome.runtime.sendMessage({op:"clear"});if(r.error)throw Error(r.error);return "Список удалён, доступ отключён.";});
