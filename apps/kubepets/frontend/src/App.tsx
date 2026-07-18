import { useCallback, useEffect, useRef, useState } from "react";
import { authStatus, createPet, feedPet, listPets, loginUrl, logout, type AuthStatus, type Pet } from "./api";
import AdoptForm from "./components/AdoptForm";
import CreepyEyes from "./components/CreepyEyes";
import DevPanel from "./components/DevPanel";
import PetCard from "./components/PetCard";
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
  const [loaded, setLoaded] = useState(false);
  const footerClicks = useRef<{ count: number; timer: number }>({ count: 0, timer: 0 });

  const refresh = useCallback(async () => {
    try { setPets(await listPets()); setError(null); }
    catch (e) { setError(e instanceof Error ? e.message : "something answered, but not the API"); }
    finally { setLoaded(true); }
  }, []);

  useEffect(() => {
    void refresh();
    const timer = setInterval(() => void refresh(), POLL_MS);
    return () => clearInterval(timer);
  }, [refresh]);

  useEffect(() => {
    void authStatus().then(setAuth).catch(() => setAuth({ enabled: false, user: null }));
    // surface a failed OAuth callback (the API redirects back with ?auth_error=)
    const params = new URLSearchParams(window.location.search);
    const authError = params.get("auth_error");
    if (authError) {
      setError(`the identification failed. (${authError})`);
      window.history.replaceState(null, "", window.location.pathname);
    }
  }, []);

  const adopt = async (name: string) => {
    try { const pet = await createPet(name); setSelectedId(pet.id); setShowAdopt(false); await refresh(); }
    catch (e) { setError(e instanceof Error ? e.message : "adoption failed"); }
  };

  const feed = async (id: number) => {
    try { const fed = await feedPet(id); setPets((current) => current.map((pet) => pet.id === fed.id ? fed : pet)); setError(null); }
    catch (e) { setError(e instanceof Error ? e.message : "it refused the food"); }
  };

  const severLink = async () => {
    try { await logout(); setAuth((current) => current ? { ...current, user: null } : current); setShowAccount(false); await refresh(); }
    catch (e) { setError(e instanceof Error ? e.message : "the link would not sever"); }
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
            <button onClick={() => { setShowOthers((value) => !value); setShowAdopt(false); setShowAccount(false); }}>the others</button>
            <button onClick={() => { setShowAdopt((value) => !value); setShowOthers(false); setShowAccount(false); }}>adopt</button>
            {auth?.enabled && !user && <button onClick={() => { window.location.href = loginUrl; }}>identify yourself</button>}
            {user && <button onClick={() => { setShowAccount((value) => !value); setShowOthers(false); setShowAdopt(false); }}>{user.name.split(" ")[0].toLowerCase() || "you"}</button>}
          </nav>
        </header>

        {error && <div className="error-note">{error}</div>}
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

        {loaded && pets.length === 0 && !error ? <div className="empty-stage"><p>{user ? "nothing of yours lives here yet." : "nothing lives here yet."}</p><AdoptForm onAdopt={adopt} /></div> : <main className="pet-home">{selectedPet && <PetCard key={selectedPet.id} pet={selectedPet} onFeed={feed} />}</main>}

        {devMode && <DevPanel onUnleashed={() => void refresh()} />}

        <footer className="site-footer" onClick={footerClick}>
          {maxHunger >= 75 ? "it is watching you" : "all quiet. for now."}{devMode ? " · dev" : ""}
        </footer>
      </div>
    </div>
  );
}
