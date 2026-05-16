import { useState, useEffect, useCallback } from "react";

interface AuthState {
  accessToken: string | null;
  username: string | null;
  role: string | null;
  isAuthenticated: boolean;
}

const AUTH_STORAGE_KEY = "banyan_auth";

function loadAuthState(): AuthState {
  try {
    const stored = localStorage.getItem(AUTH_STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored) as {
        accessToken?: string;
        username?: string;
        role?: string;
      };
      if (parsed.accessToken) {
        return {
          accessToken: parsed.accessToken,
          username: parsed.username ?? null,
          role: parsed.role ?? null,
          isAuthenticated: true,
        };
      }
    }
  } catch {
    // invalid stored data
  }
  return { accessToken: null, username: null, role: null, isAuthenticated: false };
}

export function useAuth() {
  const [auth, setAuth] = useState<AuthState>(loadAuthState);

  useEffect(() => {
    if (auth.isAuthenticated) {
      localStorage.setItem(
        AUTH_STORAGE_KEY,
        JSON.stringify({
          accessToken: auth.accessToken,
          username: auth.username,
          role: auth.role,
        }),
      );
    } else {
      localStorage.removeItem(AUTH_STORAGE_KEY);
    }
  }, [auth]);

  const login = useCallback(
    (accessToken: string, username: string, role: string) => {
      setAuth({ accessToken, username, role, isAuthenticated: true });
    },
    [],
  );

  const logout = useCallback(() => {
    setAuth({
      accessToken: null,
      username: null,
      role: null,
      isAuthenticated: false,
    });
  }, []);

  return { ...auth, login, logout };
}

/** Get the current access token for API calls */
export function getAccessToken(): string | null {
  try {
    const stored = localStorage.getItem(AUTH_STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored) as { accessToken?: string };
      return parsed.accessToken ?? null;
    }
  } catch {
    // ignore
  }
  return null;
}
