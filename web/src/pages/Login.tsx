import { useState, type FormEvent } from "react";
import { login, ApiError } from "@/api/client";

interface LoginProps {
  onLogin: (accessToken: string, username: string, role: string) => void;
}

export function Login({ onLogin }: LoginProps) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      const resp = await login(username, password);
      onLogin(resp.accessToken, resp.username, resp.role);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message || "Invalid credentials");
      } else {
        setError("Failed to connect to engine");
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        minHeight: "100vh",
        background: "var(--bg-primary)",
      }}
    >
      <div
        style={{
          width: 360,
          padding: "32px 28px",
          border: "1px solid var(--border)",
          borderRadius: 8,
          background: "var(--bg-secondary)",
        }}
      >
        <h1
          style={{
            fontFamily: "var(--font-mono)",
            fontSize: 16,
            fontWeight: 600,
            marginBottom: 4,
            color: "var(--text-primary)",
          }}
        >
          banyan
        </h1>
        <p
          style={{
            fontSize: 13,
            color: "var(--text-muted)",
            marginBottom: 24,
          }}
        >
          Sign in to continue
        </p>

        <form onSubmit={handleSubmit}>
          <div style={{ marginBottom: 16 }}>
            <label
              htmlFor="username"
              style={{
                display: "block",
                fontSize: 12,
                fontWeight: 600,
                marginBottom: 6,
                color: "var(--text-secondary)",
              }}
            >
              Username
            </label>
            <input
              id="username"
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoFocus
              required
              style={{
                width: "100%",
                padding: "8px 10px",
                fontSize: 13,
                fontFamily: "var(--font-mono)",
                border: "1px solid var(--border)",
                borderRadius: 4,
                background: "var(--bg-primary)",
                color: "var(--text-primary)",
                outline: "none",
                boxSizing: "border-box",
              }}
            />
          </div>

          <div style={{ marginBottom: 20 }}>
            <label
              htmlFor="password"
              style={{
                display: "block",
                fontSize: 12,
                fontWeight: 600,
                marginBottom: 6,
                color: "var(--text-secondary)",
              }}
            >
              Password
            </label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              style={{
                width: "100%",
                padding: "8px 10px",
                fontSize: 13,
                fontFamily: "var(--font-mono)",
                border: "1px solid var(--border)",
                borderRadius: 4,
                background: "var(--bg-primary)",
                color: "var(--text-primary)",
                outline: "none",
                boxSizing: "border-box",
              }}
            />
          </div>

          {error && (
            <p
              style={{
                fontSize: 12,
                color: "var(--red)",
                marginBottom: 16,
              }}
            >
              {error}
            </p>
          )}

          <button
            type="submit"
            disabled={loading || !username || !password}
            style={{
              width: "100%",
              padding: "8px 0",
              fontSize: 13,
              fontWeight: 600,
              border: "1px solid var(--border)",
              borderRadius: 4,
              background: loading ? "var(--bg-tertiary)" : "var(--text-primary)",
              color: loading ? "var(--text-muted)" : "var(--bg-primary)",
              cursor: loading ? "wait" : "pointer",
            }}
          >
            {loading ? "Signing in..." : "Sign in"}
          </button>
        </form>
      </div>
    </div>
  );
}
