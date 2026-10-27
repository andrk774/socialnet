// Страницы по ссылкам из писем: подтверждение почты и новый пароль, а также
// форма «Забыли пароль?». Работают и без входа.
import { ComponentChildren, Fragment, h } from "../vendor/preact/index.js";
import { useEffect, useRef, useState } from "../vendor/preact/hooks.js";
import { api } from "./api.js";
import { Captcha, captchaSiteKey, tokenFrom } from "./captcha.js";
import { query } from "./router.js";
import { ErrorLine, Loading } from "./ui.js";

function Shell({ title, children }: { title: string; children: ComponentChildren }) {
  return (
    <main class="auth">
      <div class="auth-mark" aria-hidden="true" />
      <h1 class="wordmark">socialnet</h1>
      <h2>{title}</h2>
      {children}
      <p class="auth-switch"><a href="/">На главную</a></p>
    </main>
  );
}

export function VerifyEmailPage() {
  const [state, setState] = useState<"wait" | "ok" | "error">("wait");
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    api("POST", "/auth/email/verify", { token: query("token") ?? "" }).then(() => setState("ok"), (e) => { setError(e); setState("error"); });
  }, []);
  return (
    <Shell title="Подтверждение почты">
      {state === "wait" && <Loading />}
      {state === "ok" && <p>Почта подтверждена. Теперь доступны кошелёк и подписка.</p>}
      {state === "error" && <><ErrorLine error={error} /><p class="muted">Ссылка устарела? Войдите и отправьте письмо ещё раз.</p></>}
    </Shell>
  );
}

export function ResetPasswordPage() {
  const [password, setPassword] = useState("");
  const [done, setDone] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const submit = (e: Event) => {
    e.preventDefault(); setError(null);
    api("POST", "/auth/password/reset", { token: query("token") ?? "", password }).then(() => setDone(true), setError);
  };
  return (
    <Shell title="Новый пароль">
      {done ? <p>Пароль изменён, все сеансы завершены. <a href="/">Войдите</a> с новым паролем.</p> : (
        <form class="auth-form" onSubmit={submit}>
          <label>Новый пароль<input type="password" autoComplete="new-password" minLength={8} maxLength={128} required value={password}
            onInput={(e) => setPassword((e.target as HTMLInputElement).value)} /></label>
          <ErrorLine error={error} />
          <button class="primary wide">Сохранить</button>
        </form>
      )}
    </Shell>
  );
}

export function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [siteKey, setSiteKey] = useState("");
  const box = useRef<HTMLDivElement | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => { captchaSiteKey().then(setSiteKey); }, []);
  const submit = (e: Event) => {
    e.preventDefault(); setError(null);
    api("POST", "/auth/password/forgot", { email, captcha_token: tokenFrom(box.current) }).then(() => setSent(true), setError);
  };
  return (
    <Shell title="Восстановление пароля">
      {sent ? <p>Если такой адрес зарегистрирован, мы отправили на него ссылку. Она действует ограниченное время.</p> : (
        <form class="auth-form" onSubmit={submit}>
          <label>Почта<input type="email" autoComplete="email" required value={email} onInput={(e) => setEmail((e.target as HTMLInputElement).value)} /></label>
          <Captcha siteKey={siteKey} box={box} />
          <ErrorLine error={error} />
          <button class="primary wide">Отправить ссылку</button>
        </form>
      )}
    </Shell>
  );
}
