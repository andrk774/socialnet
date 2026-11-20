import { h, Fragment } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { Audience, Comment, Doc, Page, Photo, Poll, Post, PostDocument, Reaction, Route, Scheduled, UserCard, api, del, fileSize, fullName, get, patch, post, put, saveFile, upload } from "./api.js";
import { me } from "./session.js";
import { RouteCard, km } from "./routes.js";
import { Avatar, Empty, ErrorLine, Loading, RichText, Time, UserLink, duration, useLoad } from "./ui.js";
import { ReportButton } from "./report.js";
import { Icon } from "./shellparts.js";
import { CommentExtras, CommentMedia, emptyDraft } from "./commenttools.js";

const reactions: [Reaction, string, string][] = [
  ["like", "❤️", "Нравится"], ["love", "😍", "Люблю"], ["haha", "😂", "Смешно"],
  ["wow", "😮", "Ого"], ["sad", "😢", "Грустно"], ["angry", "😠", "Возмутительно"],
];

// Свои эмодзи сообществ: загружаются один раз на сообщество.
export interface Emoji { id: number; community_id: number; shortcode: string; url: string; reaction: string }
const emojiCache = new Map<number, Promise<Emoji[]>>();
export function communityEmoji(id: number, fresh = false): Promise<Emoji[]> {
  if (fresh || !emojiCache.has(id)) {
    emojiCache.set(id, get<{ items: Emoji[] }>(`/communities/${id}/emoji`).then((r) => r.items).catch(() => { emojiCache.delete(id); return []; }));
  }
  return emojiCache.get(id)!;
}

function ReactionIcon({ r, custom }: { r: string; custom: Emoji[] }) {
  if (r === "like") return <Heart filled />;
  const std = reactions.find(([k]) => k === r);
  if (std) return <>{std[1]}</>;
  const e = custom.find((x) => x.reaction === r);
  return e ? <img class="emoji" src={e.url} alt={`:${e.shortcode}:`} title={`:${e.shortcode}:`} width={20} height={20} /> : <>👍</>;
}

function Heart({ filled = false }: { filled?: boolean }) {
  return <svg class={filled ? "heart-icon heart-active" : "heart-icon"} width="22" height="22" viewBox="0 0 24 24" aria-hidden="true" fill={filled ? "currentColor" : "none"} stroke="currentColor" stroke-width="1.8"><path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.6a5.5 5.5 0 0 0 0-7.8Z" /></svg>;
}

const audienceLabel: Record<Audience, string> = { everyone: "Все", friends: "Друзья", only_me: "Только я", list: "Выбранные друзья" };

interface PollDraft { question: string; options: string[]; multiple: boolean; anonymous: boolean }

// localInput: значение для <input type="datetime-local"> в часовом поясе браузера.
const localInput = (d: Date) => new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);

