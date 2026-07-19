import { useState, type FormEvent } from "react";
import { forgotPassword, loginUrl, passwordLogin, register, resendVerification } from "../api";

type Mode = "login" | "register" | "forgot";

// The logged-out gate: Google on top (only when configured), email/password
// below. onAuthed fires only after a successful password login (register/forgot
// just show a status line, no session yet - the user still has to click the
// emailed link).
export default function AuthPanel({ onAuthed, google }: { onAuthed: () => void; google: boolean }) {
  const [mode, setMode] = useState<Mode>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [unverified, setUnverified] = useState(false); // login blocked on verification

  const switchMode = (next: Mode) => {
    setMode(next);
    setError(null);
    setNotice(null);
    setUnverified(false);
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    setUnverified(false);
    try {
      if (mode === "login") {
        await passwordLogin(email.trim(), password);
        onAuthed();
      } else if (mode === "register") {
        setNotice((await register(email.trim(), password, name.trim())).status);
      } else {
        setNotice((await forgotPassword(email.trim())).status);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : "something went wrong";
      setError(msg);
      if (/verify your email/i.test(msg)) setUnverified(true);
    } finally {
      setBusy(false);
    }
  };

  const resend = async () => {
    setUnverified(false);
    try {
      setNotice((await resendVerification(email.trim())).status);
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not resend");
    }
  };

  const submitLabel = busy ? "..." : mode === "login" ? "sign in" : mode === "register" ? "create account" : "send reset link";

  return (
    <div className="auth-panel">
      <p className="auth-lede">your creatures are bound to your identity.</p>

      {google && (
        <>
          <button className="google-button" onClick={() => { window.location.href = loginUrl; }}>
            continue with Google
          </button>
          <div className="auth-or"><span>or</span></div>
        </>
      )}

      <form className="auth-fields" onSubmit={submit}>
        {mode === "register" && (
          <input value={name} onChange={(e) => setName(e.target.value)}
            placeholder="what shall we call you? (optional)" aria-label="Name" maxLength={60} />
        )}
        <input type="email" required value={email} onChange={(e) => setEmail(e.target.value)}
          placeholder="email" aria-label="Email" autoComplete="email" />
        {mode !== "forgot" && (
          <input type="password" required minLength={8} value={password}
            onChange={(e) => setPassword(e.target.value)} placeholder="password" aria-label="Password"
            autoComplete={mode === "login" ? "current-password" : "new-password"} />
        )}
        <button type="submit" disabled={busy}>{submitLabel}</button>
      </form>

      {error && <p className="auth-msg auth-msg-error">{error}</p>}
      {unverified && <button className="auth-link" onClick={resend}>resend the verification email</button>}
      {notice && <p className="auth-msg">{notice}</p>}

      <div className="auth-switch">
        {mode !== "login" && <button onClick={() => switchMode("login")}>have an account? sign in</button>}
        {mode !== "register" && <button onClick={() => switchMode("register")}>new here? create an account</button>}
        {mode !== "forgot" && <button onClick={() => switchMode("forgot")}>forgot your password?</button>}
      </div>
    </div>
  );
}
