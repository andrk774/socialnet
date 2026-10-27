// /search?q= — общий поиск: походы, маршруты каталога, статьи, люди,
// сообщества, музыка. «Всё» — по несколько из каждого раздела.
import { h, Fragment, ComponentChildren } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { UserCard, fullName, get } from "./api.js";
import { query } from "./router.js";
import { Avatar, Empty, ErrorLine, Loading, useLoad } from "./ui.js";

interface Hit { id: number | string; [k: string]: any }
type Kind = "hikes" | "routes" | "articles" | "users" | "communities" | "tracks";
const tabs: [Kind | "all", string][] = [["all", "Всё"], ["hikes", "Походы"], ["routes", "Маршруты"], ["articles", "Статьи"],
  ["users", "Люди"], ["communities", "Сообщества"], ["tracks", "Музыка"]];
const sections: Kind[] = ["hikes", "routes", "articles", "users", "communities"];

const dateFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long", weekday: "short" });
const km = (m: number) => `${(m / 1000).toFixed(m < 10000 ? 1 : 0)} км`;

function Item({ kind, x }: { kind: Kind; x: Hit }): ComponentChildren {
  switch (kind) {
    case "hikes": return <li><a href={`/events/${x.id}`}>{x.title}</a> <span class="muted">{dateFmt.format(new Date(x.starts_at))} · идут {x.going_count}{x.capacity ? ` из ${x.capacity}` : ""}</span></li>;
    case "routes": return <li><a href={x.url} target="_blank" rel="noopener">{x.title}</a> <span class="muted">{km(x.distance_m)}, набор {x.gain_m} м · {fullName(x.author as UserCard)}</span></li>;
    case "articles": return <li><a href={`/articles/${x.id}`}>{x.title}</a> <span class="muted">{fullName(x.author as UserCard)}</span></li>;
    case "users": return <li><Avatar user={x as unknown as UserCard} size={32} /> <a href={`/u/${x.username}`}>{fullName(x as unknown as UserCard)}</a> <span class="muted">@{x.username}</span></li>;
    case "communities": return <li><a href={`/c/${x.slug}`}>{x.name}</a> <span class="muted">{x.members_count} уч.</span></li>;
    default: return <li>{x.artist} — {x.title} <audio controls preload="none" src={x.url} /></li>;
  }
}

export function SearchPage() {
  const q = query("q") ?? "";
  const [type, setType] = useState<Kind | "all">("all");
  const res = useLoad(() => (q.trim().length < 2 ? Promise.resolve(null) :
    get<Record<string, Hit[]>>(`/search?q=${encodeURIComponent(q)}&type=${type}`)), [q, type]);
  const d = res.data;
  return (
    <section class="search-page">
      <h1>Поиск{q && `: ${q}`}</h1>
      <div class="tabs" role="tablist">
        {tabs.map(([k, label]) => <button key={k} role="tab" aria-selected={type === k} class={type === k ? "tab on" : "tab"} onClick={() => setType(k)}>{label}</button>)}
      </div>
      {q.trim().length < 2 ? <Empty>Введите в строке поиска хотя бы две буквы: название похода, вершину, район, имя.</Empty> :
        res.error ? <ErrorLine error={res.error} /> : !d ? <Loading /> : type !== "all" ? (
          d.items.length === 0 ? <Empty>Ничего не нашлось. Попробуйте другое написание.</Empty> :
            <ul class="plain-list">{d.items.map((x) => <Item key={x.id} kind={type} x={x} />)}</ul>
        ) : sections.every((k) => !d[k]?.length) ? <Empty>Ничего не нашлось. Попробуйте другое написание.</Empty> : (
          <>{sections.filter((k) => d[k]?.length).map((k) => (
            <section key={k} class="search-section" data-kind={k}>
              <h2>{tabs.find(([t]) => t === k)![1]}</h2>
              <ul class="plain-list">{d[k].map((x) => <Item key={x.id} kind={k} x={x} />)}</ul>
              {d[k].length >= 5 && <button class="link" onClick={() => setType(k)}>Показать все</button>}
            </section>
          ))}</>
        )}
    </section>
  );
}
