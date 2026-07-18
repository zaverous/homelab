// Thin client for the real Go API routes (apps/kubepets/api/main.go).
// Base path /api, same-origin: in prod nginx strips /api and proxies to the
// kubepets-api Service; in dev vite.config.ts does the identical rewrite.
const BASE = "/api";

export interface Pet {
  id: number;
  name: string;
  hunger: number; // 0 (fed) .. 100 (starving)
  created_at: string;
  last_fed_at?: string;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, init);
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* non-JSON error body - keep the status message */
    }
    throw new Error(msg);
  }
  return (await res.json()) as T;
}

export const listPets = () => request<Pet[]>("/pets");

export const createPet = (name: string) =>
  request<Pet>("/pets", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });

export const getPet = (id: number) => request<Pet>(`/pets/${id}`);

export const feedPet = (id: number) =>
  request<Pet>(`/pets/${id}/feed`, { method: "POST" });
