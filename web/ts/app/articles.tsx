// Статьи — длинные отчёты о походах. Сервер присылает текст деревом блоков;
// здесь каждый блок рисуется компонентом, HTML из текста не вставляется.
import { Fragment, h } from "../vendor/preact/index.js";
import { useRef, useState } from "../vendor/preact/hooks.js";
import { del, get, Page, patch, post, Route, upload, UserCard } from "./api.js";
import { navigate } from "./router.js";
import { RouteCard } from "./routes.js";
import { useMe } from "./session.js";
import { Avatar, Empty, ErrorLine, Loading, UserLink, useLoad } from "./ui.js";

interface Span { t: "text" | "b" | "i" | "a"; text: string; href?: string }
interface Block { kind: "h2" | "h3" | "p" | "ul" | "ol" | "quote" | "hr" | "img"; spans?: Span[]; items?: Span[][]; media_id?: string; caption?: string }
interface Article {
  id: number; author: UserCard; title: string; body?: string; cover_url: string | null; cover_thumb_url: string | null;
  cover_media_id: string | null; route_id: string | null; status: "draft" | "published"; excerpt: string;
  created_at: string; updated_at: string; published_at: string | null; blocks?: Block[];
}

const dateFmt = new Intl.DateTimeFormat("ru", { day: "numeric", month: "long", year: "numeric" });

function Spans({ spans }: { spans?: Span[] }) {
  return <>{(spans ?? []).map((s, i) =>
    s.t === "b" ? <strong key={i}>{s.text}</strong>
      : s.t === "i" ? <em key={i}>{s.text}</em>
        : s.t === "a" ? <a key={i} href={s.href} target="_blank" rel="noopener nofollow ugc">{s.text}</a>
          : <Fragment key={i}>{s.text}</Fragment>)}</>;
}

export function ArticleBody({ blocks }: { blocks: Block[] }) {
  return (
    <div class="article-body">
      {blocks.map((b, i) => {
        switch (b.kind) {
          case "h2": return <h2 key={i}><Spans spans={b.spans} /></h2>;
          case "h3": return <h3 key={i}><Spans spans={b.spans} /></h3>;
          case "ul": return <ul key={i}>{b.items!.map((it, j) => <li key={j}><Spans spans={it} /></li>)}</ul>;
          case "ol": return <ol key={i}>{b.items!.map((it, j) => <li key={j}><Spans spans={it} /></li>)}</ol>;
          case "quote": return <blockquote key={i}><Spans spans={b.spans} /></blockquote>;
          case "hr": return <hr key={i} />;
          case "img": return (
            <figure key={i}>
              <img src={`/media/${b.media_id}/full`} alt={b.caption ?? ""} loading="lazy" />
              {b.caption && <figcaption>{b.caption}</figcaption>}
            </figure>
          );
          default: return <p key={i}><Spans spans={b.spans} /></p>;
        }
      })}
    </div>
  );
}

function Card({ a }: { a: Article }) {
  return (
    <li class="article-card">
      {a.cover_thumb_url && <a href={`/articles/${a.id}`}><img src={a.cover_thumb_url} alt="" width={120} height={90} loading="lazy" /></a>}
      <div class="grow">
        <a class="article-title" href={`/articles/${a.id}`}>{a.title}</a>
        {a.status === "draft" && <span class="pill">черновик</span>}
        <p class="muted">{a.author.first_name || a.author.username}{a.published_at ? ` · ${dateFmt.format(new Date(a.published_at))}` : ""}</p>
        {a.excerpt && <p class="article-excerpt">{a.excerpt}</p>}
      </div>
    </li>
  );
}

export function ArticlesPage() {
  const list = useLoad(() => get<Page<Article>>("/articles?limit=30"), []);
  const mine = useLoad(() => get<{ items: Article[] }>("/articles/mine"), []);
  const drafts = (mine.data?.items ?? []).filter((a) => a.status === "draft");
  return (
    <section>
      <div class="page-head">
        <h1>Статьи</h1>
        <a class="primary button-link" href="/articles/new">Написать отчёт</a>
      </div>
      {drafts.length > 0 && <><h2>Мои черновики</h2><ul class="article-list">{drafts.map((a) => <Card key={a.id} a={a} />)}</ul></>}
      {list.error ? <ErrorLine error={list.error} /> : !list.data ? <Loading /> : list.data.items.length === 0 ? <Empty>Статей пока нет. Расскажите о своём походе первым.</Empty> : (
        <ul class="article-list">{list.data.items.map((a) => <Card key={a.id} a={a} />)}</ul>
      )}
    </section>
  );
}

