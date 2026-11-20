import { AIText } from "./ai.js";
import { CustomStickers } from "./customstickers.js";
import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Conversation, Doc, Message, Page, UserCard, fetchBlob, fullName, get, post, put, upload } from "./api.js";
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
  return m.attachments.map((a) => ({ photo: "Фото", track: "Трек", voice: "Голосовое", document: "Документ" })[a.kind]).join(", ");
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
    if (sending.current || (!text.trim() && !doc && !sticker)) return;
    sending.current = true;
    setError(null);
    try {
      const m = await post<Message>(`/conversations/${id}/messages`, { body: text, attachments: [...(doc?[{kind:"document",id:doc.id}]:[]),...(sticker?[{kind:"photo",id:sticker.id}]:[])] });
      setText(""); setDoc(null); setSticker(null);
      setMsgs((xs) => (xs && !xs.some((x) => x.id === m.id) ? [...xs, m] : xs));
    } catch (err) { setError(err); } finally { sending.current = false; }
  }

  function input(e: Event) {
    setText((e.target as HTMLInputElement).value);
    if (Date.now() - lastTyping.current > 3000) { lastTyping.current = Date.now(); sendTyping(id); }
  }

  const self = me()?.id;
  return (
    <section class="chat">
      <h1><a href="/im" class="back" aria-label="Все диалоги">‹</a> {conv ? title(conv) : "Диалог"}</h1>
      {conv?.peer&&<div class="chat-call-actions"><button class="ghost" onClick={()=>startCall(conv.peer!)}>Позвонить</button><button class="ghost" onClick={()=>startCall(conv.peer!,true)}>Видеозвонок</button></div>}
      {conv?.event_id && <p class="muted">Чат похода: в нём все, кто идёт. <a href={`/events/${conv.event_id}`}>Страница похода</a></p>}
      <ErrorLine error={error} />
      {pinned&&<aside class="music-panel"><strong>Закреплённое сообщение</strong><p>{preview(pinned)}</p>{pinned.attachments.filter(a=>a.kind==="photo").map(a=><img src={a.thumb_url??a.url} alt="" style="max-height:100px"/>)}<button class="link" onClick={()=>document.getElementById(`message-${pinned.id}`)?.scrollIntoView({block:"center"})}>Перейти</button><button class="link" onClick={()=>pin(0)}>Открепить</button></aside>}
      <div class="chat-log" role="log" aria-live="polite">
        {msgs === null ? <Loading /> : msgs.map((m) => (
          m.kind === "service" ? <p key={m.id} class="msg-service">{m.body}</p> : (
            <div id={`message-${m.id}`} key={m.id} class={m.sender_id === self ? "msg mine" : "msg"}>
              {m.encrypted ? <em>Зашифрованное сообщение</em> : m.body && <p>{m.body}</p>}
              {m.attachments.map((a) => (
                a.kind === "photo" ? <img key={a.id} src={a.thumb_url ?? a.url} alt="" /> :
                a.kind === "track" ? <span key={a.id} class="track">{a.artist} — {a.title} <audio controls preload="none" src={a.url} /></span> :
                a.kind === "document" ? <DocList key={a.id} docs={[{ id: a.id, title: a.title ?? "Документ", ext: a.ext ?? "", size: a.size ?? 0, url: a.url }]} /> :
                <VoiceNote key={a.id} url={a.url} seconds={a.duration_sec ?? 0} />
              ))}
              <Time iso={m.created_at} /><button type="button" class="link" onClick={()=>pin(m.id)}>Закрепить</button>
            </div>
          )
        ))}
        {typing && <p class="muted typing">печатает…</p>}
        <div ref={end} />
      </div>
      <AIText text={text} apply={setText}/>{showStickers&&<CustomStickers select={x=>{setSticker(x);setShowStickers(false)}}/>}
      {sticker&&<p><img src={sticker.url} alt={sticker.title} style="max-height:120px"/><button class="link" onClick={()=>setSticker(null)}>Убрать</button></p>}
      {doc && <p class="muted">Документ: {doc.title}.{doc.ext} <button class="link" onClick={() => setDoc(null)}>убрать</button></p>}
      <form class="chat-form" onSubmit={send}>
        <input ref={docInput} type="file" hidden onChange={async (e) => {
          const f = (e.target as HTMLInputElement).files?.[0];
          if (f) { setError(null); try { setDoc(await upload<Doc>("/documents", f)); } catch (err) { setError(err); } }
        }} />
        <button type="button" class="icon-btn" aria-label="Прикрепить документ" onClick={() => docInput.current?.click()}>📎</button>
        <button type="button" class="ghost" onClick={()=>setShowStickers(!showStickers)}>Стикеры</button><MessageField value={text} onInput={input} />
        <button class="primary" disabled={!text.trim() && !doc && !sticker}>Отправить</button>
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