export function Composer({ communityId, onPosted }: { communityId?: number; onPosted: (p: Post) => void }) {
  const [text, setText] = useState("");
  const [photos, setPhotos] = useState<Photo[]>([]);
  const [docs, setDocs] = useState<Doc[]>([]);
  const [routes, setRoutes] = useState<Route[]>([]);
  const [hideEnds, setHideEnds] = useState("200");
  const [poll, setPoll] = useState<PollDraft | null>(null);
  const [audience, setAudience] = useState<Audience>("everyone");
  const [picked, setPicked] = useState<number[]>([]);
  const [when, setWhen] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [note, setNote] = useState("");
  const file = useRef<HTMLInputElement>(null);
  const docFile = useRef<HTMLInputElement>(null);
  const gpxFile = useRef<HTMLInputElement>(null);
  const friends = useLoad(() => (audience === "list" ? get<Page<UserCard>>(`/users/${me()!.id}/friends?limit=50`) : Promise.resolve(null)), [audience]);

  async function addPhotos(files: FileList | null) {
    if (!files) return;
    setError(null);
    for (const f of Array.from(files).slice(0, 10 - photos.length)) {
      try {
        const p = await upload<Photo>("/media", f);
        setPhotos((xs) => [...xs, p]);
      } catch (e) { setError(e); }
    }
  }

  async function addDocs(files: FileList | null) {
    if (!files) return;
    setError(null);
    for (const f of Array.from(files).slice(0, 10 - docs.length)) {
      try {
        const d = await upload<Doc>("/documents", f);
        setDocs((xs) => [...xs, d]);
      } catch (e) { setError(e); }
    }
  }

  async function addRoute(files: FileList | null) {
    const f = files?.[0];
    if (!f) return;
    setError(null);
    try { setRoutes([...routes, await upload<Route>("/routes", f, { hidden_ends_m: hideEnds })]); } catch (e) { setError(e); }
    if (gpxFile.current) gpxFile.current.value = "";
  }

  async function submit(e: Event) {
    e.preventDefault();
    setBusy(true); setError(null); setNote("");
    const body: Record<string, unknown> = { text, photo_ids: photos.map((x) => x.id), document_ids: docs.map((x) => x.id), route_ids: routes.map((x) => x.id) };
    if (communityId) body.community_id = communityId;
    else { body.audience = audience; if (audience === "list") body.audience_ids = picked; }
    if (poll) body.poll = { ...poll, options: poll.options.filter((o) => o.trim()) };
    if (when) body.publish_at = new Date(when).toISOString();
    try {
      const r = await post<Post | Scheduled>("/posts", body);
      setText(""); setPhotos([]); setDocs([]); setRoutes([]); setPoll(null); setWhen(null);
      if (when) setNote(`Запись выйдет ${new Date((r as Scheduled).publish_at).toLocaleString("ru", { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" })}. Её можно перенести в разделе «Отложенные».`);
      else onPosted(r as Post);
    } catch (err) { setError(err); } finally { setBusy(false); }
  }

  const setOpt = (i: number, v: string) => setPoll(poll && { ...poll, options: poll.options.map((o, j) => (j === i ? v : o)) });
  const empty = !text.trim() && photos.length === 0 && docs.length === 0 && routes.length === 0 && !poll;
  return (
    <form class="composer" onSubmit={submit}>
      <label class="sr-only" for="composer-text">Текст записи</label>
      <textarea id="composer-text" placeholder="Что у вас нового?" value={text} maxLength={10000}
        onInput={(e) => setText((e.target as HTMLTextAreaElement).value)} rows={text.split("\n").length > 2 ? 5 : 2} />
      {photos.length > 0 && (
        <ul class="composer-photos">
          {photos.map((p) => (
            <li key={p.id}>
              <img src={p.thumb_url} alt="" />
              <button type="button" class="icon-btn" aria-label="Убрать фото" onClick={() => setPhotos(photos.filter((x) => x !== p))}>×</button>
            </li>
          ))}
        </ul>
      )}
      {docs.length > 0 && (
        <ul class="doc-list">
          {docs.map((d) => (
            <li key={d.id} class="doc">
              <span class="doc-ext" aria-hidden="true">{d.ext}</span>
              <span class="doc-name">{d.title}<span class="muted"> · {fileSize(d.size)}</span></span>
              <button type="button" class="icon-btn" aria-label={`Убрать документ ${d.title}`} onClick={() => setDocs(docs.filter((x) => x !== d))}>×</button>
            </li>
          ))}
        </ul>
      )}
      {routes.length > 0 && (
        <ul class="doc-list">
          {routes.map((r) => (
            <li key={r.id} class="doc">
              <span class="doc-ext" aria-hidden="true">gpx</span>
              <span class="doc-name">{r.title}<span class="muted"> · {km(r.distance_m)}{r.max_ele !== null ? ` · ↑ ${r.gain_m} м` : ""}</span></span>
              <button type="button" class="icon-btn" aria-label={`Убрать трек ${r.title}`} onClick={() => setRoutes(routes.filter((x) => x !== r))}>×</button>
            </li>
          ))}
        </ul>
      )}
      {poll && (
        <fieldset class="poll-edit">
          <legend>Опрос</legend>
          <label class="sr-only" for="poll-q">Вопрос</label>
          <input id="poll-q" placeholder="Вопрос" maxLength={255} value={poll.question} onInput={(e) => setPoll({ ...poll, question: (e.target as HTMLInputElement).value })} />
          {poll.options.map((o, i) => (
            <div class="row tight" key={i}>
              <label class="sr-only" for={`poll-o${i}`}>Вариант {i + 1}</label>
              <input id={`poll-o${i}`} placeholder={`Вариант ${i + 1}`} maxLength={100} value={o} onInput={(e) => setOpt(i, (e.target as HTMLInputElement).value)} />
              {poll.options.length > 2 && <button type="button" class="icon-btn" aria-label={`Убрать вариант ${i + 1}`} onClick={() => setPoll({ ...poll, options: poll.options.filter((_, j) => j !== i) })}>×</button>}
            </div>
          ))}
          <div class="row tight">
            {poll.options.length < 10 && <button type="button" class="link" onClick={() => setPoll({ ...poll, options: [...poll.options, ""] })}>Добавить вариант</button>}
            <label><input type="checkbox" checked={poll.multiple} onChange={(e) => setPoll({ ...poll, multiple: (e.target as HTMLInputElement).checked })} /> Несколько ответов</label>
            <label><input type="checkbox" checked={poll.anonymous} onChange={(e) => setPoll({ ...poll, anonymous: (e.target as HTMLInputElement).checked })} /> Анонимный</label>
            <button type="button" class="link" onClick={() => setPoll(null)}>Убрать опрос</button>
          </div>
        </fieldset>
      )}
      {audience === "list" && !communityId && (
        <fieldset class="audience-pick">
          <legend>Кому показать</legend>
          {!friends.data ? <Loading /> : friends.data.items.length === 0 ? <p class="muted">Друзей пока нет.</p> :
            friends.data.items.map((f) => (
              <label key={f.id}><input type="checkbox" checked={picked.includes(f.id)}
                onChange={(e) => setPicked((e.target as HTMLInputElement).checked ? [...picked, f.id] : picked.filter((x) => x !== f.id))} /> {fullName(f)}</label>
            ))}
        </fieldset>
      )}
      {when !== null && (
        <div class="row tight">
          <label for="publish-at">Опубликовать</label>
          <input id="publish-at" type="datetime-local" value={when} min={localInput(new Date())} onInput={(e) => setWhen((e.target as HTMLInputElement).value)} />
          <button type="button" class="link" onClick={() => setWhen(null)}>Сейчас</button>
        </div>
      )}
      <ErrorLine error={error} />
      {note && <p class="note" role="status">{note}</p>}
      <div class="composer-bar">
        <div class="composer-tools">
          <input ref={file} type="file" accept="image/jpeg,image/png,image/gif" multiple hidden onChange={(e) => addPhotos((e.target as HTMLInputElement).files)} />
          <input ref={docFile} type="file" multiple hidden onChange={(e) => addDocs((e.target as HTMLInputElement).files)} />
          <input ref={gpxFile} type="file" accept=".gpx,application/gpx+xml" hidden data-kind="gpx" onChange={(e) => addRoute((e.target as HTMLInputElement).files)} />
          <button type="button" class="ghost" onClick={() => file.current?.click()} disabled={photos.length >= 10}>Фото</button>
          <button type="button" class="ghost" onClick={() => docFile.current?.click()} disabled={docs.length >= 10}>Документ</button>
          <span class="track-pick">
            <button type="button" class="ghost" onClick={() => gpxFile.current?.click()} disabled={routes.length >= 3}>Трек GPX</button>
            <select aria-label="Скрыть начало и конец трека" title="Скрыть начало и конец трека, чтобы по нему нельзя было найти ваш дом"
              value={hideEnds} onChange={(e) => setHideEnds((e.target as HTMLSelectElement).value)}>
              <option value="0">концы видны</option><option value="200">скрыть 200 м</option>
              <option value="500">скрыть 500 м</option><option value="1000">скрыть 1 км</option>
            </select>
          </span>
          <button type="button" class="ghost" aria-pressed={!!poll} onClick={() => setPoll(poll ? null : { question: "", options: ["", ""], multiple: false, anonymous: false })}>Опрос</button>
          <button type="button" class="ghost" aria-pressed={when !== null} onClick={() => setWhen(when === null ? localInput(new Date(Date.now() + 3600_000)) : null)}>Отложить</button>
          {!communityId && (
            <select aria-label="Кто увидит запись" value={audience} onChange={(e) => setAudience((e.target as HTMLSelectElement).value as Audience)}>
              {(Object.keys(audienceLabel) as Audience[]).map((a) => <option key={a} value={a}>{audienceLabel[a]}</option>)}
            </select>
          )}
        </div>
        <button type="submit" class="primary" disabled={busy || empty || (audience === "list" && picked.length === 0 && !communityId)}>
          {busy ? "Публикуем…" : when ? "Запланировать" : "Опубликовать"}
        </button>
      </div>
    </form>
  );
}

