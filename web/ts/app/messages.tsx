import { VideoRecorder, PrivateVideo, VideoMessage, ChatGifts, chatServices, ServiceCard } from "./chatmedia.js";
import { AIText } from "./ai.js";
import { CustomStickers } from "./customstickers.js";
import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Conversation, Doc, Message, Page, UserCard, fetchBlob, fullName, get, post, put, del, upload } from "./api.js";
import { Icon } from "./shellparts.js";
import { DocList } from "./posts.js";
import { startCall } from "./calls.js";
import { on, sendTyping } from "./realtime.js";
import { navigate } from "./router.js";
import { me } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, Time, duration, useLoad } from "./ui.js";

const title = (c: Conversation) => (c.kind === "group" ? c.title || "Беседа" : c.peer ? fullName(c.peer) : "Диалог");

function MessageField({ value, onInput }: { value: string; onInput: (e: Event) => void }) {
  const field = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    const el = field.current;
    if (el) { el.style.height = "auto"; el.style.height = `${Math.min(el.scrollHeight, 144)}px`; }
  }, [value]);
  return <textarea ref={field} rows={1} value={value} onInput={onInput} placeholder="Сообщение" aria-label="Сообщение" title="Enter — отправить, Shift+Enter — новая строка" maxLength={4000} onKeyDown={(e) => {
    if (e.key === "Enter" && !e.shiftKey && !e.isComposing) { e.preventDefault(); e.currentTarget.form?.requestSubmit(); }
  }} />;
}

