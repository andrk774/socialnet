/* No VK cookies, tokens, hidden APIs or URL decoding. Only data exposed in the page. */
globalThis.SocialnetVK = (() => {
  const MAX_BYTES = 50 * 1024 * 1024;
  function destination(raw) { try {const u=new URL(raw);return ["http://127.0.0.1:8080","http://localhost:8080"].includes(u.origin);}catch{return false;} }
  function vkPage(raw) {try{const u=new URL(raw);return u.protocol==="https:"&&["vk.ru","www.vk.ru","m.vk.ru","vk.com","www.vk.com","m.vk.com"].includes(u.hostname);}catch{return false;}}
  function audioURL(raw) {
    try {const u=new URL(raw);if(u.protocol!=="https:"||u.username||u.password||u.port||u.hash)return null;
      if(!["vkuseraudio.net","vkuseraudio.com","useraudio.net","useraudio.com","vk-cdn.net"].some(h=>u.hostname===h||u.hostname.endsWith("."+h)))return null;
      if(!/\.(mp3|m4a|ogg|flac|wav)$/i.test(u.pathname))return null;
      return u.href;
    }catch{return null;}
  }
  function clean(value, limit=200) {return typeof value==="string"?value.replace(/[\u0000-\u001f\u007f]/g," ").trim().slice(0,limit):"";}
  function track(raw) {
    const a=Array.isArray(raw)?{id:raw[0],owner_id:raw[1],url:raw[2],title:raw[3],artist:raw[4]}:raw;
    if(!a||!/^\d+$/.test(String(a.id))||! /^-?\d+$/.test(String(a.owner_id)))return null;
    const title=clean(a.title),artist=clean(a.artist);if(!title)return null;
    const url=audioURL(a.url);
    return {key:`${a.owner_id}_${a.id}`,title,artist,url,reason:url?"":"На странице нет прямого аудиофайла (возможен HLS или ограничение доступа)",playlists:[]};
  }
  function merge(existing,incoming,label) {
    const map=new Map(existing.map(t=>[t.key,t]));
    for(const t of incoming){const old=map.get(t.key);map.set(t.key,{...old,...t,url:t.url||old?.url||null,reason:t.url||old?.url?"":t.reason,playlists:[...new Set([...(old?.playlists||[]),...(label?[clean(label,100)]:[])])].slice(0,20)});}
    if(map.size>5000)throw Error("Не более 5000 записей за один перенос. Очистите список после импорта.");
    return [...map.values()];
  }
  return {MAX_BYTES,destination,vkPage,audioURL,clean,track,merge};
})();
