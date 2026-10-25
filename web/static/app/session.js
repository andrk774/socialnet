// Текущий пользователь — общий для всех экранов.
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { get } from "./api.js";
let current = null;
const subs = new Set();
export const me = () => current;
export function setMe(u) { current = u; subs.forEach((f) => f()); }
export async function loadMe() { setMe(await get("/users/me")); }
export function useMe() {
    const [, force] = useState(0);
    useEffect(() => { const f = () => force((n) => n + 1); subs.add(f); return () => { subs.delete(f); }; }, []);
    return current;
}