export function ConversationsPage() {
  const list = useLoad(() => get<{ items: Conversation[] }>("/conversations"), []);
  useEffect(() => on("message", () => list.reload()), []);
  useEffect(() => on("message_updated", () => list.reload()), []);
  if (list.error) return <ErrorLine error={list.error} />;
  if (!list.data) return <Loading />;
  return (
    <section>
      <h1>Сообщения</h1>
      {list.data.items.length === 0 ? <Empty>Диалогов пока нет. Напишите кому-нибудь из друзей.</Empty> : (
        <ul class="conv-list">
          {list.data.items.map((c) => (
            <li key={c.id}>
              <a href={`/im/${c.id}`} class={c.unread ? "conv unread" : "conv"}>
                {c.peer ? <Avatar user={c.peer} /> : <span class="avatar avatar-letter group" aria-hidden="true">{c.members_count}</span>}
                <span class="conv-main">
                  <span class="conv-title">{title(c)}</span>
                  <span class="muted conv-last">{preview(c.last_message)}</span>
                </span>
                <span class="conv-side">
                  {c.last_message && <Time iso={c.last_message.created_at} />}
                  {c.unread > 0 && <span class="badge" aria-label={`непрочитанных: ${c.unread}`}>{c.unread}</span>}
                </span>
              </a>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function preview(m?: Message) {
  if (!m) return "";
  if (m.encrypted) return "Зашифрованное сообщение";
  if (m.body) return m.body;
  return m.attachments.map((a) => ({ video: "Видеосообщение", photo: "Фото", track: "Трек", voice: "Голосовое", document: "Документ" })[a.kind]).join(", ");
}

// /im/u/:id — открыть (или создать) личный диалог с пользователем
export function DirectPage({ userId }: { userId: number }) {
  const sending = useRef(false);
  const [error, setError] = useState<unknown>(null);
  const [text, setText] = useState("");
  const peer = useLoad(() => get<UserCard>(`/users/${userId}`), [userId]);
  async function send(e: Event) {
    e.preventDefault();
    if (sending.current || !text.trim()) return;
    sending.current = true;
    try {
      const m = await post<Message>(`/users/${userId}/messages`, { body: text });
      navigate(`/im/${m.conversation_id}`, true);
    } catch (err) { setError(err); } finally { sending.current = false; }
  }
  return (
    <section class="chat">
      <h1>{peer.data ? fullName(peer.data) : "Новый диалог"}</h1>
      <Empty>Сообщений ещё нет.</Empty>
      <form class="chat-form" onSubmit={send}>
        <MessageField value={text} onInput={(e) => setText((e.target as HTMLTextAreaElement).value)} />
        <button class="primary" disabled={!text.trim()}>Отправить</button>
      </form>
      <ErrorLine error={error} />
    </section>
  );
}

export function ChatPage({ id }: { id: number }) {
  const [editing,setEditing]=useState<Message|null>(null);
  const [reply,setReply]=useState<Message|null>(null);
  const [menu,setMenu]=useState<number|null>(null);
  const [selected,setSelected]=useState<number[]>([]);
  const [stars,setStars]=useState<number[]>([]);
  const [notice,setNotice]=useState("");
  const [submitting,setSubmitting]=useState(false);
  const [forward,setForward]=useState<Message|null>(null);
  const [targets,setTargets]=useState<Conversation[]>([]);
  const [search,setSearch]=useState("");
  const [showSearch,setShowSearch]=useState(false);
  const draft=useRef("");
  const [attachMenu,setAttachMenu]=useState(false);
  const [photo,setPhoto]=useState<{id:string;url:string}|null>(null);
  const [track,setTrack]=useState<{id:string;title:string}|null>(null);
  const [video,setVideo]=useState<VideoMessage|null>(null);
  const [videoPicker,setVideoPicker]=useState(false);
  const [giftPicker,setGiftPicker]=useState(false);
  const [servicePicker,setServicePicker]=useState(false);
  const [voice,setVoice]=useState<{id:string;duration_sec:number}|null>(null);
  const [musicChoices,setMusicChoices]=useState<{id:string;title:string}[]>([]);
  const [emoji,setEmoji]=useState(false);
  const [suggestions,setSuggestions]=useState<{id:string;url:string;title:string}[]>([]);
  const photoInput=useRef<HTMLInputElement>(null);
  const recorder=useRef<MediaRecorder|null>(null);
  const recordingTimer=useRef<ReturnType<typeof setTimeout>>();
  const [recording,setRecording]=useState(false);
  useEffect(()=>{void get<{items:{id:string;url:string;title:string}[]}>("/stickers").then(r=>setSuggestions(r.items),()=>{});return()=>{clearTimeout(recordingTimer.current);const rec=recorder.current;if(rec){rec.onstop=null;if(rec.state!=="inactive")rec.stop();rec.stream.getTracks().forEach(t=>t.stop());}};},[]);
  async function recordVoice(){try{if(recording){recorder.current?.stop();return;}const stream=await navigator.mediaDevices.getUserMedia({audio:true});let rec:MediaRecorder;try{rec=new MediaRecorder(stream);}catch(e){stream.getTracks().forEach(t=>t.stop());throw e;}recorder.current=rec;const chunks:BlobPart[]=[];rec.ondataavailable=e=>{if(e.data.size)chunks.push(e.data)};const timer=setTimeout(()=>{if(rec.state!=="inactive")rec.stop()},290000);recordingTimer.current=timer;rec.onstop=async()=>{clearTimeout(timer);stream.getTracks().forEach(t=>t.stop());setRecording(false);try{setVoice(await upload("/voice",new File(chunks,"voice.webm",{type:rec.mimeType})));}catch(e){setError(e);}};rec.start();setRecording(true);}catch(e){setError(e);}}
  async function chooseMusic(){setAttachMenu(false);try{setMusicChoices((await get<{items:{id:string;title:string}[]}>(`/users/${me()?.id}/music?limit=50`)).items);}catch(e){setError(e);}}
  function attachLocation(){setAttachMenu(false);navigator.geolocation.getCurrentPosition(p=>setText(old=>`${old}\nhttps://www.openstreetmap.org/?mlat=${p.coords.latitude.toFixed(5)}&mlon=${p.coords.longitude.toFixed(5)}`),e=>setError(new Error(e.message)),{timeout:15000});}
  useEffect(()=>{setEditing(null);setReply(null);setMenu(null);setSelected([]);setVideo(null);setVideoPicker(false);setGiftPicker(false);setServicePicker(false);try{const stored=JSON.parse(localStorage.getItem(`sn-message-stars-${me()?.id}`)||"[]");setStars(Array.isArray(stored)?stored.filter(x=>Number.isSafeInteger(x)):[]);}catch{setStars([]);}},[id]);
  useEffect(()=>{const close=(e:Event)=>{if(!(e.target as Element)?.closest?.(".message-actions"))setMenu(null);};const esc=(e:KeyboardEvent)=>{if(e.key==="Escape"){setMenu(null);setForward(null);}};document.addEventListener("pointerdown",close);document.addEventListener("keydown",esc);return()=>{document.removeEventListener("pointerdown",close);document.removeEventListener("keydown",esc);};},[]);
  function updateMessage(m:Message){setMsgs(xs=>xs?.map(x=>x.id===m.id?m:x)??null);setPinned(x=>x?.id===m.id?m:x);}
  useEffect(()=>on("message_updated",(m:Message)=>{if(m.conversation_id===id)updateMessage(m);}),[id]);
  function edit(m:Message){draft.current=text;setText(m.body||"");setEditing(m);setReply(null);setMenu(null);}
  function cancelEdit(){setEditing(null);setText(draft.current);}
  async function remove(m:Message){setMenu(null);if(!confirm("Удалить сообщение у всех участников?"))return;try{updateMessage(await del<Message>(`/conversations/${id}/messages/${m.id}`));if(editing?.id===m.id)cancelEdit();}catch(e){setError(e);}}
  function star(mid:number){const next=stars.includes(mid)?stars.filter(x=>x!==mid):[...stars,mid];try{localStorage.setItem(`sn-message-stars-${me()?.id}`,JSON.stringify(next));setStars(next);setMenu(null);setNotice("Отметка сохранена на этом устройстве");}catch(e){setError(e);}}
  async function copy(value:string){try{await navigator.clipboard.writeText(value);setNotice("Текст скопирован");setMenu(null);}catch(e){setError(e);}}
  async function chooseForward(m:Message){setMenu(null);try{setTargets((await get<{items:Conversation[]}>("/conversations")).items);setForward(m);}catch(e){setError(e);}}
  async function forwardTo(target:number){if(!forward||submitting)return;setSubmitting(true);try{await post(`/conversations/${target}/messages`,{body:`Пересланное сообщение:\n${forward.body}`});setForward(null);setNotice("Текст переслан");}catch(e){setError(e);}finally{setSubmitting(false);}}
  const sending = useRef(false);
  const [msgs, setMsgs] = useState<Message[] | null>(null);
  const [conv, setConv] = useState<Conversation | null>(null);
  const [text, setText] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [typing, setTyping] = useState(false);
  const end = useRef<HTMLDivElement>(null);
  const lastTyping = useRef(0);
  const docInput = useRef<HTMLInputElement>(null);
  const [doc, setDoc] = useState<Doc | null>(null);
  const [sticker,setSticker]=useState<{id:string;url:string;title:string}|null>(null);
  const [showStickers,setShowStickers]=useState(false);
  const [pinned,setPinned]=useState<Message|null>(null);
  async function loadPin(){const r=await get<{items:Message[]}>(`/conversations/${id}/pin`);setPinned(r.items[0]??null)}
  async function pin(message_id:number){try{await put(`/conversations/${id}/pin`,{message_id});await loadPin()}catch(e){setError(e)}}
  useEffect(()=>{setPinned(null);void loadPin().catch(setError);return on("conversation_pin",(d:{conversation_id:number})=>{if(d.conversation_id===id)void loadPin().catch(setError)})},[id]);

  useEffect(() => {
    setMsgs(null);
    get<Page<Message>>(`/conversations/${id}/messages?limit=50`).then((r) => setMsgs(r.items.slice().reverse()), setError);
    get<{ items: Conversation[] }>("/conversations").then((r) => setConv(r.items.find((c) => c.id === id) ?? null), () => {});
    let t: number | undefined;
    const offMsg = on("message", (m: Message) => {
      if (m.conversation_id !== id) return;
      setMsgs((xs) => (xs && !xs.some((x) => x.id === m.id) ? [...xs, m] : xs));
      if (m.sender_id !== me()?.id) { setTyping(false); clearTimeout(t); } // дописал — уже не печатает
    });
    const offTyping = on("typing", (d: { conversation_id: number }) => {
      if (d.conversation_id !== id) return;
      setTyping(true); clearTimeout(t); t = window.setTimeout(() => setTyping(false), 4000);
    });
    return () => { offMsg(); offTyping(); clearTimeout(t); };
  }, [id]);

  useEffect(() => {
    end.current?.scrollIntoView({ block: "end" });
    const last = msgs?.[msgs.length - 1];
    if (last && last.sender_id !== me()?.id) post(`/conversations/${id}/read`, { up_to: last.id }).catch(() => {});
  }, [msgs?.length]);

  async function send(e: Event) {
    e.preventDefault();
    if (sending.current || recording || (!text.trim() && !doc && !sticker && !photo && !track && !voice && !video)) return;
    sending.current = true;
    setSubmitting(true);
    setError(null);
    try {
      if(editing){const m=await put<Message>(`/conversations/${id}/messages/${editing.id}`,{body:text});updateMessage(m);cancelEdit();return;}
      const body=reply?`> ${(reply.body||"Вложение").slice(0,500)}\n\n${text}`:text;
      if(body.length>4000)throw new Error("Сообщение вместе с цитатой превышает 4000 символов.");
      const m = await post<Message>(`/conversations/${id}/messages`, { body, attachments: [...(doc?[{kind:"document",id:doc.id}]:[]),...(sticker?[{kind:"photo",id:sticker.id}]:[]),...(photo?[{kind:"photo",id:photo.id}]:[]),...(track?[{kind:"track",id:track.id}]:[]),...(voice?[{kind:"voice",id:voice.id}]:[]),...(video?[{kind:"video",id:video.id}]:[])] });
      setPhoto(null);setTrack(null);setVoice(null);setVideo(null);
      setReply(null);
      setText(""); setDoc(null); setSticker(null);
      setMsgs((xs) => (xs && !xs.some((x) => x.id === m.id) ? [...xs, m] : xs));
    } catch (err) { setError(err); } finally { sending.current = false; setSubmitting(false); }
  }

  function input(e: Event) {
    setText((e.target as HTMLInputElement).value);
    if (Date.now() - lastTyping.current > 3000) { lastTyping.current = Date.now(); sendTyping(id); }
  }

  const self = me()?.id;

  return (
    <section class="chat">
      <header class="chat-top"><a href="/im" aria-label="Все диалоги"><Icon name="close"/></a>{conv?.peer&&<Avatar user={conv.peer}/>}<h1>{conv ? title(conv) : "Диалог"}</h1>{conv?.peer&&<button class="icon-btn" aria-label="Позвонить" onClick={()=>startCall(conv.peer!)}><Icon name="phone"/></button>}<button class="icon-btn" aria-label="Поиск по загруженным сообщениям" onClick={()=>setShowSearch(!showSearch)}><Icon name="search"/></button></header>
      {showSearch&&<input type="search" aria-label="Поиск по загруженным сообщениям" placeholder="Поиск по загруженным сообщениям" value={search} onInput={e=>setSearch(e.currentTarget.value)}/>}
      {notice&&<p role="status">{notice} <button class="link" onClick={()=>setNotice("")}>Закрыть</button></p>}
      {selected.length>0&&<div class="message-selection">Выбрано: {selected.length}<button onClick={()=>copy((msgs||[]).filter(m=>selected.includes(m.id)).map(m=>m.body||"").join("\n"))}>Копировать текст</button><button onClick={()=>setSelected([])}>Отменить выбор</button></div>}
      {forward&&<div class="music-panel" role="region" aria-label="Переслать текст"><h2>Переслать текст в диалог</h2><p>Вложения не пересылаются.</p>{targets.map(c=><button disabled={submitting} onClick={()=>forwardTo(c.id)}>{title(c)}</button>)}<button onClick={()=>setForward(null)}>Отмена</button></div>}
      {conv?.event_id && <p class="muted">Чат похода: в нём все, кто идёт. <a href={`/events/${conv.event_id}`}>Страница похода</a></p>}
      <ErrorLine error={error} />
      {pinned&&<aside class="music-panel"><strong>Закреплённое сообщение</strong><p>{preview(pinned)}</p>{pinned.attachments.filter(a=>a.kind==="photo").map(a=><img src={a.thumb_url??a.url} alt="" style="max-height:100px"/>)}<button class="link" onClick={()=>document.getElementById(`message-${pinned.id}`)?.scrollIntoView({block:"center"})}>Перейти</button><button class="link" onClick={()=>pin(0)}>Открепить</button></aside>}
      <div class="chat-log" role="log" aria-live="polite">
        {msgs === null ? <Loading /> : msgs.filter(m=>!showSearch||!search||m.body?.toLocaleLowerCase().includes(search.toLocaleLowerCase())).map((m,index) => (
          m.kind === "service" ? <p key={m.id} class="msg-service">{m.body}</p> : (
            <div id={`message-${m.id}`} key={m.id} class={m.sender_id === self ? "msg mine" : "msg"}>
              {(index===0||new Date(msgs[index-1]?.created_at).toDateString()!==new Date(m.created_at).toDateString())&&<div class="chat-day">{new Date(m.created_at).toLocaleDateString("ru-RU",{day:"numeric",month:"long"})}</div>}
              <div class="message-author">{m.sender_id===self?fullName(me()!):conv?.peer?.id===m.sender_id?fullName(conv.peer):`Участник #${m.sender_id}`}{stars.includes(m.id)&&" ★"}</div>
              {(m.sender_id===self?me():conv?.peer?.id===m.sender_id?conv.peer:null)&&<div class="message-avatar"><Avatar user={(m.sender_id===self?me():conv!.peer)!}/></div>}
              {selected.includes(m.id)&&<span class="badge">Выбрано</span>}
              {m.encrypted ? <em>Зашифрованное сообщение</em> : m.body && <ServiceCard body={m.body}/>}
              {m.attachments.map((a) => (
                a.kind === "photo" ? <img key={a.id} src={a.thumb_url ?? a.url} alt="" /> :
                a.kind === "video" ? <PrivateVideo key={a.id} url={a.url}/> :
                a.kind === "track" ? <span key={a.id} class="track">{a.artist} — {a.title} <audio controls preload="none" src={a.url} /></span> :
                a.kind === "document" ? <DocList key={a.id} docs={[{ id: a.id, title: a.title ?? "Документ", ext: a.ext ?? "", size: a.size ?? 0, url: a.url }]} /> :
                <VoiceNote key={a.id} url={a.url} seconds={a.duration_sec ?? 0} />
              ))}
              <span class="message-time" title={new Date(m.created_at).toLocaleString("ru-RU")}>{new Date(m.created_at).toLocaleTimeString("ru-RU",{hour:"2-digit",minute:"2-digit"})}{m.edited_at&&!m.deleted_at&&" · изменено"}</span>
              {!m.deleted_at&&!m.encrypted&&<div class="message-actions"><button class="icon-btn" aria-label="Действия с сообщением" aria-expanded={menu===m.id} onClick={()=>setMenu(menu===m.id?null:m.id)}>•••</button>{menu===m.id&&<div class="message-menu">
                <button onClick={()=>{setReply(m);setMenu(null);if(editing)cancelEdit();}}>↩ Ответить</button>
                {m.body&&<button onClick={()=>chooseForward(m)}>↗ Переслать текст</button>}
                <button onClick={()=>star(m.id)}>☆ {stars.includes(m.id)?"Убрать из важного":"Отметить как важное"}</button>
                {m.body&&<button onClick={()=>copy(m.body!)}>Копировать текст</button>}
                {m.sender_id===self&&<button onClick={()=>edit(m)}><Icon name="edit"/>Редактировать</button>}
                {m.sender_id===self&&<button class="danger" onClick={()=>remove(m)}><Icon name="trash"/>Удалить</button>}
                <button onClick={()=>{setSelected(xs=>xs.includes(m.id)?xs.filter(x=>x!==m.id):[...xs,m.id]);setMenu(null);}}>Выбрать</button>
                <button onClick={()=>{void pin(m.id);setMenu(null);}}>Закрепить</button>
              </div>}</div>}
            </div>
          )
        ))}
        {typing && <p class="muted typing">печатает…</p>}
        <div ref={end} />
      </div>
      {videoPicker&&<VideoRecorder key={id} close={()=>setVideoPicker(false)} done={v=>{setVideo(v);setVideoPicker(false);}}/>}
      {giftPicker&&conv?.peer&&<ChatGifts key={id} recipient={conv.peer.id} close={()=>setGiftPicker(false)}/> }
      <AIText text={text} apply={setText}/>{showStickers&&<CustomStickers select={x=>{setSticker(x);setShowStickers(false)}}/>}
      {!editing&&text.trim().length>1&&suggestions.some(x=>x.title.toLocaleLowerCase().includes(text.trim().toLocaleLowerCase()))&&<div class="chat-sticker-suggestions">{suggestions.filter(x=>x.title.toLocaleLowerCase().includes(text.trim().toLocaleLowerCase())).slice(0,5).map(x=><button title={x.title} onClick={()=>{setSticker(x);setText("");}}><img src={x.url} alt={x.title}/></button>)}</div>}
      {photo&&<p><img src={photo.url} alt="Прикреплённое фото" style="max-height:100px"/><button onClick={()=>setPhoto(null)}>Убрать фото</button></p>}
      {track&&<p>{track.title}<button onClick={()=>setTrack(null)}>Убрать музыку</button></p>}
      {video&&<div><PrivateVideo key={video.id} url={video.url}/><button onClick={()=>setVideo(null)}>Убрать видео</button></div>}
      {servicePicker&&<div class="music-panel" aria-label="Сервисы socialnet"><h2>Поделиться сервисом</h2>{chatServices.map(s=><button onClick={()=>{setText(t=>`${t?t+"\n":""}Сервис socialnet: ${s.name}\n${s.path}`);setServicePicker(false);}}>{s.name}</button>)}<button onClick={()=>setServicePicker(false)}>Закрыть</button></div>}
      {voice&&<p>Голосовое · {duration(voice.duration_sec)}<button onClick={()=>setVoice(null)}>Убрать</button></p>}
      {musicChoices.length>0&&<div class="music-panel">{musicChoices.map(t=><button onClick={()=>{setTrack(t);setMusicChoices([]);}}>{t.title}</button>)}<button onClick={()=>setMusicChoices([])}>Закрыть</button></div>}
      {emoji&&<div class="chat-emoji" aria-label="Эмодзи">{["😀","😂","😊","😍","😮","😢","😡","👍","❤️","🔥","🎉","🙏"].map(x=><button onClick={()=>{setText(t=>t+x);setEmoji(false);}}>{x}</button>)}</div>}
      {sticker&&<p><img src={sticker.url} alt={sticker.title} style="max-height:120px"/><button class="link" onClick={()=>setSticker(null)}>Убрать</button></p>}
      {doc && <p class="muted">Документ: {doc.title}.{doc.ext} <button class="link" onClick={() => setDoc(null)}>убрать</button></p>}
      <form class="chat-form" onSubmit={send}>
        {(editing||reply)&&<div class="message-context"><strong>{editing?"Редактируемое сообщение":"Ответ с цитатой"}</strong><p>{(editing||reply)?.body||"Вложение"}</p><button type="button" aria-label="Отменить" onClick={()=>editing?cancelEdit():setReply(null)}><Icon name="close"/></button></div>}
        <input ref={docInput} type="file" hidden onChange={async (e) => {
          const f = (e.target as HTMLInputElement).files?.[0];
          if (f) { setError(null); try { setDoc(await upload<Doc>("/documents", f)); } catch (err) { setError(err); } }
        }} />
        <input ref={photoInput} type="file" accept="image/*" hidden onChange={async e=>{const f=e.currentTarget.files?.[0];e.currentTarget.value="";if(f)try{setPhoto(await upload("/media",f));}catch(e){setError(e);}}}/>
        {!editing&&<div class="chat-attachment"><button type="button" class="icon-btn" aria-label="Прикрепить" aria-expanded={attachMenu} onClick={()=>setAttachMenu(!attachMenu)}><Icon name="plus"/></button>{attachMenu&&<div class="message-menu"><button type="button" onClick={()=>{photoInput.current?.click();setAttachMenu(false);}}><Icon name="photo"/>Фото</button><button type="button" onClick={chooseMusic}><Icon name="music"/>Музыка</button><button type="button" onClick={()=>{docInput.current?.click();setAttachMenu(false);}}><Icon name="file"/>Файл</button><button type="button" onClick={()=>{setServicePicker(true);setAttachMenu(false);}}>▦ Сервис</button>{conv?.peer&&<button type="button" onClick={()=>{setGiftPicker(true);setAttachMenu(false);}}>🎁 Подарок</button>}<button type="button" onClick={attachLocation}>Отправить моё местоположение</button></div>}</div>}
        <button type="button" class="icon-btn" aria-label="Стикеры" title="Стикеры" onClick={()=>setShowStickers(!showStickers)}>▧</button><MessageField value={text} onInput={input} />
        <button type="button" class="icon-btn" aria-label="Эмодзи" onClick={()=>setEmoji(!emoji)}>☺</button>
        {!editing&&<button type="button" class="icon-btn" disabled={recording} aria-label="Записать видеосообщение" onClick={()=>setVideoPicker(true)}>▷</button>}
        {!editing&&<button type="button" class="icon-btn" disabled={videoPicker} aria-label={recording?"Закончить запись":"Записать голосовое сообщение"} onClick={recordVoice}>{recording?"■":"🎙"}</button>}
        <button class="primary chat-send" aria-label={editing?"Сохранить":"Отправить"} title={editing?"Сохранить":"Отправить"} disabled={submitting||recording||(editing?!text.trim():!text.trim() && !doc && !sticker && !photo && !track && !voice && !video)}>{editing?"✓":"➤"}</button>
      </form>
    </section>
  );
}

// Голосовые закрыты токеном — их нельзя дать <audio> по ссылке, грузим через fetch.
function VoiceNote({ url, seconds }: { url: string; seconds: number }) {
  const [src, setSrc] = useState<string | null>(null);
  async function load() {
    const blob = await fetchBlob(url).catch(() => null);
    if (blob) setSrc(URL.createObjectURL(blob));
  }
  return src ? <audio controls autoPlay src={src} /> : <button class="chip" onClick={load}>▶ Голосовое {duration(seconds)}</button>;
}