export function DocList({ docs }: { docs: PostDocument[] }) {
  const [error, setError] = useState<unknown>(null);
  return (
    <Fragment>
      <ul class="doc-list">
        {docs.map((d) => (
          <li key={d.id} class="doc">
            <span class="doc-ext" aria-hidden="true">{d.ext}</span>
            <button class="link doc-name" onClick={() => saveFile(d.url, `${d.title}.${d.ext}`).catch(setError)}>{d.title}.{d.ext}</button>
            <span class="muted">{fileSize(d.size)}</span>
          </li>
        ))}
      </ul>
      <ErrorLine error={error} />
    </Fragment>
  );
}

function PollView({ postId, initial }: { postId: number; initial: Poll }) {
  const [poll, setPoll] = useState(initial);
  const [chosen, setChosen] = useState<number[]>([]);
  const [error, setError] = useState<unknown>(null);
  const voted = poll.my_votes.length > 0;
  const canVote = !poll.closed && !voted;

  async function vote(ids: number[]) {
    setError(null);
    try { setPoll(await post<Poll>(`/posts/${postId}/poll/votes`, { option_ids: ids })); setChosen([]); } catch (e) { setError(e); }
  }
  async function retract() {
    setError(null);
    try { setPoll(await del<Poll>(`/posts/${postId}/poll/votes`)); } catch (e) { setError(e); }
  }

  const total = poll.voters_count;
  return (
    <fieldset class="poll">
      <legend>{poll.question}</legend>
      <p class="muted">
        {poll.anonymous ? "Анонимный опрос" : "Публичный опрос"}{poll.multiple && " · несколько ответов"}
        {poll.closed ? " · завершён" : poll.closes_at ? ` · до ${new Date(poll.closes_at).toLocaleString("ru", { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" })}` : ""}
      </p>
      <ul class="poll-options">
        {poll.options.map((o) => {
          const pct = o.votes !== null && total > 0 ? Math.round((o.votes / total) * 100) : 0;
          const mine = poll.my_votes.includes(o.id);
          return (
            <li key={o.id}>
              {canVote ? (
                poll.multiple ? (
                  <label class="poll-choice"><input type="checkbox" checked={chosen.includes(o.id)}
                    onChange={(e) => setChosen((e.target as HTMLInputElement).checked ? [...chosen, o.id] : chosen.filter((x) => x !== o.id))} /> {o.text}</label>
                ) : <button class="poll-choice" onClick={() => vote([o.id])}>{o.text}</button>
              ) : (
                <div class={mine ? "poll-result mine" : "poll-result"}>
                  <span class="poll-bar" style={{ width: `${pct}%` }} aria-hidden="true" />
                  <span class="poll-text">{o.text}{mine && <span class="sr-only"> (ваш выбор)</span>}{mine && " ✓"}</span>
                  {o.votes !== null && <span class="poll-pct">{pct}%</span>}
                </div>
              )}
            </li>
          );
        })}
      </ul>
      <div class="row tight">
        <span class="muted">{total === 0 ? "Пока никто не голосовал" : `Проголосовали: ${total}`}</span>
        {canVote && poll.multiple && <button class="primary" disabled={chosen.length === 0} onClick={() => vote(chosen)}>Голосовать</button>}
        {voted && !poll.closed && <button class="link" onClick={retract}>Отменить голос</button>}
      </div>
      <ErrorLine error={error} />
    </fieldset>
  );
}

