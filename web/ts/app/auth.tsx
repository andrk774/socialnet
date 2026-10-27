import { h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { login, register, verify2FA } from "./api.js";
import { ConsentChecks, noConsents } from "./legal.js";
import { Captcha, captchaSiteKey, tokenFrom } from "./captcha.js";
import { ErrorLine } from "./ui.js";

export function AuthPage() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [mfa, setMfa] = useState<string | null>(null);
  const [f, setF] = useState({ email: "", password: "", username: "", first_name: "", last_name: "", code: "" });
  const [consents, setConsents] = useState(noConsents);
  const [siteKey, setSiteKey] = useState("");
  const captchaBox = useRef<HTMLDivElement | null>(null);
  useEffect(() => { captchaSiteKey().then(setSiteKey); }, []);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const field = (k: keyof typeof f) => ({ value: f[k], onInput: (e: Event) => setF({ ...f, [k]: (e.target as HTMLInputElement).value }) });

  async function submit(e: Event) {
    e.preventDefault();
    setBusy(true); setError(null);
    try {
      if (mfa) await verify2FA(mfa, f.code.trim());
      else if (mode === "login") { const r = await login(f.email, f.password); if (r.mfa_token) setMfa(r.mfa_token); }
      else await register({ email: f.email, password: f.password, username: f.username, first_name: f.first_name, last_name: f.last_name, ...consents, captcha_token: tokenFrom(captchaBox.current) });
    } catch (err) { setError(err); } finally { setBusy(false); }
  }

  return (
    <main class="auth">
      <div class="auth-mark" aria-hidden="true" />
      <h1 class="wordmark">socialnet</h1>
      <p class="auth-lead">Друзья, сообщения и сообщества — без рекламы в ленте.</p>
      <form class="auth-form" onSubmit={submit}>
        {mfa ? (
          <label>Код из приложения или резервный код
            <input {...field("code")} inputMode="numeric" autoComplete="one-time-code" autoFocus required />
          </label>
        ) : (
          <fieldset>
            <legend class="sr-only">{mode === "login" ? "Вход" : "Регистрация"}</legend>
            {mode === "register" && (
              <div class="row">
                <label>Имя<input {...field("first_name")} autoComplete="given-name" required maxLength={64} /></label>
                <label>Фамилия<input {...field("last_name")} autoComplete="family-name" maxLength={64} /></label>
              </div>
            )}
            {mode === "register" && <label>Имя пользователя<input {...field("username")} autoComplete="username" pattern="[A-Za-z0-9_]{3,32}" required title="3–32 символа: латиница, цифры, _" /></label>}
            <label>Почта<input {...field("email")} type="email" autoComplete="email" required /></label>
            <label>Пароль<input {...field("password")} type="password" autoComplete={mode === "login" ? "current-password" : "new-password"} minLength={8} maxLength={128} required /></label>
            {mode === "register" && <ConsentChecks value={consents} onChange={setConsents} />}
            {mode === "register" && <Captcha siteKey={siteKey} box={captchaBox} />}
          </fieldset>
        )}
        <ErrorLine error={error} />
        <button class="primary wide" disabled={busy}>{mfa ? "Подтвердить" : mode === "login" ? "Войти" : "Создать аккаунт"}</button>
      </form>
      {!mfa && (
        <p class="auth-switch">
          {mode === "login" ? "Ещё нет аккаунта? " : "Уже есть аккаунт? "}
          <button class="link" onClick={() => { setMode(mode === "login" ? "register" : "login"); setError(null); }}>
            {mode === "login" ? "Зарегистрироваться" : "Войти"}
          </button>
        </p>
      )}
      {mode === "login" && !mfa && <p class="auth-switch"><a href="/forgot-password">Забыли пароль?</a></p>}
      <p class="legal-links muted"><a href="/legal/terms">Соглашение</a> · <a href="/legal/privacy">Персональные данные</a></p>
    </main>
  );
}
