(() => {
  if(!SocialnetVK.vkPage(location.href))throw Error("Откройте страницу музыки VK.");
  const found=new Map();let rejected=0;
  for(const element of document.querySelectorAll("[data-audio]")) {
    try {
      const data=JSON.parse(element.getAttribute("data-audio"));const t=SocialnetVK.track(data);if(!t){rejected++;continue;}
      // Prefer rendered labels; do not put source HTML into the destination.
      t.title=SocialnetVK.clean(element.querySelector(".audio_row__title_inner, .audio_row__title, .ai_title")?.textContent||t.title);
      t.artist=SocialnetVK.clean(element.querySelector(".audio_row__performers, .ai_artist")?.textContent||t.artist);
      // Only a source associated with this exact track, never a global player URL.
      const source=element.querySelector("audio[src], audio source[src], a[download]");
      t.url=t.url||SocialnetVK.audioURL(source?.getAttribute("src")||source?.getAttribute("href"));
      if(t.url)t.reason="";
      found.set(t.key,t);
    }catch{rejected++;}
  }
  return {items:[...found.values()].slice(0,5000),rejected};
})();