export function PostView({ initial, onDelete }: { initial: Post; onDelete?: () => void }) {
  const [p, setP] = useState(initial);
  const [open, setOpen] = useState(false);
  const [picking, setPicking] = useState(false);
  const [photoIndex, setPhotoIndex] = useState<number | null>(null);
  const mine = me()?.id === p.author.id;

  const [error, setError] = useState<unknown>(null);
  // любое действие с записью: ошибка показывается под записью, а состояние
  // меняется только после ответа сервера — интерфейс не врёт о несохранённом
  const act = async (fn: () => Promise<void>) => {
    setError(null);
    try { await fn(); } catch (e) { setError(e); }
  };

  const [custom, setCustom] = useState<Emoji[]>([]);
  useEffect(() => { if (p.community) communityEmoji(p.community.id).then(setCustom); }, [p.community?.id]);

  const react = (r: Reaction | string | null) => act(async () => {
    setPicking(false);
    const res = r === null || r === p.my_reaction
      ? await api<{ likes_count: number }>("DELETE", `/posts/${p.id}/reactions`)
      : await post<{ likes_count: number }>(`/posts/${p.id}/reactions`, { reaction: r });
    setP(await get<Post>(`/posts/${p.id}`).catch(() => ({ ...p, likes_count: res.likes_count })));
  });

  const remove = () => act(async () => {
    if (!confirm("Удалить запись?")) return;
    await del(`/posts/${p.id}`);
    onDelete?.();
  });

  const bookmark = () => act(async () => {
    if (p.bookmarked) await del(`/bookmarks/post/${p.id}`);
    else await put(`/bookmarks/post/${p.id}`);
    setP({ ...p, bookmarked: !p.bookmarked });
  });

  const changeAudience = (a: Audience, select: HTMLSelectElement) => act(async () => {
    select.value = p.audience; // пока сервер не подтвердил, показываем сохранённое значение
    if (a === "list") { alert("Выбрать отдельных друзей можно при создании записи."); return; }
    setP(await patch<Post>(`/posts/${p.id}`, { audience: a }));
  });

  const top = (Object.entries(p.reactions) as [string, number][]).filter(([, n]) => n > 0).sort((a, b) => b[1] - a[1]).slice(0, 3);
  const mineR = reactions.find(([r]) => r === p.my_reaction);
  const mineCustom = custom.find((e) => e.reaction === p.my_reaction);
  return (
    <article class="post" aria-labelledby={`post-${p.id}-author`}>
      <header class="post-head">
        <Avatar user={p.author} />
        <div>
          <div id={`post-${p.id}-author`}>
            {p.community ? <a class="user-link" href={`/c/${p.community.slug}`}>{p.community.name}</a> : <UserLink user={p.author} />}
          </div>
          <a class="muted" href={`/p/${p.id}`}><Time iso={p.created_at} /></a>
          {!p.community && p.audience !== "everyone" && <span class="muted"> · видят: {audienceLabel[p.audience].toLowerCase()}</span>}
        </div>
        {mine && <button class="icon-btn post-menu" aria-label="Удалить запись" onClick={remove}>×</button>}
      </header>
      {p.text && <p class="post-text"><RichText text={p.text} /></p>}
      {p.photos.length > 0 && (
        <div class={`post-photos n${Math.min(p.photos.length, 4)}`}>
          {p.photos.map((ph, index) => <a key={ph.id} href={ph.url} aria-label={`Открыть фотографию ${index + 1}`} onClick={(e) => { if (e.button !== 0 || e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return; e.preventDefault(); setPhotoIndex(index); }}><img src={ph.url} alt="" loading="lazy" decoding="async" /></a>)}
        </div>
      )}
      {p.tracks.map((t) => (
        <div class="track" key={t.id}><span>{t.artist} — {t.title}</span><span class="muted">{duration(t.duration_sec)}</span><audio controls preload="none" src={t.url} /></div>
      ))}
      {p.videos.map((v) => <Video key={v.id} src={v.hls_url} mp4={v.mp4_url} poster={v.poster_url} title={v.title} />)}
      {p.routes?.map((r) => <RouteCard key={r.id} route={r} />)}
      {p.documents.length > 0 && <DocList docs={p.documents} />}
      {p.poll && <PollView postId={p.id} initial={p.poll} />}
      <footer class="post-actions">
        <div class="react-wrap">
          <button class={p.my_reaction ? "chip active" : "chip"} aria-pressed={!!p.my_reaction}
            title={p.my_reaction ? "Убрать реакцию" : "Нравится"}
            onClick={() => (p.my_reaction ? react(null) : react("like"))}
            onContextMenu={(e) => { e.preventDefault(); setPicking(!picking); }}>
            {p.my_reaction ? <ReactionIcon r={p.my_reaction} custom={custom} /> : <Heart />}
            <span class="sr-only">{mineR ? `Ваша реакция: ${mineR[2]}. Убрать` : mineCustom ? `Ваша реакция: ${mineCustom.shortcode}. Убрать` : "Нравится"}</span>
          </button>
          <button class="chip" aria-expanded={picking} aria-label="Выбрать реакцию" onClick={() => setPicking(!picking)}>…</button>
          {picking && (
            <div class="react-picker" role="menu">
              {reactions.map(([r, emoji, label]) => <button role="menuitem" key={r} title={label} aria-label={label} onClick={() => react(r)}>{emoji}</button>)}
              {custom.map((e) => <button role="menuitem" key={e.id} title={`:${e.shortcode}:`} aria-label={e.shortcode} onClick={() => react(e.reaction)}><img class="emoji" src={e.url} alt="" width={22} height={22} /></button>)}
              <div class="reaction-picker-footer"><ReactionUsers postId={p.id} count={p.likes_count} custom={custom} panel /></div>
            </div>
          )}
          <ReactionUsers postId={p.id} count={p.likes_count} custom={custom} />
          {top.length > 1 && <span class="muted react-top">{top.map(([r]) => <ReactionIcon key={r} r={r} custom={custom} />)}</span>}
        </div>
        <button class="chip post-comment-action" title="Оставить комментарий" aria-label={`Комментарии${p.comments_count>0?` ${p.comments_count}`:""}`} aria-expanded={open} onClick={() => setOpen(!open)}>
          <Icon name="message"/><span>{p.comments_count}</span>
        </button>
        <SharePost postId={p.id}/>
        <button class={p.bookmarked ? "chip active" : "chip"} aria-pressed={p.bookmarked} onClick={bookmark}>
          {p.bookmarked ? "В закладках" : "В закладки"}
        </button>
        {mine && !p.community && (
          <select class="chip-select" aria-label="Кто видит запись" value={p.audience} onChange={(e) => { const el = e.target as HTMLSelectElement; changeAudience(el.value as Audience, el); }}>
            {(Object.keys(audienceLabel) as Audience[]).filter((a) => a !== "list" || p.audience === "list").map((a) => <option key={a} value={a}>{audienceLabel[a]}</option>)}
          </select>
        )}
        {!mine && <ReportButton target={{ type: "post", id: p.id }} cls="chip" />}
        <a class="post-footer-time" href={`/p/${p.id}`}><Time iso={p.created_at}/></a>
      </footer>
      <ErrorLine error={error} />
      {open && photoIndex === null && <Comments post={p} onCount={(n) => setP({ ...p, comments_count: n })} />}
      {photoIndex !== null && <PhotoViewer p={p} initialIndex={photoIndex} close={() => setPhotoIndex(null)} react={() => react(p.my_reaction ? null : "like")} bookmark={bookmark} onCount={(n) => setP((current) => ({ ...current, comments_count: n }))} error={error} />}
    </article>
  );
}

function ReactionUsers({ postId, count, custom = [], panel = false, comment = false }: { postId: number; count: number; custom?: Emoji[]; panel?: boolean; comment?: boolean }) {
  const endpoint=`/${comment?"comments":"posts"}/${postId}/reactions`;
  const dialog = useRef<HTMLDialogElement>(null);
  const [items, setItems] = useState<{ user: UserCard; reaction: string }[]>([]);
  const [cursor, setCursor] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [preview,setPreview] = useState<UserCard[]>([]);
  const requested = useRef(false);
  useEffect(()=>{requested.current=false;setPreview([]);},[postId,count]);
  const previewUsers=()=>{if(requested.current||!count)return;requested.current=true;get<Page<{user:UserCard}>>(`${endpoint}?limit=3`).then(r=>setPreview(r.items.map(x=>x.user)),()=>{requested.current=false;});};
  useEffect(()=>{if(panel)previewUsers();},[panel,postId,count]);
  const openList=()=>{setItems([]);setCursor(null);dialog.current?.showModal();void load();};
  const load = async (after = 0) => {
    setBusy(true); setError(null);
    try {
      const result = await get<Page<{ user: UserCard; reaction: string }>>(`${endpoint}?limit=50&after=${after}`);
      setItems((old) => after ? [...old, ...result.items] : result.items);
      setCursor(result.next_cursor);
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  return <><span class={panel?"reaction-summary reaction-summary-panel":"reaction-summary"} onMouseEnter={previewUsers} onFocus={previewUsers}>{!panel&&<button class="chip reaction-count" title="Посмотреть реакции" aria-label={`Кто поставил реакцию: ${count}`} onClick={openList}>{count}</button>}<span class="reaction-preview"><span class="reaction-preview-avatars">{preview.map(user=><Avatar key={user.id} user={user} size={30}/>)}</span><button class="link" onClick={openList}>{count===0?"Пока нет реакций":`Реакции: ${count}`}</button></span></span>
    <dialog class="reaction-dialog" ref={dialog} aria-label={comment?"Реакции на комментарий":"Реакции на публикацию"} onClick={(e) => { if (e.target === e.currentTarget) dialog.current?.close(); }} onKeyDown={(e) => e.stopPropagation()}>
      <header><h2>{comment?"Оценили комментарий":"Оценили публикацию"}</h2><button class="icon-btn" aria-label="Закрыть список" onClick={() => dialog.current?.close()}>×</button></header>
      <ul>{items.map((item) => <li key={item.user.id}><Avatar user={item.user} /><UserLink user={item.user} /><ReactionIcon r={item.reaction} custom={custom} /></li>)}</ul>
      {!busy && !error && items.length === 0 && <p class="muted">Пока нет реакций</p>}
      <ErrorLine error={error} />
      {busy ? <Loading /> : error ? <button class="chip" onClick={() => load(cursor ?? 0)}>Повторить</button> : cursor !== null && <button class="chip" onClick={() => load(cursor)}>Показать ещё</button>}
    </dialog></>;
}

function SharePost({postId}:{postId:number}){
  const dialog=useRef<HTMLDialogElement>(null);const input=useRef<HTMLInputElement>(null);const [copied,setCopied]=useState(false);const [error,setError]=useState<unknown>(null);const url=new URL(`/p/${postId}`,location.origin).href;
  return <><button class="chip post-share-action" title="Поделиться" aria-label="Поделиться" onClick={()=>{setCopied(false);setError(null);dialog.current?.showModal();}}><svg class="nav-icon" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linejoin="round" aria-hidden="true"><path d="M14 3l8 7-8 7v-5C7 12 4 15 2 20c0-10 5-14 12-14z"/></svg></button><dialog class="music-dialog share-post-dialog" ref={dialog} aria-label="Поделиться записью"><h2>Поделиться записью</h2><p>Получатель сможет открыть запись, если ему разрешён доступ к ней.</p><label>Ссылка на запись<input ref={input} readOnly value={url} onFocus={e=>e.currentTarget.select()}/></label><ErrorLine error={error}/><p role="status">{copied?"Ссылка скопирована":""}</p><div class="library-actions"><button class="primary" onClick={async()=>{try{await navigator.clipboard.writeText(url);setCopied(true);setError(null);}catch{input.current?.focus();input.current?.select();setError(new Error("Выделите и скопируйте ссылку вручную."));}}}>Копировать ссылку</button><button class="ghost" onClick={()=>dialog.current?.close()}>Закрыть</button></div></dialog></>;
}

function PhotoViewer({ p, initialIndex, close, react, bookmark, onCount, error }: {
  p: Post; initialIndex: number; close: () => void; react: () => void;
  bookmark: () => void; onCount: (n: number) => void; error: unknown;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [index, setIndex] = useState(initialIndex);
  const move = (step: number) => setIndex((i) => (i + step + p.photos.length) % p.photos.length);
  useEffect(() => {
    const el = dialog.current!;
    const previous = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    el.showModal();
    return () => { el.close(); document.body.style.overflow = overflow; previous?.focus(); };
  }, []);
  return <dialog ref={dialog} class="photo-viewer" aria-label="Просмотр фотографий" onCancel={(e) => { e.preventDefault(); close(); }} onClick={(e) => { if (e.target === e.currentTarget) close(); }} onKeyDown={(e) => {
    if ((e.target as HTMLElement).closest("input, textarea, select, [contenteditable]")) return;
    if (e.key === "ArrowLeft") { e.preventDefault(); move(-1); }
    if (e.key === "ArrowRight") { e.preventDefault(); move(1); }
  }}>
    <div class="photo-viewer-layout">
      <button class="photo-close" aria-label="Закрыть просмотр" onClick={close} autoFocus>×</button>
      <div class="photo-stage">
        <div class="photo-image-wrap">
          <img key={p.photos[index].id} src={p.photos[index].url} alt={`Фотография ${index + 1} из ${p.photos.length}`} />
          {p.photos.length > 1 && <><button class="photo-prev" aria-label="Предыдущая фотография" onClick={() => move(-1)}>‹</button><button class="photo-next" aria-label="Следующая фотография" onClick={() => move(1)}>›</button></>}
        </div>
        <footer class="photo-toolbar"><span>{index + 1} / {p.photos.length}</span><a href={p.photos[index].url} target="_blank" rel="noopener" data-external>Открыть оригинал</a><a href={`/p/${p.id}`}>К публикации</a></footer>
      </div>
      <aside class="photo-details">
        <header class="photo-author"><Avatar user={p.author} /><div>{p.community ? <a href={`/c/${p.community.slug}`}>{p.community.name}</a> : <UserLink user={p.author} />}<div class="muted"><Time iso={p.created_at} /></div></div></header>
        <div class="photo-description"><div class="photo-actions"><button class="chip" aria-pressed={!!p.my_reaction} onClick={react} aria-label="Нравится"><Heart filled={!!p.my_reaction} /></button><ReactionUsers postId={p.id} count={p.likes_count} /><button class="chip" aria-pressed={p.bookmarked} onClick={bookmark}>{p.bookmarked ? "В закладках" : "В закладки"}</button></div>{p.text && <p class="post-text"><RichText text={p.text} /></p>}<ErrorLine error={error} /></div>
        <Comments post={p} onCount={onCount} />
      </aside>
    </div>
  </dialog>;
}

export function Video({ src, mp4, poster, title, onPlay }: { src: string; mp4?: string; poster: string; title?: string; onPlay?: () => void }) {
  // Браузер берёт первый источник, который умеет играть: HLS (Safari, iOS,
  // Android — с подстройкой качества под сеть) или MP4 (остальные).
  const hls = document.createElement("video").canPlayType("application/vnd.apple.mpegurl") !== "";
  return (
    <figure class="video">
      {hls || mp4 ? (
        <video onPlay={onPlay} controls preload="none" poster={poster} playsInline>
          {hls && <source src={src} type="application/vnd.apple.mpegurl" />}
          {mp4 && <source src={mp4} type="video/mp4" />}
        </video>
      ) : (
        <div class="video-fallback" style={{ backgroundImage: `url("${poster}")` }}>
          <p>Видео ещё готовится для этого браузера. Откройте его на телефоне или в Safari.</p>
        </div>
      )}
      {title && <figcaption>{title}</figcaption>}
    </figure>
  );
}

function Comments({ post: p, onCount }: { post: Post; onCount: (n: number) => void }) {
  const roots = useLoad(() => get<Page<Comment>>(`/posts/${p.id}/comments?limit=50`), [p.id]);
  const [replyTo, setReplyTo] = useState<Comment | null>(null);
  const [text, setText] = useState("");
  const [draft,setDraft]=useState(emptyDraft);
  const [sending,setSending]=useState(false);
  const [error, setError] = useState<unknown>(null);
  const [expanded, setExpanded] = useState<Record<number, Comment[]>>({});

  async function loadReplies(c: Comment) {
    const r = await get<Page<Comment>>(`/comments/${c.id}/replies?limit=50`);
    setExpanded((x) => ({ ...x, [c.id]: r.items }));
  }

  async function send(e: Event) {
    e.preventDefault();
    if(sending)return;
    setSending(true);
    setError(null);
    try {
      await post(`/posts/${p.id}/comments`, { text, photo_ids:draft.photos.map(x=>x.id),track_ids:draft.tracks.map(x=>x.id),video_ids:draft.videos.map(x=>x.id),document_ids:draft.documents.map(x=>x.id),sticker:draft.sticker, ...(replyTo ? { reply_to_id: replyTo.id } : {}) });
      setText("");
      setDraft(emptyDraft());
      const root = replyTo ? (replyTo.parent_id ?? replyTo.id) : null;
      setReplyTo(null);
      onCount(p.comments_count + 1);
      if (root) { const c = roots.data?.items.find((x) => x.id === root); if (c) await loadReplies(c); }
      roots.reload();
    } catch (err) { setError(err); } finally {setSending(false);}
  }

  async function remove(c: Comment) {
    await del(`/comments/${c.id}`);
    roots.reload();
    if (c.parent_id) setExpanded((x) => ({ ...x, [c.parent_id!]: (x[c.parent_id!] ?? []).filter((r) => r.id !== c.id) }));
  }

  const reactComment=async(c:Comment,r:string)=>{try{if(r===c.my_reaction)await del(`/comments/${c.id}/reactions`);else await post(`/comments/${c.id}/reactions`,{reaction:r});roots.reload();if(c.parent_id){const root=roots.data?.items.find(x=>x.id===c.parent_id);if(root)await loadReplies(root)}}catch(e){setError(e)}};
  const one = (c: Comment) => (
    <li key={c.id} class="comment" id={`comment-${c.id}`}>
      <Avatar user={c.author} size={28} />
      <div>
        <UserLink user={c.author} />{c.reply_to && <span class="muted"> ответ {c.reply_to.first_name}</span>}
        <p><RichText text={c.text} /></p>
        <CommentMedia c={c}/>
        <div class="comment-meta">
          <Time iso={c.created_at} />
          <button class="link" onClick={() => { setReplyTo(c); setText(`${c.author.first_name}, `); }}>Ответить</button>
          <button class="link" title="Скопировать ссылку на публикацию" onClick={()=>navigator.clipboard.writeText(`${location.origin}/p/${p.id}`).catch(setError)}>Поделиться</button>
          <button class="chip" aria-label="Нравится комментарий" aria-pressed={!!c.my_reaction} onClick={()=>reactComment(c,c.my_reaction||"like")}>{c.my_reaction?<ReactionIcon r={c.my_reaction} custom={[]}/>:<Heart/>}</button>
          <details class="comment-reaction-menu"><summary aria-label="Выбрать реакцию на комментарий">…</summary><div>{reactions.map(([id,emoji,label])=><button title={label} aria-label={label} onClick={e=>{void reactComment(c,id);e.currentTarget.closest("details")?.removeAttribute("open");}}>{emoji}</button>)}</div></details>
          <ReactionUsers postId={c.id} count={c.likes_count} comment/>
          {me()?.id === c.author.id && <button class="link" onClick={() => remove(c)}>Удалить</button>}
        </div>
        {me()?.id!==c.author.id&&<ReportButton target={{type:"comment",id:c.id}}/>}
        {!c.parent_id && c.replies_count > 0 && !expanded[c.id] && (
          <button class="link" onClick={() => loadReplies(c)}>Показать ответы: {c.replies_count}</button>
        )}
        {expanded[c.id] && <ul class="replies">{expanded[c.id].map(one)}</ul>}
      </div>
    </li>
  );

  return (
    <section class="comments" aria-label="Комментарии">
      {roots.error ? <ErrorLine error={roots.error} /> : !roots.data ? <Loading /> : <ul>{roots.data.items.map(one)}</ul>}
      {roots.data?.next_cursor&&<button class="link" onClick={async()=>{try{const old=roots.data!;const page=await get<Page<Comment>>(`/posts/${p.id}/comments?limit=50&after=${old.next_cursor}`);roots.set({...page,items:[...old.items,...page.items]});}catch(e){setError(e)}}}>Показать следующие комментарии</button>}
      <CommentExtras value={draft} onChange={setDraft} disabled={sending}/>
      <form class="comment-form" onSubmit={send}>
        {replyTo && <p class="muted">Ответ для {replyTo.author.first_name} <button type="button" class="link" onClick={() => setReplyTo(null)}>отменить</button></p>}
        <label class="sr-only" for={`c-${p.id}`}>Комментарий</label>
        <input id={`c-${p.id}`} value={text} maxLength={2000} placeholder="Написать комментарий" onInput={(e) => setText((e.target as HTMLInputElement).value)} />
        <button class="primary" disabled={sending||(!text.trim()&&!draft.sticker&&!draft.photos.length&&!draft.tracks.length&&!draft.videos.length&&!draft.documents.length)}>{sending?"Отправляем…":"Отправить"}</button>
      </form>
      <ErrorLine error={error} />
    </section>
  );
}

// Лента с подгрузкой по курсору.
export function PostList({ path, empty, header }: { path: string; empty: string; header?: (add: (p: Post) => void) => h.JSX.Element | null }) {
  const [items, setItems] = useState<Post[] | null>(null);
  const [cursor, setCursor] = useState<number | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [key, setKey] = useState(path);

  async function load(before?: number | null) {
    setBusy(true);
    try {
      const sep = path.includes("?") ? "&" : "?";
      const r = await get<Page<Post>>(path + (before ? `${sep}before=${before}` : ""));
      setItems((xs) => (before && xs ? [...xs, ...r.items] : r.items));
      setCursor(r.next_cursor);
    } catch (e) { setError(e); } finally { setBusy(false); }
  }
  if (key !== path) { setKey(path); setItems(null); }
  if (items === null && !busy && !error) load();

  const add = (p: Post) => setItems((xs) => [p, ...(xs ?? [])]);
  return (
    <div class="post-list">
      {header?.(add)}
      <ErrorLine error={error} />
      {items === null ? <Loading /> : items.length === 0 ? <Empty>{empty}</Empty> :
        items.map((p) => <PostView key={p.id} initial={p} onDelete={() => setItems(items.filter((x) => x.id !== p.id))} />)}
      {cursor && <button class="ghost more" disabled={busy} onClick={() => load(cursor)}>Показать ещё</button>}
    </div>
  );
}
