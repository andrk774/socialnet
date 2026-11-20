import { h, ComponentChildren } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { PostVideo, UserCard, fullName, get, post, upload } from "./api.js";
import { Avatar, ErrorLine, useLoad } from "./ui.js";
import { useMe } from "./session.js";
import { StoryViewer } from "./storyviewer.js";
import { Icon } from "./shellparts.js";

interface Story { id: number; caption: string; photo?: { url: string }; video?: PostVideo }
interface Group { author: UserCard; stories: Story[]; all_seen: boolean }
function StoryDialog({ children, title, close }: { children: ComponentChildren; title: string; close: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => { ref.current?.showModal(); }, []);
  return <dialog class="story-dialog" ref={ref} aria-label={title} onClose={close} onClick={(e) => { if (e.target === ref.current) close(); }}><div class="story-dialog-inner"><button class="story-close" aria-label="Закрыть историю" onClick={close}>×</button>{children}</div></dialog>;
}
export function Stories() {
  const me = useMe();
  const feed = useLoad(() => get<{ items: Group[] }>("/stories/feed"), []);
  const [selected, setSelected] = useState<{ group: Group; index: number } | null>(null);
  const [creating, setCreating] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [caption, setCaption] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const story = selected?.group.stories[selected.index];
  useEffect(() => { if (story) post(`/stories/${story.id}/view`).catch(setError); }, [story?.id]);
  const close = () => { setSelected(null); feed.reload(); };
  async function create() {
    if (!file || busy) return;
    setBusy(true); setError(null);
    try { const photo = await upload<{ id: string }>("/media", file); await post("/stories", { photo_id: photo.id, caption }); setCreating(false); setFile(null); setCaption(""); feed.reload(); }
    catch (e) { setError(e); } finally { setBusy(false); }
  }
  return <div class="stories-block">
    <div class="stories-strip" aria-label="Истории">
      <button class="story-card story-create" onClick={() => { setError(null); setCreating(true); }}>{me && <Avatar user={me} size={48} />}<span class="story-plus"><Icon name="plus" /></span><strong>Моя история</strong></button>
      {feed.data?.items.filter((g) => g.stories.length > 0).map((g) => <button key={g.author.id} class={`story-card ${g.all_seen ? "story-seen" : ""}`} aria-label={`Истории: ${fullName(g.author)}`} onClick={() => { setError(null); setSelected({ group: g, index: 0 }); }}>
        {(g.stories[0].photo?.url || g.stories[0].video?.poster_url) && <img class="story-cover" src={g.stories[0].photo?.url ?? g.stories[0].video?.poster_url} alt="" />}
        <span class="story-author"><Avatar user={g.author} size={36} /><strong>{fullName(g.author)}</strong></span>
      </button>)}
      {feed.data && !feed.data.items.length && <div class="stories-empty"><strong>Поделитесь моментом</strong><span>Фото исчезнет через 24 часа.<br />Здесь появятся истории ваших друзей.</span></div>}
    </div>
    {feed.error && <ErrorLine error={feed.error} />}
    {creating && <StoryDialog title="Новая история" close={() => { if (!busy) setCreating(false); }}><h2>Новая история</h2><p class="muted">Фото будет доступно 24 часа.</p><form onSubmit={(e) => { e.preventDefault(); void create(); }}><label>Фото для истории<input type="file" accept="image/jpeg,image/png,image/gif" required disabled={busy} onChange={(e) => setFile((e.target as HTMLInputElement).files?.[0] ?? null)} /></label><label>Подпись<input value={caption} maxLength={200} disabled={busy} onInput={(e) => setCaption((e.target as HTMLInputElement).value)} /></label><ErrorLine error={error} /><button class="primary" disabled={!file || busy}>{busy ? "Публикуем…" : "Опубликовать историю"}</button></form></StoryDialog>}
    {selected && story && <StoryViewer groups={feed.data?.items.filter(g=>g.stories.length>0)??[]} group={selected.group} index={selected.index} choose={(group,index)=>setSelected({group,index})} close={close}/>}
  </div>;
}
