// Клиент API. Access-токен живёт только в памяти вкладки (не в localStorage —
// его не украдёт XSS из хранилища), refresh-токен — в HttpOnly-cookie.

export interface UserCard { id: number; username: string; first_name: string; last_name: string; avatar_url: string | null }
export interface Self extends UserCard {
  email: string; email_verified: boolean; status_text: string; city: string; about: string;
  role: "user" | "moderator" | "admin"; two_factor_enabled: boolean;
}
export interface Photo { id: string; url: string; thumb_url: string }
export interface PostTrack { id: string; title: string; artist: string; duration_sec: number; url: string }
export interface PostVideo { id: string; title: string; duration_sec: number; poster_url: string; hls_url: string; mp4_url?: string }
export type Reaction = "like" | "love" | "haha" | "wow" | "sad" | "angry";
export type Audience = "everyone" | "friends" | "only_me" | "list";
export interface PostDocument { id: string; title: string; ext: string; size: number; url: string }
export interface Doc extends PostDocument { owner_id: number; created_at: string }
export interface PollOption { id: number; text: string; votes: number | null }
export interface Poll {
  id: number; question: string; multiple: boolean; anonymous: boolean; closes_at: string | null; closed: boolean;
  voters_count: number; results_hidden: boolean; my_votes: number[]; options: PollOption[];
}
export interface Post {
  id: number; author: UserCard; community?: { id: number; slug: string; name: string }; text: string;
  photos: Photo[]; tracks: PostTrack[]; videos: PostVideo[]; documents: PostDocument[]; poll: Poll | null;
  audience: Audience; bookmarked: boolean; likes_count: number; comments_count: number;
  my_reaction: Reaction | null; reactions: Partial<Record<Reaction, number>>; created_at: string;
}
export interface Scheduled {
  id: number; author_id: number; community_id?: number; publish_at: string; status: "pending" | "published" | "failed" | "canceled";
  error?: string; payload: { text?: string; poll?: { question: string }; photo_ids?: string[]; document_ids?: string[] }; created_at: string;
}
export interface Comment {
  id: number; post_id: number; parent_id?: number; reply_to?: UserCard; replies_count: number;
  author: UserCard; text: string; created_at: string;
}
export interface Attachment {
  kind: "photo" | "track" | "voice" | "document"; id: string; url: string; thumb_url?: string; title?: string;
  artist?: string; duration_sec?: number; ext?: string; size?: number;
}
export interface Message { id: number; conversation_id: number; sender_id: number; kind: "text" | "service"; body?: string; encrypted: boolean; attachments: Attachment[]; created_at: string }
export interface Conversation { id: number; kind: "direct" | "group"; title?: string; members_count?: number; peer?: UserCard; last_message: Message; unread: number }
export interface Page<T> { items: T[]; next_cursor: number | null }
export interface ApiErrorBody { error: { code: string; message: string; fields?: Record<string, string> } }

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string, public fields: Record<string, string> = {}) {
    super(message);
  }
}

let token: string | null = null;
let refreshing: Promise<boolean> | null = null;
const listeners = new Set<(signedIn: boolean) => void>();

export const onAuthChange = (fn: (signedIn: boolean) => void) => { listeners.add(fn); return () => listeners.delete(fn); };
export const signedIn = () => token !== null;

function setToken(t: string | null) {
  const was = token !== null;
  token = t;
  if (was !== (t !== null)) listeners.forEach((fn) => fn(t !== null));
}

async function parse(res: Response): Promise<unknown> {
  // тело читаем всегда, даже пустое: иначе Chromium считает запрос незавершённым
  // и обрывает его при переходе на другую страницу
  if (res.status === 204) { await res.arrayBuffer().catch(() => null); return null; }
  const type = res.headers.get("Content-Type") ?? "";
  return type.includes("application/json") ? res.json() : res.text();
}

