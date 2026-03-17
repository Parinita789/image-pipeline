import { create } from "zustand";
import { persist } from "zustand/middleware";

interface AuthState {
  token: string | null;
  userId: string | null;
  firstName: string | null;
  setAuth: (token: string, userId: string, firstName: string) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      userId: null,
      firstName: null,
      setAuth: (token, userId, firstName) => set({ token, userId, firstName }),
      logout: () => set({ token: null, userId: null, firstName: null }),
    }),
    { name: "auth-storage" }
  )
);
