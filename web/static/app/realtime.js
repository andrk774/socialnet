// WebSocket с одноразовым тикетом: токен не попадает в URL и логи.
import { post, signedIn } from "./api.js";
const handlers = new Map();
let ws = null;
let retry = 1000;
let stopped = true;
export function on(type, fn) {
    if (!handlers.has(type))
        handlers.set(type, new Set());
    handlers.get(type).add(fn);
    return () => { handlers.get(type).delete(fn); };
}
async function connect() {
    if (stopped || !signedIn())
        return;
    try {
        const { ticket } = await post("/ws/ticket");
        const proto = location.protocol === "https:" ? "wss:" : "ws:";
        ws = new WebSocket(`${proto}//${location.host}/api/v1/ws?ticket=${encodeURIComponent(ticket)}`);
        ws.onopen = () => { retry = 1000; };
        ws.onmessage = (ev) => {
            try {
                const { type, data } = JSON.parse(ev.data);
                handlers.get(type)?.forEach((fn) => fn(data));
            }
            catch { /* мусор игнорируем */ }
        };
        ws.onclose = () => { ws = null; schedule(); };
    }
    catch {
        schedule();
    }
}
function schedule() {
    if (stopped)
        return;
    setTimeout(connect, retry);
    retry = Math.min(retry * 2, 30000);
}
export function startRealtime() { if (!stopped)
    return; stopped = false; connect(); }
export function stopRealtime() { stopped = true; ws?.close(); ws = null; }
export function sendTyping(conversationId) {
    ws?.readyState === WebSocket.OPEN && ws.send(JSON.stringify({ type: "typing", conversation_id: conversationId }));
}
