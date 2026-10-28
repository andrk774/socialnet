// QR-код похода: картинка для объявления или чата, скачивание SVG и
// статистика сканов для организатора.
import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { fetchBlob, get, saveFile } from "./api.js";
import { ErrorLine } from "./ui.js";

interface QRStats { scans: number; joins: number; scans_week: number; days: { day: string; scans: number; joins: number }[] }

export function HikeQR({ id, title, organizer }: { id: number; title: string; organizer: boolean }) {
  const [open, setOpen] = useState(false);
  const [src, setSrc] = useState<string | null>(null);
  const [stats, setStats] = useState<QRStats | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    if (!open) return;
    // SVG — картинкой через data:, без вставки разметки в страницу
    fetchBlob(`/api/v1/events/${id}/qr.svg`).then((b) => b.text())
      .then((svg) => setSrc("data:image/svg+xml;charset=utf-8," + encodeURIComponent(svg)), setError);
    if (organizer) get<QRStats>(`/events/${id}/qr-stats`).then(setStats, setError);
  }, [open, id, organizer]);
  return (
    <details class="hike-qr" onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)}>
      <summary>QR-код похода</summary>
      <p class="muted">Для объявления в клубе, на вокзале или в чате: камера телефона откроет страницу похода.</p>
      {src && <img src={src} width={200} height={200} alt={`QR-код: ${title}`} />}
      <div class="row">
        <button class="ghost" onClick={() => saveFile(`/api/v1/events/${id}/qr.svg`, `qr-${title}.svg`).catch(setError)}>Скачать SVG</button>
        <a class="ghost button-link" href={`/events/${id}/print`}>Лист для печати</a>
      </div>
      {stats && (
        <p class="qr-stats" aria-live="polite">
          Отсканировали: <strong data-stat="scans">{stats.scans}</strong>{stats.scans_week > 0 ? ` (за неделю ${stats.scans_week})` : ""}
          {" · "}записались после скана: <strong data-stat="joins">{stats.joins}</strong>
          {stats.scans > 0 ? ` · ${Math.round((stats.joins / stats.scans) * 100)}%` : ""}
        </p>
      )}
      <ErrorLine error={error} />
    </details>
  );
}