// refresh — один на все параллельные запросы, иначе ротация токена
// сочтёт второй запрос повторным использованием и завершит сеанс.
export function refresh(): Promise<boolean> {
  refreshing ??= (async () => {
    try {
      const res = await fetch("/api/v1/auth/refresh", { method: "POST", credentials: "same-origin" });
      if (!res.ok) { setToken(null); return false; }
      const body = (await res.json()) as { access_token: string };
      setToken(body.access_token);
      return true;
    } catch {
      return false;
    } finally {
      refreshing = null;
    }
  })();
  return refreshing;
}

export async function api<T = unknown>(method: string, path: string, body?: unknown, retry = true): Promise<T> {
  const headers: Record<string, string> = {};
  let payload: BodyInit | undefined;
  if (body instanceof FormData) payload = body;
  else if (body !== undefined) { headers["Content-Type"] = "application/json"; payload = JSON.stringify(body); }
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch("/api/v1" + path, { method, headers, body: payload, credentials: "same-origin" });
  if (res.status === 401 && retry && !path.startsWith("/auth/")) {
    if (await refresh()) return api<T>(method, path, body, false);
  }
  const data = await parse(res);
  if (!res.ok) {
    const e = (data as ApiErrorBody | null)?.error;
    throw new ApiError(res.status, e?.code ?? "http_" + res.status, e?.message ?? res.statusText, e?.fields ?? {});
  }
  return data as T;
}

export const get = <T>(path: string) => api<T>("GET", path);
export const post = <T = unknown>(path: string, body?: unknown) => api<T>("POST", path, body);
export const del = <T = unknown>(path: string) => api<T>("DELETE", path);
export const patch = <T = unknown>(path: string, body?: unknown) => api<T>("PATCH", path, body);
export const put = <T = unknown>(path: string, body?: unknown) => api<T>("PUT", path, body);

export async function login(email: string, password: string): Promise<{ mfa_token?: string }> {
  const r = await api<{ access_token?: string; mfa_required?: boolean; mfa_token?: string }>("POST", "/auth/login", { email, password });
  if (r.mfa_required) return { mfa_token: r.mfa_token };
  setToken(r.access_token!);
  return {};
}

export async function verify2FA(mfa_token: string, code: string) {
  const backup = code.includes("-") || code.length > 8;
  const r = await api<{ access_token: string }>("POST", "/auth/2fa/verify", backup ? { mfa_token, backup_code: code } : { mfa_token, code });
  setToken(r.access_token);
}

export async function register(fields: { email: string; username: string; password: string; first_name: string; last_name: string }) {
  const r = await api<{ access_token: string }>("POST", "/auth/register", fields);
  setToken(r.access_token);
}

export async function logout() {
  try { await api("POST", "/auth/logout"); } finally { setToken(null); }
}

export async function upload<T>(path: string, file: File, fields: Record<string, string> = {}): Promise<T> {
  const fd = new FormData();
  for (const [k, v] of Object.entries(fields)) fd.append(k, v);
  fd.append("file", file);
  return api<T>("POST", path, fd);
}

export const fullName = (u: UserCard) => [u.first_name, u.last_name].filter(Boolean).join(" ") || u.username;

// fetchBlob — бинарный ответ с авторизацией (голосовые сообщения).
export async function fetchBlob(path: string, retry = true): Promise<Blob> {
  const res = await fetch(path, { headers: token ? { Authorization: `Bearer ${token}` } : {}, credentials: "same-origin" });
  if (res.status === 401 && retry && (await refresh())) return fetchBlob(path, false);
  if (!res.ok) throw new ApiError(res.status, "http_" + res.status, res.statusText);
  return res.blob();
}

// saveFile — скачать документ: он закрыт токеном, поэтому не простой ссылкой.
export async function saveFile(path: string, name: string) {
  const url = URL.createObjectURL(await fetchBlob(path));
  const a = document.createElement("a");
  a.href = url; a.download = name;
  document.body.appendChild(a); a.click();
  setTimeout(() => a.remove(), 0);
  setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

export function fileSize(n: number): string {
  if (n < 1024) return `${n} Б`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} КБ`;
  return `${(n / 1024 / 1024).toFixed(1).replace(".", ",")} МБ`;
}
