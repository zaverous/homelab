import { useCallback, useEffect, useRef, useState } from "react";
import { authStatus, createPet, feedPet, listPets, logout, type AuthStatus, type Pet } from "./api";
import AdoptForm from "./components/AdoptForm";
import AuthPanel from "./components/AuthPanel";
import CreepyEyes from "./components/CreepyEyes";
import DevPanel from "./components/DevPanel";
import PetCard from "./components/PetCard";
import ResetPanel from "./components/ResetPanel";
import Wordmark from "./components/Wordmark";

const POLL_MS = 5000;
const DEV_KEY = "kp_dev_mode";
const DEV_CLICKS = 5; // clicks on the footer within the window to toggle dev mode

export default function App() {
  const [pets, setPets] = useState<Pet[]>([]);
  const [auth, setAuth] = useState<AuthStatus | null>(null);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [showOthers, setShowOthers] = useState(false);
  const [showAdopt, setShowAdopt] = useState(false);
  const [showAccount, setShowAccount] = useState(false);
  const [devMode, setDevMode] = useState(() => localStorage.getItem(DEV_KEY) === "1");
  const [error, setError] = useState<string | null>(null);
  const [flash, setFlash] = useState<string | null>(null);
  const [loaded, setLoaded] = useState(false);
  // a reset link (?reset_token=...) takes over the gate until the user finishes
  const [resetToken, setResetToken] = useState<string | null>(() =>
    new URLSearchParams(window.location.search).get("reset_token"));
  const footerClicks = useRef<{ count: number; timer: number }>({ count: 0, timer: 0 });

  const refresh = useCallback(async () => {
    try { setPets(await listPets()); setError(null); }
    catch (e) { setError(e instanceof Error ? e.message : "something answered, but not the API"); }
    finally { setLoaded(true); }
  }, []);

  // re-read the session (after a password login the cookie is already set)
  const refreshAuth = useCallback(async () => {
    try { setAuth(await authStatus()); }
    catch { setAuth({ enabled: false, user: null }); setError("identification service unavailable"); }
  }, []);

  const clearReset = useCallback(() => {
    setResetToken(null);
    window.history.replaceState(null, "", window.location.pathname);
  }, []);

  useEffect(() => {
    if (!auth?.enabled || !auth.user) {
      setPets([]);
      setSelectedId(null);
      setLoaded(auth !== null);
      return;
    }
    setLoaded(false);
    void refresh();
    const timer = setInterval(() => void refresh(), POLL_MS);
    return () => clearInterval(timer);
  }, [auth?.enabled, auth?.user?.id, refresh]);

  useEffect(() => {
    void refreshAuth();
    // surface post-redirect signals the API/email links append to the URL
    const params = new URLSearchParams(window.location.search);
    const authError = params.get("auth_error");
    if (authError) {
      setError(`the identification failed. (${authError})`);
      window.history.replaceState(null, "", window.location.pathname);
    } else if (params.get("verified")) {
      setFlash("your email is confirmed. the link holds.");
      window.history.replaceState(null, "", window.location.pathname);
    }
  }, [refreshAuth]);

  const adopt = async (name: string) => {
    try { const pet = await createPet(name); setSelectedId(pet.id); setShowAdopt(false); await refresh(); }
    catch (e) { setError(e instanceof Error ? e.message : "adoption failed"); }
  };

  const feed = async (id: number) => {
    try { const fed = await feedPet(id); setPets((current) => current.map((pet) => pet.id === fed.id ? fed : pet)); setError(null); }
    catch (e) { setError(e instanceof Error ? e.message : "it refused the food"); }
  };

  const severLink = async () => {
    try {
      await logout();
      setAuth((current) => current ? { ...current, user: null } : current);
      setPets([]);
      setSelectedId(null);
      setShowAccount(false);
    }
    catch (e) { setError(e instanceof Error ? e.message : "the link would not sever"); }
  };

  const closeDevMode = () => {
    localStorage.setItem(DEV_KEY, "0");
    setDevMode(false);
  };

  // the hidden switch: click the footer DEV_CLICKS times in quick succession
  const footerClick = () => {
    const state = footerClicks.current;
    window.clearTimeout(state.timer);
    state.count += 1;
    if (state.count >= DEV_CLICKS) {
      state.count = 0;
      setDevMode((on) => {
        localStorage.setItem(DEV_KEY, on ? "0" : "1");
        return !on;
      });
    } else {
      state.timer = window.setTimeout(() => { state.count = 0; }, 1600);
    }
  };

  const maxHunger = pets.reduce((value, pet) => Math.max(value, pet.hunger), 0);
  const selectedPet = pets.find((pet) => pet.id === selectedId) ?? pets[0];
  const user = auth?.user ?? null;

  return (
    <div className="app-shell">
      <CreepyEyes intensity={maxHunger} />
      <div className="paper-noise" aria-hidden />
      <div className="site-frame">
        <header className="site-header">
          <button className="wordmark flicker" onClick={() => { setShowOthers(false); setShowAdopt(false); setShowAccount(false); }}>
            <Wordmark />
          </button>
          <nav aria-label="Main navigation">
            {user && <button onClick={() => { setShowOthers((value) => !value); setShowAdopt(false); setShowAccount(false); }}>the others</button>}
            {user && <button onClick={() => { setShowAdopt((value) => !value); setShowOthers(false); setShowAccount(false); }}>adopt</button>}
            {auth && !auth.enabled && <button disabled title="OAuth credentials are not configured">identity unavailable</button>}
            {user && <button onClick={() => { setShowAccount((value) => !value); setShowOthers(false); setShowAdopt(false); }}>{user.name.split(" ")[0].toLowerCase() || "you"}</button>}
          </nav>
        </header>

        {error && <div className="error-note">{error}</div>}
        {flash && <div className="flash-note">{flash}</div>}
        {showAdopt && <div className="adopt-drawer"><AdoptForm onAdopt={adopt} /></div>}
        {showOthers && (
          <aside className="pet-selector" aria-label="Choose another pet">
            {pets.map((pet) => <button key={pet.id} className={pet.id === selectedPet?.id ? "selected" : ""} onClick={() => { setSelectedId(pet.id); setShowOthers(false); }}><span>{pet.name}</span><small>{pet.hunger}% hungry</small></button>)}
            {pets.length === 0 && <p>there are no others.</p>}
          </aside>
        )}
        {showAccount && user && (
          <aside className="account-drawer" aria-label="Your account">
            {user.picture && <img src={user.picture} alt="" referrerPolicy="no-referrer" />}
            <p className="account-name">{user.name}</p>
            <p className="account-email">{user.email}</p>
            <button className="sever-button" onClick={severLink}>sever the link</button>
          </aside>
        )}

        {auth === null && <div className="empty-stage"><p>checking your mark...</p></div>}
        {auth && !auth.enabled && (
          <div className="empty-stage auth-gate" role="status">
            <p>identification is not configured.</p>
            <small>The keeper must configure the Google OAuth secret before pets can be accessed.</small>
          </div>
        )}
        {auth?.enabled && !user && resetToken && (
          <div className="empty-stage auth-gate">
            <ResetPanel token={resetToken} onDone={clearReset} />
          </div>
        )}
        {auth?.enabled && !user && !resetToken && (
          <div className="empty-stage auth-gate">
            <AuthPanel onAuthed={() => void refreshAuth()} />
          </div>
        )}
        {user && loaded && pets.length === 0 && !error
          ? <div className="empty-stage"><p>nothing of yours lives here yet.</p><AdoptForm onAdopt={adopt} /></div>
          : user && <main className="pet-home">{selectedPet && <PetCard key={selectedPet.id} pet={selectedPet} onFeed={feed} />}</main>}

        {devMode && user && <DevPanel onUnleashed={() => void refresh()} onClose={closeDevMode} />}

        <footer className="site-footer" onClick={footerClick}>
          {maxHunger >= 75 ? "it is watching you" : "all quiet. for now."}{devMode ? " · dev" : ""}
        </footer>
      </div>
    </div>
  );
}
