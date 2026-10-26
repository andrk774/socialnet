import { h, Fragment } from "../vendor/preact/index.js";
import { useRef, useState } from "../vendor/preact/hooks.js";
import { Audience, Comment, Doc, Page, Photo, Poll, Post, PostDocument, Reaction, Scheduled, UserCard, api, del, fileSize, fullName, get, patch, post, put, saveFile, upload } from "./api.js";
import { me } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, RichText, Time, UserLink, duration, useLoad } from "./ui.js";

const reactions: [Reaction, string, string][] = [
  ["like", "👍", "Нравится"], ["love", "❤️", "Люблю"], ["haha", "😄", "Смешно"],
  ["wow", "😮", "Ого"], ["sad", "😢", "Грустно"], ["angry", "😠", "Возмутительно"],
];

const audienceLabel: Record<Audience, string> = { everyone: "Все", friends: "Друзья", only_me: "Только я", list: "Выбранные друзья" };

interface PollDraft { question: string; options: string[]; multiple: boolean; anonymous: boolean }

// localInput: значение для <input type="datetime-local"> в часовом поясе браузера.
const localInput = (d: Date) => new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);

export function Composer({ communityId, onPosted }: { communityId?: number; onPosted: (p: Post) => void }) {
  const [text, setText] = useState("");
  const [photos, setPhotos] = useState<Photo[]>([]);
  const [docs, setDocs] = useState<Doc[]>([]);
  const [poll, setPoll] = useState<PollDraft | null>(null);
  const [audience, setAudience] = useState<Audience>("everyone");
  const [picked, setPicked] = useState<number[]>([]);
  const [when, setWhen] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [note, setNote] = useState("");
  const file = useRef<HTMLInputElement>(null);
  const docFile = useRef<HTMLInputElement>(null);
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

  async function submit(e: Event) {
    e.preventDefault();
    setBusy(true); setError(null); setNote("");
    const body: Record<string, unknown> = { text, photo_ids: photos.map((x) => x.id), document_ids: docs.map((x) => x.id) };
    if (communityId) body.community_id = communityId;
    else { body.audience = audience; if (audience === "list") body.audience_ids = picked; }
    if (poll) body.poll = { ...poll, options: poll.options.filter((o) => o.trim()) };
    if (when) body.publish_at = new Date(when).toISOString();
    try {
      const r = await post<Post | Scheduled>("/posts", body);
      setText(""); setPhotos([]); setDocs([]); setPoll(null); setWhen(null);
      if (when) setNote(`Запись выйдет ${new Date((r as Scheduled).publish_at).toLocaleString("ru", { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" })}. Её можно перенести в разделе «Отложенные».`);
      else onPosted(r as Post);
    } catch (err) { setError(err); } finally { setBusy(false); }
  }

  const setOpt = (i: number, v: string) => setPoll(poll && { ...poll, options: poll.options.map((o, j) => (j === i ? v : o)) });
  const empty = !text.trim() && photos.length === 0 && docs.length === 0 && !poll;
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
          <button type="button" class="ghost" onClick={() => file.current?.click()} disabled={photos.length >= 10}>Фото</button>
          <button type="button" class="ghost" onClick={() => docFile.current?.click()} disabled={docs.length >= 10}>Документ</button>
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
  const mine = me()?.id === p.author.id;

  const [error, setError] = useState<unknown>(null);
  // любое действие с записью: ошибка показывается под записью, а состояние
  // меняется только после ответа сервера — интерфейс не врёт о несохранённом
  const act = async (fn: () => Promise<void>) => {
    setError(null);
    try { await fn(); } catch (e) { setError(e); }
  };

  const react = (r: Reaction | null) => act(async () => {
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

  const top = (Object.entries(p.reactions) as [Reaction, number][]).filter(([, n]) => n > 0).sort((a, b) => b[1] - a[1]).slice(0, 3);
  const mineR = reactions.find(([r]) => r === p.my_reaction);
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
          {p.photos.map((ph) => <a key={ph.id} href={ph.url} target="_blank" rel="noopener" data-external><img src={ph.thumb_url} alt="" loading="lazy" /></a>)}
        </div>
      )}
      {p.tracks.map((t) => (
        <div class="track" key={t.id}><span>{t.artist} — {t.title}</span><span class="muted">{duration(t.duration_sec)}</span><audio controls preload="none" src={t.url} /></div>
      ))}
      {p.videos.map((v) => <Video key={v.id} src={v.hls_url} mp4={v.mp4_url} poster={v.poster_url} title={v.title} />)}
      {p.documents.length > 0 && <DocList docs={p.documents} />}
      {p.poll && <PollView postId={p.id} initial={p.poll} />}
      <footer class="post-actions">
        <div class="react-wrap">
          <button class={p.my_reaction ? "chip active" : "chip"} aria-pressed={!!p.my_reaction}
            onClick={() => (p.my_reaction ? react(null) : react("like"))}
            onContextMenu={(e) => { e.preventDefault(); setPicking(!picking); }}>
            {mineR ? mineR[1] : "👍"} {p.likes_count > 0 ? p.likes_count : ""}
            <span class="sr-only">{mineR ? `Ваша реакция: ${mineR[2]}. Убрать` : "Нравится"}</span>
          </button>
          <button class="chip" aria-expanded={picking} aria-label="Выбрать реакцию" onClick={() => setPicking(!picking)}>…</button>
          {picking && (
            <div class="react-picker" role="menu">
              {reactions.map(([r, emoji, label]) => <button role="menuitem" key={r} title={label} aria-label={label} onClick={() => react(r)}>{emoji}</button>)}
            </div>
          )}
          {top.length > 1 && <span class="muted react-top">{top.map(([r]) => reactions.find((x) => x[0] === r)![1]).join("")}</span>}
        </div>
        <button class="chip" aria-expanded={open} onClick={() => setOpen(!open)}>
          Комментарии{p.comments_count > 0 ? ` ${p.comments_count}` : ""}
        </button>
        <button class={p.bookmarked ? "chip active" : "chip"} aria-pressed={p.bookmarked} onClick={bookmark}>
          {p.bookmarked ? "В закладках" : "В закладки"}
        </button>
        {mine && !p.community && (
          <select class="chip-select" aria-label="Кто видит запись" value={p.audience} onChange={(e) => { const el = e.target as HTMLSelectElement; changeAudience(el.value as Audience, el); }}>
            {(Object.keys(audienceLabel) as Audience[]).filter((a) => a !== "list" || p.audience === "list").map((a) => <option key={a} value={a}>{audienceLabel[a]}</option>)}
          </select>
        )}
      </footer>
      <ErrorLine error={error} />
      {open && <Comments post={p} onCount={(n) => setP({ ...p, comments_count: n })} />}
    </article>
  );
}

