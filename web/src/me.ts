import { useQuery } from "@tanstack/react-query";
import { api } from "./api";

export type Me = {
  user_id: string;
  email: string;
  full_name: string;
  avatar_url?: string | null;
};

/** Logged-in admin user. Shares the ["me"] cache with AdminApp. */
export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => api<Me>("/api/me"),
  });
}

/** "Budi" → "Budi (saya)" when the id belongs to the logged-in user. */
export function nameWithSaya(name: string, userId: string | null | undefined, meId?: string): string {
  if (userId && meId && userId === meId) return `${name} (saya)`;
  return name;
}
