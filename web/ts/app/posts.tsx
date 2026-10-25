import { h } from "../vendor/preact/index.js";
import { useRef, useState } from "../vendor/preact/hooks.js";
import { Comment, Page, Photo, Post, Reaction, api, del, get, post, upload } from "./api.js";
import { me } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, RichText, Time, UserLink, duration, useLoad } from "./ui.js";

const reactions: [Reaction, string, string][] = [
  ["like", "👍", "Нравится"], ["love", "❤️", "Люблю"], ["haha", "😄", "Смешно"],
  ["wow", "😮", "Ого"], ["sad", "😢", "Грустно"], ["angry", "😠", "Возмутительно"],
];

export function Composer({ communityId, onPosted }: { communityId?: number; onPosted: (p: Post) => void }) {
  const [text, setText] = useState("");
  const [photos, setPhotos] = useState<Photo[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const file = useRef<HTMLInputElement>(null);

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

  async function submit(e: Event) {
    e.preventDefault();
    setBusy(true); setError(null);
    try {
      const p = await post<Post>("/posts", { text, photo_ids: photos.map((x) => x.id), ...(communityId ? { community_id: communityId } : {}) });
      setText(""); setPhotos([]);
      onPosted(p);
    } catch (err) { setError(err); } finally { setBusy(false); }
  }

  const empty = !text.trim() && photos.length === 0;
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
      <ErrorLine error={error} />
      <div class="composer-bar">
        <input ref={file} type="file" accept="image/jpeg,image/png,image/gif" multiple hidden onChange={(e) => addPhotos((e.target as HTMLInputElement).files)} />
        <button type="button" class="ghost" onClick={() => file.current?.click()} disabled={photos.length >= 10}>Фото</button>
        <button type="submit" class="primary" disabled={busy || empty}>{busy ? "Публикуем…" : "Опубликовать"}</button>
      </div>
    </form>
  );
}

export function PostView({ initial, onDelete }: { initial: Post; onDelete?: () => void }) {
  const [p, setP] = useState(initial);
  const [open, setOpen] = useState(false);
  const [picking, setPicking] = useState(false);
  const mine = me()?.id === p.author.id;

  async function react(r: Reaction | null) {
    setPicking(false);
    const res = r === null || r === p.my_reaction
      ? await api<{ likes_count: number }>("DELETE", `/posts/${p.id}/reactions`)
      : await post<{ likes_count: number }>(`/posts/${p.id}/reactions`, { reaction: r });
    setP(await get<Post>(`/posts/${p.id}`).catch(() => ({ ...p, likes_count: res.likes_count })));
  }

  async function remove() {
    if (!confirm("Удалить запись?")) return;
    await del(`/posts/${p.id}`);
    onDelete?.();
  }

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
      {p.videos.map((v) => <Video key={v.id} src={v.hls_url} poster={v.poster_url} title={v.title} />)}
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
      </footer>
      {open && <Comments post={p} onCount={(n) => setP({ ...p, comments_count: n })} />}
    </article>
  );
}

export function Video({ src, poster, title }: { src: string; poster: string; title?: string }) {
  // HLS нативно умеют Safari, iOS и большинство Android. Остальным — честно скажем.
  const native = document.createElement("video").canPlayType("application/vnd.apple.mpegurl") !== "";
  return (
    <figure class="video">
      {native ? <video controls preload="none" poster={poster} src={src} /> : (
        <div class="video-fallback" style={{ backgroundImage: `url("${poster}")` }}>
          <p>Этот браузер не воспроизводит HLS. Откройте видео в Safari или на телефоне.</p>
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
