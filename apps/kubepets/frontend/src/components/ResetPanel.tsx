import { useState, type FormEvent } from "react";
import { resetPassword } from "../api";

// Shown when the app is opened from a password-reset link (?reset_token=...).
// On success the user still returns to the normal gate and signs in with the new
// password - reset does not auto-login.
export default function ResetPanel({ token, onDone }: { token: string; onDone: () => void }) {
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (busy) return;
    if (password !== confirm) { setError("the passwords do not match"); return; }
    setBusy(true);
    setError(null);
    try {
      await resetPassword(token, password);
      setDone(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not reset password");
    } finally {
      setBusy(false);
    }
  };

  if (done) {
    return (
      <div className="auth-panel">
        <p className="auth-lede">your password has been changed.</p>
        <button className="google-button" onClick={onDone}>return and sign in</button>
      </div>
    );
  }

  return (
    <div className="auth-panel">
      <p className="auth-lede">choose a new password.</p>
      <form className="auth-fields" onSubmit={submit}>
        <input type="password" required minLength={8} value={password}
          onChange={(e) => setPassword(e.target.value)} placeholder="new password"
          aria-label="New password" autoComplete="new-password" />
        <input type="password" required minLength={8} value={confirm}
          onChange={(e) => setConfirm(e.target.value)} placeholder="confirm password"
          aria-label="Confirm password" autoComplete="new-password" />
        <button type="submit" disabled={busy}>{busy ? "..." : "set new password"}</button>
      </form>
      {error && <p className="auth-msg auth-msg-error">{error}</p>}
      <div className="auth-switch">
        <button onClick={onDone}>back to sign in</button>
      </div>
    </div>
  );
}
