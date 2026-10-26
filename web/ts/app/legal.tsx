// Согласия: три отдельные отметки при регистрации и при новой редакции
// документов; удаление аккаунта (отзыв согласия на обработку ПДн).
import { h } from "../vendor/preact/index.js";
import { useState } from "../vendor/preact/hooks.js";
import { api, logout, post } from "./api.js";
import { loadMe } from "./session.js";
import { ErrorLine } from "./ui.js";

export interface Consents { age_confirmed: boolean; accept_terms: boolean; pd_consent: boolean }
export const noConsents: Consents = { age_confirmed: false, accept_terms: false, pd_consent: false };

const doc = (key: string, text: string) => <a href={`/legal/${key}`} target="_blank" rel="noopener">{text}</a>;

export function ConsentChecks({ value, onChange }: { value: Consents; onChange: (v: Consents) => void }) {
  const box = (k: keyof Consents) => ({
    type: "checkbox", required: true, checked: value[k],
    onChange: (e: Event) => onChange({ ...value, [k]: (e.target as HTMLInputElement).checked }),
  });
  return (
    <fieldset class="consents">
      <legend class="sr-only">Согласия</legend>
      <label class="check"><input {...box("age_confirmed")} /> <span>Мне исполнилось 18 лет</span></label>
      <label class="check"><input {...box("accept_terms")} /> <span>Принимаю {doc("terms", "пользовательское соглашение")} и {doc("privacy", "политику обработки персональных данных")}</span></label>
      <label class="check"><input {...box("pd_consent")} /> <span>Даю {doc("consent", "согласие на обработку персональных данных")}</span></label>
    </fieldset>
  );
}

// ConsentGate: документы обновились — до принятия сайт только для чтения.
export function ConsentGate() {
  const [c, setC] = useState(noConsents);
  const [error, setError] = useState<unknown>(null);
  async function submit(e: Event) {
    e.preventDefault();
    setError(null);
    try { await post("/users/me/consents", c); await loadMe(); } catch (err) { setError(err); }
  }
  return (
    <main class="auth consent-gate">
      <h1>Мы обновили документы</h1>
      <p>Чтобы продолжить, прочитайте новую редакцию и подтвердите согласие. Если не согласны, можно выйти или удалить аккаунт в разделе «Безопасность».</p>
      <form class="auth-form" onSubmit={submit}>
        <ConsentChecks value={c} onChange={setC} />
        <ErrorLine error={error} />
        <button class="primary wide">Продолжить</button>
      </form>
      <p class="auth-switch"><button class="link" onClick={() => logout()}>Выйти</button> · <a href="/settings">Удалить аккаунт</a></p>
    </main>
  );
}

export function DeleteAccount() {
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState("");
  const [sure, setSure] = useState("");
  const [error, setError] = useState<unknown>(null);
  async function submit(e: Event) {
    e.preventDefault();
    setError(null);
    try {
      await api("DELETE", "/users/me", { password });
      await logout().catch(() => {});
      location.assign("/");
    } catch (err) { setError(err); }
  }
  return (
    <section class="danger-zone">
      <h2>Удаление аккаунта</h2>
      <p class="muted">Удаляются записи, комментарии, фото, видео, документы, маршруты, походы и объявления, имя и почта стираются — это отзыв согласия на обработку персональных данных. Сообщения, уже отправленные другим, останутся у них от «удалённого аккаунта». Монеты на счёте сгорят. Сообщества перейдут к старшим участникам. Отменить удаление нельзя.</p>
      {!open ? <button class="ghost danger" onClick={() => setOpen(true)}>Удалить аккаунт…</button> : (
        <form class="stack" onSubmit={submit}>
          <label>Пароль<input type="password" autoComplete="current-password" required value={password} onInput={(e) => setPassword((e.target as HTMLInputElement).value)} /></label>
          <label>Напишите «удалить»<input value={sure} required onInput={(e) => setSure((e.target as HTMLInputElement).value)} /></label>
          <ErrorLine error={error} />
          <div class="row">
            <button class="ghost danger" disabled={sure.trim().toLowerCase() !== "удалить"}>Удалить навсегда</button>
            <button type="button" class="ghost" onClick={() => setOpen(false)}>Отмена</button>
          </div>
        </form>
      )}
    </section>
  );
}
