// Вход по QR-коду: на новом устройстве — код и ожидание, на телефоне, где
// вы уже вошли, — подтверждение с описанием устройства.
import { h } from "../vendor/preact/index.js";
import { useEffect, useState } from "../vendor/preact/hooks.js";
import { ApiError, QRLoginStart, get, post, qrLoginPoll, qrLoginStart } from "./api.js";
import { navigate, query } from "./router.js";
import { ErrorLine, Loading } from "./ui.js";

// QRLoginBox — на странице входа: код обновляется сам, пока его не подтвердят.
export function QRLoginBox({ onCancel }: { onCancel: () => void }) {
  const [start, setStart] = useState<QRLoginStart | null>(null);
  const [left, setLeft] = useState(0);
  const [state, setState] = useState<"wait" | "expired" | "rejected">("wait");
  const [error, setError] = useState<unknown>(null);
  const [round, setRound] = useState(0);

  useEffect(() => {
    let stop = false;
    setStart(null); setState("wait"); setError(null);
    qrLoginStart().then((s) => {
      if (stop) return;
      setStart(s);
      const until = new Date(s.expires_at).getTime();
      const tick = async () => {
        if (stop) return;
        setLeft(Math.max(0, Math.round((until - Date.now()) / 1000)));
        try {
          if (await qrLoginPoll(s.id, s.poll_token)) return; // вошли: App переключится сам
        } catch (e) {
          if (e instanceof ApiError && e.code === "qr_rejected") { setState("rejected"); return; }
          if (e instanceof ApiError && e.code === "qr_expired") { setState("expired"); return; }
          if (!(e instanceof ApiError && e.status === 429)) { setError(e); return; }
        }
        setTimeout(tick, 2000);
      };
      tick();
    }, setError);
    return () => { stop = true; };
  }, [round]);

  const src = start ? "data:image/svg+xml;charset=utf-8," + encodeURIComponent(start.qr_svg) : "";
  return (
    <section class="qr-login" aria-live="polite">
      <h2>Вход по QR-коду</h2>
      {state === "wait" && <ol class="muted">
        <li>Откройте камеру на телефоне, где вы уже вошли в socialnet.</li>
        <li>Наведите на код и откройте ссылку.</li>
        <li>Проверьте устройство и нажмите «Подтвердить вход».</li>
      </ol>}
      {state === "wait" && (start ? <img src={src} width={220} height={220} alt="QR-код для входа" /> : <Loading />)}
      {state === "wait" && start && <p class="muted">Код действует ещё {left} с.</p>}
      {state === "expired" && <p>Код устарел.</p>}
      {state === "rejected" && <p>Вход отклонён на телефоне.</p>}
      <ErrorLine error={error} />
      <div class="row">
        {state !== "wait" && <button class="primary" onClick={() => setRound(round + 1)}>Новый код</button>}
        <button class="ghost" onClick={onCancel}>Вход по паролю</button>
      </div>
    </section>
  );
}

interface QRInfo { id: string; user_agent: string; ip?: string; created_at: string; expires_at: string }

// describeUA — коротко: браузер и система, без разбора всех вариантов.
export function describeUA(ua: string): string {
  const browser = /YaBrowser/.test(ua) ? "Яндекс Браузер" : /Edg\//.test(ua) ? "Edge" : /OPR\//.test(ua) ? "Opera" :
    /Firefox\//.test(ua) ? "Firefox" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "Браузер";
  const os = /Windows/.test(ua) ? "Windows" : /Android/.test(ua) ? "Android" : /iPhone|iPad/.test(ua) ? "iOS" :
    /Mac OS X/.test(ua) ? "macOS" : /Linux/.test(ua) ? "Linux" : "неизвестная система";
  return `${browser}, ${os}`;
}

// QRApprovePage — /qr-login?c=… на телефоне.
export function QRApprovePage() {
  const id = query("c") ?? "";
  const [info, setInfo] = useState<QRInfo | null>(null);
  const [done, setDone] = useState<"approved" | "rejected" | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => { get<QRInfo>(`/qr-login/${encodeURIComponent(id)}`).then(setInfo, setError); }, [id]);
  const decide = (approve: boolean) =>
    post(`/qr-login/${encodeURIComponent(id)}/${approve ? "approve" : "reject"}`).then(() => setDone(approve ? "approved" : "rejected"), setError);

  if (done) return (
    <section class="qr-approve">
      <h1>{done === "approved" ? "Вход подтверждён" : "Вход отклонён"}</h1>
      <p>{done === "approved" ? "На том устройстве откроется ваш аккаунт. Завершить сеанс можно в разделе «Безопасность»." : "Устройство не получило доступ."}</p>
      <button class="ghost" onClick={() => navigate("/")}>В ленту</button>
    </section>
  );
  if (error) return (
    <section class="qr-approve">
      <h1>Код не действует</h1>
      <p>{error instanceof ApiError && error.code === "qr_expired" ? "Код устарел или уже использован. Обновите его на том устройстве и отсканируйте снова." : ""}</p>
      <ErrorLine error={error instanceof ApiError && error.code === "qr_expired" ? null : error} />
    </section>
  );
  if (!info) return <Loading />;
  return (
    <section class="qr-approve">
      <h1>Войти на другом устройстве?</h1>
      <dl class="facts">
        <div><dt>Устройство</dt><dd>{describeUA(info.user_agent)}</dd></div>
        {info.ip && <div><dt>IP-адрес</dt><dd>{info.ip}</dd></div>}
      </dl>
      <p class="warn" role="note">Подтверждайте, только если этот код сейчас перед вами — на вашем компьютере или планшете.
        Если код прислали в сообщении или показали на чужом сайте — это попытка украсть аккаунт: нажмите «Отклонить».</p>
      <div class="row">
        <button class="primary" onClick={() => decide(true)}>Подтвердить вход</button>
        <button class="ghost danger" onClick={() => decide(false)}>Отклонить</button>
      </div>
    </section>
  );
}
