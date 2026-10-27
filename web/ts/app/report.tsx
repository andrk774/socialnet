// Кнопка «Пожаловаться»: причина из готовых вариантов или своя, отправка в
// очередь модераторов (POST /reports). Жалоба анонимна для автора.
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { post } from "./api.js";
import { ErrorLine } from "./ui.js";

const reasons = ["Спам или реклама", "Оскорбления", "Ложные или опасные сведения", "Чужие личные данные", "Незаконный контент"];

export type ReportTarget =
  | { type: "post" | "comment" | "user" | "community" | "companion" | "route_point" | "article"; id: number }
  | { type: "track" | "video" | "document" | "route"; uuid: string };

export function ReportButton({ target, cls = "link" }: { target: ReportTarget; cls?: string }) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState(reasons[0]);
  const [other, setOther] = useState("");
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<unknown>(null);
  if (sent) return <span class="muted" role="status">Жалоба отправлена модераторам.</span>;
  async function submit(e: Event) {
    e.preventDefault();
    setError(null);
    const text = reason === "" ? other.trim() : reason;
    try {
      await post("/reports", "uuid" in target
        ? { target_type: target.type, target_uuid: target.uuid, reason: text }
        : { target_type: target.type, target_id: target.id, reason: text });
      setSent(true);
    } catch (err) { setError(err); }
  }
  if (!open) return <button type="button" class={cls} onClick={() => setOpen(true)}>Пожаловаться</button>;
  return (
    <form class="report-form" onSubmit={submit}>
      <label>Причина
        <select value={reason} onChange={(e) => setReason((e.target as HTMLSelectElement).value)}>
          {reasons.map((r) => <option key={r} value={r}>{r}</option>)}
          <option value="">Другое…</option>
        </select>
      </label>
      {reason === "" && <label>Опишите<input required maxLength={1000} value={other} onInput={(e) => setOther((e.target as HTMLInputElement).value)} /></label>}
      <span class="row tight">
        <button class="primary">Отправить жалобу</button>
        <button type="button" class="ghost" onClick={() => setOpen(false)}>Отмена</button>
      </span>
      <ErrorLine error={error} />
    </form>
  );
}
