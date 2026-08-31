import { useQuery } from "@tanstack/react-query";
import { api } from "./client";

// Thrown when the server refuses. Carries the status so a caller can tell an
// ended session from a real failure, and the server's own sentence so the
// screen shows what the server said rather than a sentence invented here.
export class Refused extends Error {
  constructor(readonly status: number, message: string) {
    super(message);
    this.name = "Refused";
  }
}

type Answer<T> = { data?: T; error?: unknown; response: Response };

// unwrap turns the client's { data, error } into a value or a throw, so every
// screen handles failure the same way through TanStack Query rather than each
// one inventing its own branch.
export function unwrap<T>({ data, error, response }: Answer<T>): T {
  if (error !== undefined || !response.ok) {
    throw new Refused(response.status, detailOf(error) ?? response.statusText);
  }
  return data as T;
}

function detailOf(error: unknown): string | undefined {
  if (typeof error === "object" && error !== null && "detail" in error) {
    const detail = (error as { detail?: unknown }).detail;
    if (typeof detail === "string") return detail;
  }
  return undefined;
}

export function useProducts() {
  return useQuery({
    queryKey: ["products"],
    queryFn: async () => unwrap(await api.GET("/v1/products", {})),
  });
}

export function useWhoAmI() {
  return useQuery({
    queryKey: ["session"],
    queryFn: async () => unwrap(await api.GET("/v1/session/me", {})),
    retry: false,
  });
}