export function Video({ src, mp4, poster, title }: { src: string; mp4?: string; poster: string; title?: string }) {
  // Браузер берёт первый источник, который умеет играть: HLS (Safari, iOS,
  // Android — с подстройкой качества под сеть) или MP4 (остальные).
  const hls = document.createElement("video").canPlayType("application/vnd.apple.mpegurl") !== "";
  return (
    <figure class="video">
      {hls || mp4 ? (
        <video controls preload="none" poster={poster} playsInline>
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
  const [error, setError] = useState<unknown>(null);
  const [expanded, setExpanded] = useState<Record<number, Comment[]>>({});

  async function loadReplies(c: Comment) {
    const r = await get<Page<Comment>>(`/comments/${c.id}/replies?limit=50`);
    setExpanded((x) => ({ ...x, [c.id]: r.items }));
  }

  async function send(e: Event) {
    e.preventDefault();
    setError(null);
    try {
      await post(`/posts/${p.id}/comments`, { text, ...(replyTo ? { reply_to_id: replyTo.id } : {}) });
      setText("");
      const root = replyTo ? (replyTo.parent_id ?? replyTo.id) : null;
      setReplyTo(null);
      onCount(p.comments_count + 1);
      if (root) { const c = roots.data?.items.find((x) => x.id === root); if (c) await loadReplies(c); }
      roots.reload();
    } catch (err) { setError(err); }
  }

  async function remove(c: Comment) {
    await del(`/comments/${c.id}`);
    roots.reload();
    if (c.parent_id) setExpanded((x) => ({ ...x, [c.parent_id!]: (x[c.parent_id!] ?? []).filter((r) => r.id !== c.id) }));
  }

  const one = (c: Comment) => (
    <li key={c.id} class="comment">
      <Avatar user={c.author} size={28} />
      <div>
        <UserLink user={c.author} />{c.reply_to && <span class="muted"> ответ {c.reply_to.first_name}</span>}
        <p><RichText text={c.text} /></p>
        <div class="comment-meta">
          <Time iso={c.created_at} />
          <button class="link" onClick={() => { setReplyTo(c); setText(`${c.author.first_name}, `); }}>Ответить</button>
          {me()?.id === c.author.id && <button class="link" onClick={() => remove(c)}>Удалить</button>}
        </div>
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
      <form class="comment-form" onSubmit={send}>
        {replyTo && <p class="muted">Ответ для {replyTo.author.first_name} <button type="button" class="link" onClick={() => setReplyTo(null)}>отменить</button></p>}
        <label class="sr-only" for={`c-${p.id}`}>Комментарий</label>
        <input id={`c-${p.id}`} value={text} maxLength={2000} placeholder="Написать комментарий" onInput={(e) => setText((e.target as HTMLInputElement).value)} />
        <button class="primary" disabled={!text.trim()}>Отправить</button>
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
