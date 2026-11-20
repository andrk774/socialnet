import { ProfileMedia } from "./profilemedia.js";
import { ProfileDetails, ProfileActions } from "./profiledetails.js";
import { ProfileCover } from "./profilecover.js";
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { UserCard, api, fullName, get, post } from "./api.js";
import { PostList, Composer } from "./posts.js";
import { navigate } from "./router.js";
import { Avatar, ErrorLine, Loading, useLoad } from "./ui.js";
import { ReportButton } from "./report.js";
import { OrganizerReviews } from "./reviews.js";
import { AvatarUpload } from "./uploads.js";
import { startCall } from "./calls.js";

interface Profile extends UserCard {
  closed: boolean; plus?: boolean; status_text?: string; city?: string; about?: string;
  relationship: { status: "self" | "friends" | "request_sent" | "request_received" | "none"; i_blocked: boolean; can_write: boolean };
  presence?: { online?: boolean; last_seen_approx?: string };
}

const friendLabel: Record<string, string> = {
  none: "Добавить в друзья", request_sent: "Отменить заявку", request_received: "Принять заявку", friends: "Удалить из друзей",
};

export function ProfilePage({ username }: { username: string }) {
  const prof = useLoad(() => get<Profile>(`/users/by-username/${encodeURIComponent(username)}`), [username]);
  const [error, setError] = useState<unknown>(null);
  if (prof.error) return <ErrorLine error={prof.error} />;
  const u = prof.data;
  if (!u) return <Loading />;
  const rel = u.relationship.status;

  async function friend() {
    setError(null);
    try {
      if (rel === "none" || rel === "request_received") await post(`/users/${u!.id}/friend`);
      else await api("DELETE", `/users/${u!.id}/friend`);
      prof.reload();
    } catch (e) { setError(e); }
  }

  return (
    <div class="profile">
      <ProfileCover id={u.id} self={rel === "self"}/><header class="profile-head profile-head-covered">
        {rel === "self" ? <AvatarUpload user={u} onSaved={prof.reload} /> : <Avatar user={u} size={144} />}
        <div>
          <h1>{fullName(u)}{u.plus && <span class="plus-badge" title="Подписка «Плюс»">Плюс</span>}</h1>
          <p class="muted">@{u.username}{u.presence?.online ? " · в сети" : ""}</p>
          {u.status_text && <p class="status">{u.status_text}</p>}
          {u.city && <p class="muted">{u.city}</p>}<ProfileDetails user={u}/>
        </div>
        {rel === "self" && <ProfileActions user={u} reload={prof.reload}/>}
        {rel !== "self" && (
          <div class="profile-actions">
            <button class={rel === "none" || rel === "request_received" ? "primary" : "ghost"} onClick={friend}>{friendLabel[rel]}</button>
            {u.relationship.can_write && <button class="ghost" onClick={() => navigate(`/im/u/${u.id}`)}>Написать</button>}
            {u.relationship.can_write && <button class="ghost" onClick={() => startCall(u)}>Позвонить</button>}
            <ReportButton target={{ type: "user", id: u.id }} cls="ghost" />
          </div>
        )}
      </header>
      <ErrorLine error={error} />
      {u.about && <p class="about">{u.about}</p>}
      {rel === "self" && <ProfileMedia id={u.id}/>}
      <OrganizerReviews userId={u.id} />
      {u.closed ? <p class="empty">Профиль закрыт. Записи видят только друзья.</p> : (
        <PostList path={`/users/${u.id}/posts`} empty={rel === "self" ? "Здесь появятся ваши записи." : "Записей пока нет."}
          header={(add) => (rel === "self" ? <Composer onPosted={add} /> : null)} />
      )}
    </div>
  );
}
