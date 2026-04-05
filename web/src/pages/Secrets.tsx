import { KeyRound, Plus, Trash2, Eye, EyeOff, RefreshCw, AlertTriangle } from "lucide-react";
import { useSecrets } from "@/hooks/use-api";
import { createSecret, deleteSecret, getSecret } from "@/api/client";
import { timeAgo } from "@/lib/format";
import { useState } from "react";

export function Secrets() {
  const { data: secrets, error: listError, refresh } = useSecrets();
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState("");
  const [newValue, setNewValue] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [showNewValue, setShowNewValue] = useState(false);
  const [revealed, setRevealed] = useState<Record<string, string>>({});
  const [revealing, setRevealing] = useState<Record<string, boolean>>({});

  const secretsNotEnabled = listError?.includes("secrets not enabled") || listError?.includes("secrets.key");

  const handleCreate = async () => {
    if (!newName.trim() || !newValue.trim()) return;
    setCreating(true);
    setCreateError(null);
    try {
      await createSecret(newName.trim(), newValue.trim());
      setNewName("");
      setNewValue("");
      setShowNewValue(false);
      setShowCreate(false);
      refresh();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : "Failed to create secret");
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (name: string) => {
    if (!confirm(`Delete secret "${name}"? This cannot be undone.`)) return;
    try {
      await deleteSecret(name);
      setRevealed((prev) => { const next = { ...prev }; delete next[name]; return next; });
      refresh();
    } catch (err) {
      alert(`Failed to delete: ${err instanceof Error ? err.message : "unknown error"}`);
    }
  };

  const handleReveal = async (name: string) => {
    if (revealed[name]) {
      setRevealed((prev) => { const next = { ...prev }; delete next[name]; return next; });
      return;
    }
    setRevealing((prev) => ({ ...prev, [name]: true }));
    try {
      const resp = await getSecret(name, true);
      setRevealed((prev) => ({ ...prev, [name]: resp.value ?? "" }));
    } catch (err) {
      alert(`Failed to reveal: ${err instanceof Error ? err.message : "unknown error"}`);
    } finally {
      setRevealing((prev) => ({ ...prev, [name]: false }));
    }
  };

  if (secretsNotEnabled) {
    return (
      <>
        <div className="page-header">
          <h1 className="page-title"><KeyRound size={22} /> Secrets</h1>
          <div className="header-actions">
            <button className="icon-btn" onClick={refresh} title="Retry"><RefreshCw size={16} /></button>
          </div>
        </div>
        <div className="panel">
          <div className="panel-header">
            <div className="panel-title"><AlertTriangle size={16} /> Secrets Not Enabled</div>
          </div>
          <div style={{ padding: 24 }}>
            <p style={{ marginBottom: 16, color: "var(--text-secondary)" }}>
              The engine needs an encryption key before secrets can be stored. Generate one and restart the engine:
            </p>
            <div className="log-pane" style={{ maxHeight: "none", marginBottom: 16 }}>
              <div><span className="text-muted"># Generate a 256-bit encryption key</span></div>
              <div><span className="log-msg">banyan-engine secrets generate-key</span></div>
              <div>&nbsp;</div>
              <div><span className="text-muted"># Or manually: create a 32-byte random key</span></div>
              <div><span className="log-msg">{"head -c 32 /dev/urandom > /etc/banyan/keys/secrets.key"}</span></div>
              <div><span className="log-msg">chmod 600 /etc/banyan/keys/secrets.key</span></div>
              <div>&nbsp;</div>
              <div><span className="text-muted"># Then restart the engine</span></div>
              <div><span className="log-msg">sudo systemctl restart banyan-engine</span></div>
            </div>
            <p style={{ fontSize: 12, color: "var(--text-muted)" }}>
              The key file must be exactly 32 bytes with 0600 permissions. Once configured, refresh this page.
            </p>
          </div>
        </div>
      </>
    );
  }

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><KeyRound size={22} /> Secrets</h1>
        <div className="header-actions">
          <button className="btn btn-primary" onClick={() => setShowCreate((v) => !v)}>
            <Plus size={14} /> New Secret
          </button>
          <button className="icon-btn" onClick={refresh} title="Refresh"><RefreshCw size={16} /></button>
        </div>
      </div>

      {showCreate && (
        <div className="panel" style={{ marginBottom: 16 }}>
          <div className="panel-header"><div className="panel-title"><Plus size={16} /> Create Secret</div></div>
          <div style={{ padding: 16, display: "flex", gap: 12, alignItems: "flex-end" }}>
            <div style={{ flex: 1 }}>
              <label className="stat-label" style={{ marginBottom: 4, display: "block" }}>Name</label>
              <input className="form-input" placeholder="SECRET_NAME" value={newName} onChange={(e) => setNewName(e.target.value)} style={{ width: "100%", fontSize: 12, padding: "6px 10px" }} />
            </div>
            <div style={{ flex: 2 }}>
              <label className="stat-label" style={{ marginBottom: 4, display: "block" }}>Value</label>
              <div className="input-reveal">
                <input className="form-input" type={showNewValue ? "text" : "password"} placeholder="secret value" value={newValue} onChange={(e) => setNewValue(e.target.value)} style={{ width: "100%", fontSize: 12, padding: "6px 10px" }} />
                <button className="input-reveal-toggle" type="button" onClick={() => setShowNewValue((v) => !v)} title={showNewValue ? "Hide value" : "Show value"}>
                  {showNewValue ? <EyeOff size={14} /> : <Eye size={14} />}
                </button>
              </div>
            </div>
            <button className="btn btn-primary" onClick={() => void handleCreate()} disabled={creating || !newName.trim() || !newValue.trim()}>
              {creating ? "Creating..." : "Create"}
            </button>
            <button className="btn btn-secondary" onClick={() => { setShowCreate(false); setCreateError(null); }}>Cancel</button>
          </div>
          {createError && <div className="alert alert-error" style={{ margin: "0 16px 16px" }}>{createError}</div>}
        </div>
      )}

      <div className="panel">
        <div className="panel-header">
          <div className="panel-title"><KeyRound size={16} /> All Secrets <span className="panel-count">{secrets?.length ?? 0}</span></div>
        </div>
        {!secrets?.length ? (
          <div className="empty-state"><KeyRound /><h3>No secrets</h3><p>Create a secret to store sensitive configuration values.</p></div>
        ) : (
          <table>
            <thead><tr><th>Name</th><th>Value</th><th>Created</th><th>Updated</th><th style={{ width: 100 }}>Actions</th></tr></thead>
            <tbody>
              {secrets.map((s) => (
                <tr key={s.name}>
                  <td className="mono">{s.name}</td>
                  <td className="mono" style={{ fontSize: 12 }}>
                    {revealed[s.name] !== undefined ? (
                      <code style={{ background: "var(--bg)", padding: "2px 6px", borderRadius: 4 }}>{revealed[s.name]}</code>
                    ) : (
                      <span className="text-muted">********</span>
                    )}
                  </td>
                  <td className="mono text-muted">{s.createdAt ? timeAgo(s.createdAt) : "-"}</td>
                  <td className="mono text-muted">{s.updatedAt ? timeAgo(s.updatedAt) : "-"}</td>
                  <td>
                    <div style={{ display: "flex", gap: 4 }}>
                      <button className="icon-btn" onClick={() => void handleReveal(s.name)} title={revealed[s.name] !== undefined ? "Hide" : "Reveal"} disabled={revealing[s.name]}>
                        {revealed[s.name] !== undefined ? <EyeOff size={14} /> : <Eye size={14} />}
                      </button>
                      <button className="icon-btn" onClick={() => void handleDelete(s.name)} title="Delete" style={{ color: "var(--red)" }}>
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