export function ArticlePage({ id }: { id: number }) {
  const me = useMe();
  const a = useLoad(() => get<Article>(`/articles/${id}`), [id]);
  const route = useLoad(() => (a.data?.route_id ? get<Route>(`/routes/${a.data.route_id}`) : Promise.resolve(null)), [a.data?.route_id]);
  const [error, setError] = useState<unknown>(null);
  if (a.error) return <ErrorLine error={a.error} />;
  if (!a.data) return <Loading />;
  const art = a.data, mine = art.author.id === me?.id;
  return (
    <article class="article">
      <p class="muted"><a href="/articles">← Статьи</a></p>
      {art.cover_url && <img class="article-cover" src={art.cover_url} alt="" />}
      <h1>{art.title}</h1>
      <p class="article-meta"><Avatar user={art.author} size={28} /> <UserLink user={art.author} />
        {art.published_at ? <span class="muted"> · {dateFmt.format(new Date(art.published_at))}</span> : <span class="pill">черновик</span>}</p>
      {mine && <div class="row">
        <a class="ghost button-link" href={`/articles/${art.id}/edit`}>Редактировать</a>
        <button class="ghost danger" onClick={() => confirm("Удалить статью?") && del(`/articles/${art.id}`).then(() => navigate("/articles"), setError)}>Удалить</button>
      </div>}
      <ErrorLine error={error} />
      <ArticleBody blocks={art.blocks ?? []} />
      {route.data && <section><h2>Маршрут</h2><RouteCard route={route.data} /></section>}
    </article>
  );
}

export function ArticleEditor({ id }: { id?: number }) {
  const existing = useLoad(() => (id ? get<Article>(`/articles/${id}`) : Promise.resolve(null)), [id]);
  if (id && !existing.data) return existing.error ? <ErrorLine error={existing.error} /> : <Loading />;
  return <Editor initial={existing.data ?? null} />;
}

function Editor({ initial }: { initial: Article | null }) {
  const routes = useLoad(() => get<Page<Route>>("/routes?limit=100"), []);
  const [title, setTitle] = useState(initial?.title ?? "");
  const [body, setBody] = useState(initial?.body ?? "");
  const [cover, setCover] = useState<string | null>(initial?.cover_media_id ?? null);
  const [routeId, setRouteId] = useState(initial?.route_id ?? "");
  const [preview, setPreview] = useState<Block[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const area = useRef<HTMLTextAreaElement>(null);

  const insertPhoto = async (file: File) => {
    setError(null);
    try {
      const m = await upload<{ id: string }>("/media", file);
      const el = area.current, at = el ? el.selectionStart : body.length;
      const line = `\n![${file.name.replace(/\.[^.]+$/, "").replace(/[\][]/g, "")}](media:${m.id})\n`;
      setBody(body.slice(0, at) + line + body.slice(at));
    } catch (e) { setError(e); }
  };
  const save = async (status: "draft" | "published") => {
    setBusy(true); setError(null);
    const payload = { title, body, cover_media_id: cover, route_id: routeId || null, status };
    try {
      const a = initial ? await patch<Article>(`/articles/${initial.id}`, payload) : await post<Article>("/articles", payload);
      navigate(`/articles/${a.id}`);
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  return (
    <section class="article-editor">
      <h1>{initial ? "Редактирование" : "Новый отчёт"}</h1>
      <label>Заголовок<input value={title} maxLength={150} required onInput={(e) => setTitle((e.target as HTMLInputElement).value)} placeholder="Три дня на Таганае" /></label>
      <div class="row">
        <label>Обложка<input type="file" accept="image/jpeg,image/png" onChange={async (e) => {
          const f = (e.target as HTMLInputElement).files?.[0];
          if (f) { try { setCover((await upload<{ id: string }>("/media", f)).id); } catch (err) { setError(err); } }
        }} /></label>
        {cover && <img src={`/media/${cover}/thumb`} alt="Обложка" width={96} height={72} class="cover-preview" />}
        {cover && <button class="link" onClick={() => setCover(null)}>Убрать обложку</button>}
      </div>
      <label>Маршрут
        <select value={routeId} onChange={(e) => setRouteId((e.target as HTMLSelectElement).value)}>
          <option value="">без маршрута</option>
          {routes.data?.items.map((r) => <option key={r.id} value={r.id}>{r.title}</option>)}
        </select>
      </label>
      <div class="editor-tabs row tight">
        <button class={preview ? "chip" : "chip active"} onClick={() => setPreview(null)}>Текст</button>
        <button class={preview ? "chip active" : "chip"} onClick={() => post<{ blocks: Block[] }>("/articles/preview", { body }).then((r) => setPreview(r.blocks), setError)}>Предпросмотр</button>
        <label class="chip file-chip">+ Фото в текст<input type="file" accept="image/jpeg,image/png" class="sr-only" onChange={(e) => { const f = (e.target as HTMLInputElement).files?.[0]; if (f) insertPhoto(f); }} /></label>
      </div>
      {preview ? <div class="preview-box"><ArticleBody blocks={preview} /></div> : (
        <label class="sr-only-label">Текст
          <textarea ref={area} rows={18} maxLength={100000} value={body} onInput={(e) => setBody((e.target as HTMLTextAreaElement).value)}
            placeholder={"## День первый\n\nВышли в **7 утра** от Белого ключа…\n\n- вода\n- палатка"} />
        </label>
      )}
      <p class="hint">Разметка: <code>## заголовок</code>, <code>### подзаголовок</code>, <code>- список</code>, <code>1. список</code>, <code>&gt; цитата</code>, <code>---</code>, <code>**жирный**</code>, <code>*курсив*</code>, <code>[ссылка](https://…)</code>.</p>
      <ErrorLine error={error} />
      <div class="row">
        <button class="primary" disabled={busy} onClick={() => save("published")}>{initial?.status === "published" ? "Сохранить" : "Опубликовать"}</button>
        <button class="ghost" disabled={busy} onClick={() => save("draft")}>{initial?.status === "published" ? "Снять в черновики" : "Сохранить черновик"}</button>
      </div>
    </section>
  );
}
