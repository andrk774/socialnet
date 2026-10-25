// Текущий пользователь — общий для всех экранов.
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { Self, get } from "./api.js";

let current: Self | null = null;
const subs = new Set<() => void>();

export const me = () => current;
export function setMe(u: Self | null) { current = u; subs.forEach((f) => f()); }
export async function loadMe() { setMe(await get<Self>("/users/me")); }

export function useMe(): Self | null {
  const [, force] = useState(0);
  useEffect(() => { const f = () => force((n) => n + 1); subs.add(f); return () => { subs.delete(f); }; }, []);
  return current;
}
