(() => {
  if(window.top!==window||!["http://localhost:8080","http://127.0.0.1:8080"].includes(location.origin)||globalThis.socialnetVKBridge)return;
  globalThis.socialnetVKBridge=true;
  const active=new Map();
  function answer(id,data){window.postMessage({channel:"socialnet-vk:response",id,...data},location.origin);}
  window.addEventListener("message",e=>{
    const m=e.data;if(e.source!==window||e.origin!==location.origin||m?.channel!=="socialnet-vk:request"||typeof m.id!=="string"||m.id.length>64)return;
    if(m.op==="cancel"){active.get(m.id)?.disconnect();active.delete(m.id);return;}
    if(m.op==="catalogue"){
      chrome.runtime.sendMessage({op:"catalogue"}).then(r=>answer(m.id,r.error?{type:"error",error:r.error}:{type:"catalogue",items:r.items}),()=>answer(m.id,{type:"error",error:"Перезагрузите страницу после обновления расширения."}));return;
    }
    if(m.op!=="audio"||typeof m.key!=="string"||! /^-?\d+_\d+$/.test(m.key)||active.size)return;
    const port=chrome.runtime.connect({name:"vk-audio"});active.set(m.id,port);
    port.onMessage.addListener(data=>{answer(m.id,data);if(data.type==="chunk")port.postMessage({op:"ack"});else{active.delete(m.id);port.disconnect();}});
    port.onDisconnect.addListener(()=>{if(active.delete(m.id))answer(m.id,{type:"error",error:"Соединение с расширением закрыто. Повторите попытку."});});
    port.postMessage({op:"audio",key:m.key});
  });
  window.addEventListener("pagehide",()=>{for(const p of active.values())p.disconnect();active.clear();});
})();
