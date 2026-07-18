import { useCallback, useEffect, useState } from "react";
import { createPet, feedPet, listPets, type Pet } from "./api";
import AdoptForm from "./components/AdoptForm";
import CreepyEyes from "./components/CreepyEyes";
import PetCard from "./components/PetCard";

// Poll the API - the queue workers mutate hunger in the background, so the UI
// has to keep looking over its shoulder.
const POLL_MS = 5000;

export default function App() {
  const [pets, setPets] = useState<Pet[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loaded, setLoaded] = useState(false);

  const refresh = useCallback(async () => {
    try {
      setPets(await listPets());
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "something answered, but not the API");
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const t = setInterval(() => void refresh(), POLL_MS);
    return () => clearInterval(t);
  }, [refresh]);

  const adopt = async (name: string) => {
    try {
      await createPet(name);
      setError(null);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "adoption failed");
    }
  };

  const feed = async (id: number) => {
    try {
      // Feed returns the updated pet - patch it in immediately, then let the
      // next poll reconcile whatever the workers have done meanwhile.
      const fed = await feedPet(id);
      setPets((prev) => prev.map((p) => (p.id === fed.id ? fed : p)));
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "it refused the food");
    }
  };

  // Worst-case hunger drives the dread: the hungrier the hungriest pet, the
  // more eyes open in the dark.
  const maxHunger = pets.reduce((m, p) => Math.max(m, p.hunger), 0);

  return (
    <div className="min-h-screen text-zinc-300">
      <CreepyEyes intensity={maxHunger} />

      <div className="relative z-10 mx-auto max-w-5xl px-6 py-10">
        <header className="mb-10 text-center">
          <h1 className="flicker font-mono text-4xl font-bold uppercase tracking-[0.45em] text-zinc-200">
            Kubepets
          </h1>
          <p className="mt-2 font-mono text-xs tracking-[0.2em] text-zinc-600">
            they are always hungry. the queue never sleeps.
          </p>
        </header>

        {error && (
          <div className="relative z-10 mb-6 border border-red-900 bg-red-950/40 px-4 py-3 text-center font-mono text-xs text-red-400">
            {error}
          </div>
        )}

        <div className="mb-10 flex justify-center">
          <AdoptForm onAdopt={adopt} />
        </div>

        {loaded && pets.length === 0 && !error && (
          <p className="text-center font-mono text-sm text-zinc-600">
            nothing lives here yet. adopt something.
          </p>
        )}

        <main className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {pets.map((pet) => (
            <PetCard key={pet.id} pet={pet} onFeed={feed} />
          ))}
        </main>

        <footer className="mt-14 text-center font-mono text-[10px] tracking-[0.2em] text-zinc-700">
          kubepets · phase 4c · {maxHunger >= 75 ? "it is watching you" : "all quiet. for now."}
        </footer>
      </div>
    </div>
  );
}
