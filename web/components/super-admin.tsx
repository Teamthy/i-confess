"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useAuth } from "@/lib/auth-context";
import { adminApi, useAdminResource, type AdminResult } from "./admin";

/* Super admin overview with RBAC stats */
export function SuperAdminOverview() {
  const { token } = useAuth();
  const stats = useAdminResource<Record<string, any>>("/admin/stats");
  const roles = useAdminResource<{ roles?: any[] }>("/admin/rbac/roles");
  const users = useAdminResource<{ users?: any[]; count?: number }>("/admin/users?limit=5");
  const health = useAdminResource<Record<string, any>>("/admin/system/health");
  const security = useAdminResource<Record<string, any>>("/admin/security/overview");
  const perms = useAdminResource<{ permissions?: any[]; count?: number }>("/admin/rbac/permissions");

  const roleList = Array.isArray(roles.data?.roles) ? roles.data!.roles : [];
  const superAdmins = roleList.filter((r: any) => r.name === "super_admin").length ? "System" : "—";

  return (
    <>
      <h1 className="adm-title">Super Admin Console</h1>
      <p className="adm-lede">
        Full platform control: RBAC, user management, system health, audit, and security. Every privileged action is
        logged with actor, role, and IP. Super admin is the ultimate authority — it bypasses all permission checks.
      </p>

      <div className="adm-stats">
        {[
          ["Total users", health.data?.inventory?.total_users ?? stats.data?.categories ?? "—"],
          ["Admin accounts", health.data?.inventory?.admin_users ?? "—"],
          ["Roles defined", roles.data?.roles?.length ?? "—"],
          ["Permissions", perms.data?.count ?? "—"],
          ["Categories", stats.data?.categories ?? "—"],
          ["Published confessions", stats.data?.published ?? "—"],
        ].map(([label, value]) => (
          <div className="adm-stat" key={String(label)}>
            <span>{String(label)}</span>
            <b>{String(value)}</b>
          </div>
        ))}
      </div>

      <div className="adm-super-grid">
        <section className="adm-super-card">
          <h3>🛡️ RBAC — Role hierarchy</h3>
          <p className="small" style={{ marginBottom: 12 }}>
            Roles are sorted by privilege level. Super admin (100) can assign any role. Others can only assign lower
            levels. System roles are code-defined and immutable; custom roles are DB-stored.
          </p>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(240px,1fr))", gap: 12 }}>
            {roleList.slice(0, 6).map((r: any) => (
              <div key={r.name} className="adm-role-card">
                <div className="adm-role-head">
                  <span className="adm-role-name">{r.display_name || r.name}</span>
                  <span className="adm-role-level">L{r.level ?? "—"}</span>
                </div>
                <span className="adm-role-desc">{r.description?.slice(0, 120) || ""}</span>
                <div className="adm-perm-grid">
                  {(r.permissions || []).slice(0, 4).map((p: string) => (
                    <span key={p} className={"adm-perm" + (p === "*" ? " super" : "")}>
                      {p}
                    </span>
                  ))}
                  {(r.permissions || []).length > 4 && (
                    <span className="adm-perm">+{r.permissions.length - 4} more</span>
                  )}
                </div>
              </div>
            ))}
          </div>
          <Link className="textlink" href="/admin/rbac" style={{ marginTop: 14, display: "inline-flex" }}>
            Manage RBAC →
          </Link>
        </section>

        <section className="adm-super-card">
          <h3>📊 System & Security</h3>
          {health.error ? (
            <p className="small">{health.error}</p>
          ) : (
            <ul className="adm-list">
              <li>
                <span>Database</span>
                <b>{health.data?.database?.healthy ? "Healthy" : "—"}</b>
              </li>
              <li>
                <span>Email</span>
                <b>{health.data?.subsystems?.email ? "Enabled" : "Disabled"}</b>
              </li>
              <li>
                <span>Storage</span>
                <b>{health.data?.subsystems?.storage ? "Enabled" : "Disabled"}</b>
              </li>
              <li>
                <span>Queue durable</span>
                <b>{health.data?.queue ? "Yes" : "Memory"}</b>
              </li>
              <li>
                <span>Failed logins</span>
                <b>{security.data?.failed_logins ?? "—"}</b>
              </li>
              <li>
                <span>Token reuse detected</span>
                <b>{security.data?.token_reuse ?? "—"}</b>
              </li>
            </ul>
          )}
          <div style={{ display: "flex", gap: 8, marginTop: 12, flexWrap: "wrap" }}>
            <Link className="btn btn-ghost btn-sm" href="/admin/system">
              System health
            </Link>
            <Link className="btn btn-ghost btn-sm" href="/admin/security">
              Security
            </Link>
            <Link className="btn btn-ghost btn-sm" href="/admin/audit">
              Audit log
            </Link>
          </div>
        </section>
      </div>

      <section className="adm-card">
        <div className="adm-card-head">
          <h3>Recent users</h3>
          <Link className="btn btn-ghost btn-sm" href="/admin/users">
            View all
          </Link>
        </div>
        {users.error && <p className="small">{users.error}</p>}
        <div className="adm-table-wrap">
          <table className="adm-table">
            <thead>
              <tr>
                <th>User</th>
                <th>Roles</th>
                <th>Status</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              {(users.data?.users || []).map((u: any) => (
                <tr key={u.id}>
                  <td>
                    <b>{u.display_name || u.email}</b>
                    <small>{u.email}</small>
                  </td>
                  <td>
                    <div className="adm-chips">
                      {(u.roles || []).map((r: string) => (
                        <span key={r} className="adm-chip">
                          {r}
                        </span>
                      ))}
                      {(!u.roles || u.roles.length === 0) && <span className="small">no admin role</span>}
                    </div>
                  </td>
                  <td>
                    <span className={"adm-pill " + (u.status === "active" ? "green" : "red")}>{u.status}</span>
                  </td>
                  <td>{(u.created_at || "").slice(0, 10)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </>
  );
}

/* RBAC management */
export function RBACConsole() {
  const { token } = useAuth();
  const [message, setMessage] = useState("");
  const [tab, setTab] = useState<"roles" | "permissions" | "matrix">("roles");

  const rolesRes = useAdminResource<{ roles: any[] }>("/admin/rbac/roles");
  const permsRes = useAdminResource<{ permissions: any[]; grouped: Record<string, any[]>; categories: Record<string, string[]> }>(
    "/admin/rbac/permissions"
  );

  const [createForm, setCreateForm] = useState({ name: "", display_name: "", description: "", permissions: [] as string[] });
  const [busy, setBusy] = useState(false);

  const roles = rolesRes.data?.roles || [];
  const perms = permsRes.data?.permissions || [];
  const grouped = permsRes.data?.grouped || {};

  const togglePerm = (p: string) => {
    setCreateForm((f) => ({
      ...f,
      permissions: f.permissions.includes(p) ? f.permissions.filter((x) => x !== p) : [...f.permissions, p],
    }));
  };

  const createRole = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!createForm.name.trim() || !createForm.display_name.trim() || createForm.permissions.length === 0) {
      setMessage("Name, display name, and at least one permission are required.");
      return;
    }
    setBusy(true);
    const res = await adminApi("/admin/rbac/roles", token, { method: "POST", body: createForm });
    setBusy(false);
    setMessage(res.ok ? `Role ${createForm.name} created.` : res.message);
    if (res.ok) {
      setCreateForm({ name: "", display_name: "", description: "", permissions: [] });
      rolesRes.reload();
    }
  };

  const deleteRole = async (name: string) => {
    if (!confirm(`Delete custom role ${name}? This cannot be undone and fails if still assigned.`)) return;
    setBusy(true);
    const res = await adminApi(`/admin/rbac/roles/${encodeURIComponent(name)}`, token, { method: "DELETE" });
    setBusy(false);
    setMessage(res.ok ? `Role ${name} deleted.` : res.message);
    if (res.ok) rolesRes.reload();
  };

  return (
    <>
      <h1 className="adm-title">Roles & Permissions</h1>
      <p className="adm-lede">
        System roles are defined in code and cannot be edited. Custom roles are stored in the database and can be
        created by super admins. Permissions are granular: content:read, audio:qa, voice:rights:manage, user:suspend,
        etc. Super admin has * and bypasses all checks.
      </p>

      {message && <div className="adm-note">{message}</div>}

      <div className="adm-toolbar">
        {(["roles", "permissions", "matrix"] as const).map((t) => (
          <button
            key={t}
            className={"btn btn-sm " + (tab === t ? "btn-primary" : "btn-ghost")}
            onClick={() => setTab(t)}
          >
            {t === "roles" ? "Roles" : t === "permissions" ? "Permissions" : "Matrix"}
          </button>
        ))}
        <span className="adm-pill blue">
          {roles.length} roles · {perms.length} permissions
        </span>
      </div>

      {tab === "roles" && (
        <>
          <section className="adm-card">
            <h3>Create custom role</h3>
            <form onSubmit={createRole} style={{ display: "grid", gap: 12, maxWidth: 720 }}>
              <div className="adm-split">
                <input
                  className="adm-input"
                  placeholder="name — lowercase_underscore"
                  value={createForm.name}
                  onChange={(e) => setCreateForm({ ...createForm, name: e.target.value.toLowerCase() })}
                />
                <input
                  className="adm-input"
                  placeholder="Display name"
                  value={createForm.display_name}
                  onChange={(e) => setCreateForm({ ...createForm, display_name: e.target.value })}
                />
              </div>
              <input
                className="adm-input"
                placeholder="Description"
                value={createForm.description}
                onChange={(e) => setCreateForm({ ...createForm, description: e.target.value })}
              />
              <div>
                <span className="small" style={{ display: "block", marginBottom: 8 }}>
                  Permissions — {createForm.permissions.length} selected
                </span>
                <div className="adm-chips">
                  {perms.map((p: any) => (
                    <label key={p.name} className={"adm-chip " + (createForm.permissions.includes(p.name) ? "on" : "")} style={{ cursor: "pointer" }}>
                      <input
                        type="checkbox"
                        checked={createForm.permissions.includes(p.name)}
                        onChange={() => togglePerm(p.name)}
                        style={{ display: "none" }}
                      />
                      {p.name}
                    </label>
                  ))}
                </div>
              </div>
              <button className="btn btn-primary btn-sm" type="submit" disabled={busy}>
                Create role
              </button>
            </form>
          </section>

          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(320px,1fr))", gap: 14 }}>
            {roles.map((r: any) => (
              <div key={r.name} className="adm-role-card">
                <div className="adm-role-head">
                  <div>
                    <div className="adm-role-name">{r.display_name || r.name}</div>
                    <div style={{ fontSize: 11, color: "var(--n500)", marginTop: 2 }}>{r.name}</div>
                  </div>
                  <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
                    <span className="adm-role-level">L{r.level ?? (r.is_system ? "sys" : "custom")}</span>
                    {r.is_system ? (
                      <span className="adm-pill blue">system</span>
                    ) : (
                      <span className="adm-pill">custom</span>
                    )}
                  </div>
                </div>
                <div className="adm-role-desc">{r.description}</div>
                <div className="adm-perm-grid">
                  {(r.permissions || []).map((p: string) => (
                    <span key={p} className={"adm-perm" + (p === "*" ? " super" : "")}>
                      {p}
                    </span>
                  ))}
                </div>
                {!r.is_system && (
                  <button className="btn btn-ghost btn-sm" onClick={() => deleteRole(r.name)} disabled={busy} style={{ marginTop: 8, alignSelf: "flex-start" }}>
                    Delete role
                  </button>
                )}
              </div>
            ))}
          </div>
        </>
      )}

      {tab === "permissions" && (
        <div className="adm-grid">
          {Object.entries(grouped).map(([cat, list]) => (
            <section key={cat} className="adm-card">
              <h3>{cat}</h3>
              <ul className="adm-list">
                {(list as any[]).map((p: any) => (
                  <li key={p.name}>
                    <span>
                      <b style={{ display: "inline", marginRight: 8 }}>{p.display_name}</b>
                      <span className="adm-kbd">{p.name}</span>
                      <br />
                      <small>{p.description}</small>
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
      )}

      {tab === "matrix" && (
        <section className="adm-card">
          <h3>Permission matrix — system roles</h3>
          <p className="small" style={{ marginBottom: 12 }}>
            Rows are permissions, columns are roles. A check means the role grants that permission. Super admin has all.
          </p>
          <div className="adm-table-wrap">
            <table className="adm-matrix">
              <thead>
                <tr>
                  <th>Permission</th>
                  {roles
                    .filter((r: any) => r.is_system)
                    .map((r: any) => (
                      <th key={r.name}>{r.name}</th>
                    ))}
                </tr>
              </thead>
              <tbody>
                {perms.map((p: any) => (
                  <tr key={p.name}>
                    <td>
                      <span className="adm-kbd">{p.name}</span>
                      <br />
                      <small>{p.display_name}</small>
                    </td>
                    {roles
                      .filter((r: any) => r.is_system)
                      .map((r: any) => {
                        const has = (r.permissions || []).includes("*") || (r.permissions || []).includes(p.name);
                        return (
                          <td key={r.name + p.name}>
                            <span className={"adm-check " + (has ? "on" : "")}>{has ? "✓" : ""}</span>
                          </td>
                        );
                      })}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </>
  );
}

/* Enhanced users with RBAC */
export function SuperAdminUsers() {
  const { token } = useAuth();
  const [search, setSearch] = useState("");
  const [q, setQ] = useState("");
  const [selected, setSelected] = useState<any | null>(null);
  const [detail, setDetail] = useState<any | null>(null);
  const [message, setMessage] = useState("");
  const [roleToAssign, setRoleToAssign] = useState("content_admin");
  const [busy, setBusy] = useState("");

  const usersRes = useAdminResource<{ users: any[]; count: number }>(`/admin/users?search=${encodeURIComponent(q)}&limit=50`);
  const rolesRes = useAdminResource<{ roles: any[] }>("/admin/rbac/roles");

  const users = usersRes.data?.users || [];
  const roles = rolesRes.data?.roles || [];

  const loadDetail = async (user: any) => {
    setSelected(user);
    setDetail(null);
    const res = await adminApi<any>(`/admin/users/${encodeURIComponent(user.id)}`, token);
    if (res.ok) setDetail(res.data);
    else setMessage(res.message);
  };

  const assignRole = async () => {
    if (!selected) return;
    setBusy("assign");
    const res = await adminApi(`/admin/users/${encodeURIComponent(selected.id)}/roles`, token, {
      method: "POST",
      body: { role: roleToAssign },
    });
    setBusy("");
    setMessage(res.ok ? `Assigned ${roleToAssign} to ${selected.email}` : res.message);
    if (res.ok) {
      usersRes.reload();
      loadDetail(selected);
    }
  };

  const removeRole = async (roleName: string) => {
    if (!selected) return;
    setBusy(roleName);
    const res = await adminApi(`/admin/users/${encodeURIComponent(selected.id)}/roles?role=${encodeURIComponent(roleName)}`, token, {
      method: "DELETE",
      body: { role: roleName },
    });
    setBusy("");
    setMessage(res.ok ? `Removed ${roleName} from ${selected.email}` : res.message);
    if (res.ok) {
      usersRes.reload();
      loadDetail(selected);
    }
  };

  const suspendUser = async (status: string) => {
    if (!selected) return;
    const res = await adminApi("/admin/users/status", token, {
      method: "POST",
      body: { user_id: selected.id, status },
    });
    setMessage(res.ok ? `User status set to ${status}` : res.message);
    if (res.ok) usersRes.reload();
  };

  const revokeSessions = async () => {
    if (!selected) return;
    if (!confirm(`Revoke all sessions for ${selected.email}? They will be signed out everywhere.`)) return;
    const res = await adminApi(`/admin/users/${encodeURIComponent(selected.id)}/sessions/revoke`, token, { method: "POST" });
    setMessage(res.ok ? "All sessions revoked" : res.message);
  };

  return (
    <>
      <h1 className="adm-title">Users & Access</h1>
      <p className="adm-lede">
        Search all accounts, view their effective roles and permissions, assign or revoke roles, suspend, and revoke
        sessions. Role assignment is audited and respects privilege levels — you cannot assign a role higher than your
        own.
      </p>

      {message && <div className="adm-note">{message}</div>}

      <div className="adm-toolbar">
        <div className="adm-search">
          <input
            className="adm-input"
            placeholder="Search by email or name"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") setQ(search.trim());
            }}
          />
        </div>
        <button className="btn btn-primary btn-sm" onClick={() => setQ(search.trim())}>
          Search
        </button>
        <button className="btn btn-ghost btn-sm" onClick={() => { setSearch(""); setQ(""); }}>
          Clear
        </button>
        <span className="adm-pill blue">{usersRes.data?.count ?? users.length} users</span>
      </div>

      <div className="adm-split">
        <section className="adm-card">
          <h3>Accounts</h3>
          {usersRes.error && <p className="small">{usersRes.error}</p>}
          {usersRes.loading && <p className="small">Loading…</p>}
          <div style={{ maxHeight: 600, overflowY: "auto" }}>
            {users.map((u: any) => (
              <div key={u.id} className="adm-user-row" style={{ cursor: "pointer", background: selected?.id === u.id ? "var(--n50)" : undefined, borderRadius: 10, padding: "10px 8px" }} onClick={() => loadDetail(u)}>
                <div className="adm-user-avatar">{(u.email[0] || "?").toUpperCase()}</div>
                <div className="adm-user-meta">
                  <div className="adm-user-email">{u.display_name || u.email}</div>
                  <div className="adm-user-sub">
                    <span>{u.email}</span>
                    <span className={"adm-pill " + (u.status === "active" ? "green" : "red")} style={{ fontSize: 9 }}>{u.status}</span>
                    {(u.roles || []).slice(0, 2).map((r: string) => (
                      <span key={r} className="adm-kbd">
                        {r}
                      </span>
                    ))}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </section>

        <section className="adm-card">
          {selected ? (
            <>
              <div className="adm-card-head">
                <h3>{detail?.user?.email || selected.email}</h3>
                <button className="btn btn-ghost btn-sm" onClick={() => setSelected(null)}>
                  Close
                </button>
              </div>

              {!detail ? (
                <p className="small">Loading detail…</p>
              ) : (
                <>
                  <ul className="adm-list">
                    <li><span>ID</span><b className="adm-kbd">{detail.user?.id}</b></li>
                    <li><span>Status</span><b>{detail.user?.status}</b></li>
                    <li><span>Created</span><b>{(detail.user?.created_at || "").slice(0, 10)}</b></li>
                    <li><span>Subscription</span><b>{detail.subscription?.plan || "free"} · {detail.subscription?.status || "active"}</b></li>
                  </ul>

                  <h4 style={{ fontSize: 13, margin: "16px 0 8px" }}>Roles</h4>
                  <div className="adm-chips">
                    {(detail.roles || []).map((r: any) => (
                      <span key={r.name} className="adm-chip on">
                        {r.display_name || r.name}
                        <button onClick={() => removeRole(r.name)} disabled={busy === r.name} style={{ border: 0, background: "transparent", cursor: "pointer", marginLeft: 4 }}>✕</button>
                      </span>
                    ))}
                    {(detail.roles || []).length === 0 && <span className="small">No admin role</span>}
                  </div>

                  <div className="adm-row-actions" style={{ marginTop: 12 }}>
                    <select className="adm-input" value={roleToAssign} onChange={(e) => setRoleToAssign(e.target.value)}>
                      {roles.map((r: any) => (
                        <option key={r.name} value={r.name}>{r.display_name || r.name} (L{r.level ?? "?"})</option>
                      ))}
                    </select>
                    <button className="btn btn-primary btn-sm" onClick={assignRole} disabled={busy === "assign"}>
                      Assign role
                    </button>
                  </div>

                  <h4 style={{ fontSize: 13, margin: "18px 0 8px" }}>Effective permissions — {detail.permissions?.length || 0}</h4>
                  <div className="adm-perm-grid">
                    {(detail.permissions || []).slice(0, 20).map((p: string) => (
                      <span key={p} className="adm-perm">{p}</span>
                    ))}
                    {(detail.permissions || []).length > 20 && <span className="adm-perm">+{detail.permissions.length - 20} more</span>}
                  </div>

                  <h4 style={{ fontSize: 13, margin: "18px 0 8px" }}>Sessions — {detail.sessions?.length || 0}</h4>
                  <ul className="adm-list">
                    {(detail.sessions || []).slice(0, 5).map((s: any) => (
                      <li key={s.id}>
                        <span>{s.platform || "unknown"} · {(s.created_at || "").slice(0, 10)}</span>
                        <b className="adm-kbd">{s.id.slice(0, 8)}</b>
                      </li>
                    ))}
                  </ul>

                  <div className="adm-row-actions" style={{ marginTop: 16 }}>
                    <button className="btn btn-ghost btn-sm" onClick={() => suspendUser("suspended")}>Suspend</button>
                    <button className="btn btn-ghost btn-sm" onClick={() => suspendUser("active")}>Restore</button>
                    <button className="btn btn-ghost btn-sm" onClick={revokeSessions}>Revoke sessions</button>
                  </div>

                  <div className="adm-code" style={{ marginTop: 16 }}>
                    <pre style={{ margin: 0, whiteSpace: "pre-wrap", wordBreak: "break-all" }}>{JSON.stringify(detail, null, 2)}</pre>
                  </div>
                </>
              )}
            </>
          ) : (
            <div className="adm-empty" style={{ marginTop: 0 }}>
              <h2>Select a user</h2>
              <p>Choose an account on the left to view roles, permissions, subscription, sessions, and take action. All actions are audited.</p>
            </div>
          )}
        </section>
      </div>
    </>
  );
}

/* System health */
export function SystemHealthConsole() {
  const health = useAdminResource<any>("/admin/system/health");
  const queue = useAdminResource<any>("/admin/queue");
  const metrics = useAdminResource<any>("/admin/metrics");

  return (
    <>
      <h1 className="adm-title">System Health</h1>
      <p className="adm-lede">Live inventory, subsystem status, queue, and security counters. Readiness is database-only; mail and push degrade gracefully.</p>

      {health.error && <div className="adm-note error">{health.error}</div>}

      <div className="adm-stats">
        {Object.entries(health.data?.inventory || {}).map(([k, v]) => (
          <div className="adm-stat" key={k}>
            <span>{k.replace(/_/g, " ")}</span>
            <b>{String(v)}</b>
          </div>
        ))}
      </div>

      <div className="adm-grid">
        <section className="adm-card">
          <h3>Subsystems</h3>
          <ul className="adm-list">
            {Object.entries(health.data?.subsystems || {}).map(([k, v]) => (
              <li key={k}>
                <span>{k}</span>
                <b style={{ color: v ? "green" : "var(--err)" }}>{v ? "Operational" : "Degraded"}</b>
              </li>
            ))}
          </ul>
          {health.data?.cache && (
            <>
              <h4 style={{ fontSize: 13, margin: "14px 0 8px" }}>Cache</h4>
              <div className="adm-code">
                <pre style={{ margin: 0 }}>{JSON.stringify(health.data.cache, null, 2)}</pre>
              </div>
            </>
          )}
        </section>

        <section className="adm-card">
          <h3>Queue — {queue.data?.durable ? "Durable" : "Memory"}</h3>
          {queue.error ? <p className="small">{queue.error}</p> : (
            <ul className="adm-list">
              {Object.entries(queue.data?.stats || {}).map(([k, v]) => (
                <li key={k}><span>{k}</span><b>{String(v)}</b></li>
              ))}
            </ul>
          )}
          <p className="small">Types: {(queue.data?.types || []).join(", ") || "—"}</p>
        </section>

        <section className="adm-card">
          <h3>Security counters</h3>
          {metrics.error ? <p className="small">{metrics.error}</p> : (
            <ul className="adm-list">
              {Object.entries(metrics.data?.counters || {}).map(([k, v]) => (
                <li key={k}><span>{k}</span><b>{String(v)}</b></li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </>
  );
}

/* Security console */
export function SecurityConsole() {
  const sec = useAdminResource<any>("/admin/security/overview");
  const audit = useAdminResource<any[]>("/admin/audit");

  return (
    <>
      <h1 className="adm-title">Security</h1>
      <p className="adm-lede">Authentication events, token reuse detection, rate limiting, and recent security events. Failure counts are admin-only — they reveal whether an attack is landing.</p>

      <div className="adm-stats">
        <div className="adm-stat"><span>Failed logins</span><b>{sec.data?.failed_logins ?? "—"}</b></div>
        <div className="adm-stat"><span>Token reuse</span><b>{sec.data?.token_reuse ?? "—"}</b></div>
        <div className="adm-stat"><span>Rate limited</span><b>{sec.data?.rate_limited ?? "—"}</b></div>
        <div className="adm-stat"><span>MFA failures</span><b>{sec.data?.mfa_failures ?? "—"}</b></div>
      </div>

      <div className="adm-grid">
        <section className="adm-card">
          <h3>Recent security events</h3>
          <div className="adm-table-wrap">
            <table className="adm-table">
              <thead><tr><th>When</th><th>Type</th><th>User</th><th>IP</th></tr></thead>
              <tbody>
                {(sec.data?.recent_events || []).map((e: any) => (
                  <tr key={e.id}>
                    <td>{(e.created_at || "").slice(0, 19).replace("T", " ")}</td>
                    <td><span className="adm-status">{e.event_type}</span></td>
                    <td className="adm-kbd">{(e.user_id || "").slice(0, 8)}</td>
                    <td>{e.ip_address}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        <section className="adm-card">
          <h3>Recent privileged actions</h3>
          <div className="adm-table-wrap">
            <table className="adm-table">
              <thead><tr><th>When</th><th>Actor</th><th>Action</th><th>Target</th></tr></thead>
              <tbody>
                {(Array.isArray(audit.data) ? audit.data : []).slice(0, 20).map((e: any, i: number) => (
                  <tr key={e.id || i}>
                    <td>{(e.created_at || "").slice(0, 19).replace("T", " ")}</td>
                    <td>{e.actor_email || e.actor || "—"}</td>
                    <td><span className="adm-status">{e.action}</span></td>
                    <td>{e.target_id || e.entity_id || "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      </div>
    </>
  );
}
