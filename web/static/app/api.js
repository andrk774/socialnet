// Клиент API. Access-токен живёт только в памяти вкладки (не в localStorage —
// его не украдёт XSS из хранилища), refresh-токен — в HttpOnly-cookie.
export class ApiError extends Error {
    status;
    code;
    fields;
    constructor(status, code, message, fields = {}) {
        super(message);
        this.status = status;
        this.code = code;
        this.fields = fields;
    }
}
let token = null;
let refreshing = null;
const listeners = new Set();
export const onAuthChange = (fn) => { listeners.add(fn); return () => listeners.delete(fn); };
export const signedIn = () => token !== null;
function setToken(t) {
    const was = token !== null;
    token = t;
    if (was !== (t !== null))
        listeners.forEach((fn) => fn(t !== null));
}
async function parse(res) {
    if (res.status === 204)
        return null;
    const type = res.headers.get("Content-Type") ?? "";
    return type.includes("application/json") ? res.json() : res.text();
}
// refresh — один на все параллельные запросы, иначе ротация токена
// сочтёт второй запрос повторным использованием и завершит сеанс.
export function refresh() {
    refreshing ??= (async () => {
        try {
            const res = await fetch("/api/v1/auth/refresh", { method: "POST", credentials: "same-origin" });
            if (!res.ok) {
                setToken(null);
                return false;
            }
            const body = (await res.json());
            setToken(body.access_token);
            return true;
        }
        catch {
            return false;
        }
        finally {
            refreshing = null;
        }
    })();
    return refreshing;
}
export async function api(method, path, body, retry = true) {
    const headers = {};
    let payload;
    if (body instanceof FormData)
        payload = body;
    else if (body !== undefined) {
        headers["Content-Type"] = "application/json";
        payload = JSON.stringify(body);
    }
    if (token)
        headers["Authorization"] = `Bearer ${token}`;
    const res = await fetch("/api/v1" + path, { method, headers, body: payload, credentials: "same-origin" });
    if (res.status === 401 && retry && !path.startsWith("/auth/")) {
        if (await refresh())
            return api(method, path, body, false);
    }
    const data = await parse(res);
    if (!res.ok) {
        const e = data?.error;
        throw new ApiError(res.status, e?.code ?? "http_" + res.status, e?.message ?? res.statusText, e?.fields ?? {});
    }
    return data;
}
export const get = (path) => api("GET", path);
export const post = (path, body) => api("POST", path, body);
export const del = (path) => api("DELETE", path);
export async function login(email, password) {
    const r = await api("POST", "/auth/login", { email, password });
    if (r.mfa_required)
        return { mfa_token: r.mfa_token };
    setToken(r.access_token);
    return {};
}
export async function verify2FA(mfa_token, code) {
    const backup = code.includes("-") || code.length > 8;
    const r = await api("POST", "/auth/2fa/verify", backup ? { mfa_token, backup_code: code } : { mfa_token, code });
    setToken(r.access_token);
}
export async function register(fields) {
    const r = await api("POST", "/auth/register", fields);
    setToken(r.access_token);
}
export async function logout() {
    try {
        await api("POST", "/auth/logout");
    }
    finally {
        setToken(null);
    }
}
export async function upload(path, file, fields = {}) {
    const fd = new FormData();
    for (const [k, v] of Object.entries(fields))
        fd.append(k, v);
    fd.append("file", file);
    return api("POST", path, fd);
}
export const fullName = (u) => [u.first_name, u.last_name].filter(Boolean).join(" ") || u.username;
// fetchBlob — бинарный ответ с авторизацией (голосовые сообщения).
export async function fetchBlob(path, retry = true) {
    const res = await fetch(path, { headers: token ? { Authorization: `Bearer ${token}` } : {}, credentials: "same-origin" });
    if (res.status === 401 && retry && (await refresh()))
        return fetchBlob(path, false);
    if (!res.ok)
        throw new ApiError(res.status, "http_" + res.status, res.statusText);
    return res.blob();
}
