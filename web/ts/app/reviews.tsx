// Отзывы: об организаторе (после похода) и о маршруте каталога.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { UserCard, del, get, post } from "./api.js";
import { ReportButton } from "./report.js";
import { ErrorLine, Time, UserLink, useLoad } from "./ui.js";
import { me } from "./session.js";

export interface Review {
  id: number; author: UserCard; kind: "organizer" | "route"; event_id?: number; event_title?: string; route_id?: string;
  rating: number; text: string; created_at: string; updated_at: string;
}
export interface Rating { avg: number; count: number }

const word = (n: number) => (n % 10 === 1 && n % 100 !== 11 ? "отзыв" : n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 10 || n % 100 >= 20) ? "отзыва" : "отзывов");

export function Stars({ value }: { value: number }) {
  const full = Math.round(value);
  return <span class="stars" aria-label={`${value.toFixed(1)} из 5`}>{"★".repeat(full)}<span class="stars-off">{"★".repeat(5 - full)}</span></span>;
}

export function RatingLine({ r }: { r: Rating }) {
  if (r.count === 0) return <span class="muted">отзывов пока нет</span>;
  return <span class="rating-line"><Stars value={r.avg} /> {r.avg.toFixed(1)} · {r.count} {word(r.count)}</span>;
}

// ReviewForm — оценка звёздами и текст; повторная отправка исправляет отзыв.
export function ReviewForm({ path, initial, label, onSaved }: { path: string; initial?: Review | null; label: string; onSaved: () => void }) {
  const [rating, setRating] = useState(initial?.rating ?? 0);
  const [text, setText] = useState(initial?.text ?? "");
  const [done, setDone] = useState(false);
  const [error, setError] = useState<unknown>(null);
  async function save(e: Event) {
    e.preventDefault();
    setError(null);
    try { await post(path, { rating, text }); setDone(true); onSaved(); } catch (err) { setError(err); }
  }
  async function remove() {
    setError(null);
    try { await del(path); setRating(0); setText(""); onSaved(); } catch (err) { setError(err); }
  }
  return (
    <form class="review-form stack" onSubmit={save}>
      <fieldset class="star-input">
        <legend>{label}</legend>
        {[1, 2, 3, 4, 5].map((n) => (
          <label key={n} class={n <= rating ? "on" : ""}>
            <input type="radio" name="rating" value={n} checked={rating === n} onChange={() => setRating(n)} />
            <span aria-hidden="true">★</span><span class="sr-only">{n} из 5</span>
          </label>
        ))}
      </fieldset>
      <label>Отзыв<textarea rows={3} maxLength={2000} value={text} placeholder="Что понравилось, что стоит знать другим" onInput={(e) => setText((e.target as HTMLTextAreaElement).value)} /></label>
      <ErrorLine error={error} />
      <div class="row">
        <button class="primary" disabled={rating === 0}>{initial ? "Исправить отзыв" : "Оставить отзыв"}</button>
        {initial && <button type="button" class="ghost danger" onClick={remove}>Удалить</button>}
        {done && <span class="muted" role="status">Сохранено</span>}
      </div>
    </form>
  );
}

export function ReviewList({ items, showEvent = false }: { items: Review[]; showEvent?: boolean }) {
  const self = me()?.id;
  return (
    <ul class="reviews">
      {items.map((r) => (
        <li key={r.id}>
          <div class="row tight"><Stars value={r.rating} /> <UserLink user={r.author} /> <span class="muted"><Time iso={r.created_at} /></span></div>
          {showEvent && r.event_id && <p class="muted">поход: <a href={`/events/${r.event_id}`}>{r.event_title}</a></p>}
          {r.text && <p>{r.text}</p>}
          {r.author.id !== self && <ReportButton target={{ type: "review", id: r.id }} />}
        </li>
      ))}
    </ul>
  );
}

// OrganizerReviews — на странице профиля: оценка как организатора и отзывы.
export function OrganizerReviews({ userId }: { userId: number }) {
  const res = useLoad(() => get<{ rating: Rating; items: Review[] }>(`/users/${userId}/reviews?limit=20`), [userId]);
  if (!res.data || res.data.rating.count === 0) return null;
  return (
    <details class="organizer-reviews">
      <summary>Организатор походов: <RatingLine r={res.data.rating} /></summary>
      <ReviewList items={res.data.items} showEvent />
    </details>
  );
}

// RouteReviews — под маршрутом каталога: оценка, отзывы и свой отзыв.
export function RouteReviews({ routeId, ownerId }: { routeId: string; ownerId: number }) {
  const res = useLoad(() => get<{ rating: Rating; items: Review[] }>(`/routes/${routeId}/reviews?limit=50`), [routeId]);
  const self = me()?.id;
  if (!res.data) return null;
  const mine = res.data.items.find((r) => r.author.id === self) ?? null;
  return (
    <section class="route-reviews">
      <h3>Отзывы о маршруте</h3>
      <p><RatingLine r={res.data.rating} /></p>
      <ReviewList items={res.data.items} />
      {self !== ownerId && <ReviewForm path={`/routes/${routeId}/reviews`} initial={mine} label="Прошли маршрут? Оцените" onSaved={res.reload} />}
    </section>
  );
}

