"use client";

/* ============================================================================
   iCONFESS web — the admin console.

   One web app now serves both audiences, so the console lives here rather than
   in a second Next.js project with its own visual language. Three rules keep
   that safe:

   1. THE SERVER DECIDES. Nothing here is gated by a client-side role check
      that the API does not repeat. Every panel simply calls its admin
      endpoint with the session's bearer token; a non-admin gets 403 from the
      API and the console renders that fact. The gate below is a courtesy, not
      a security boundary.
   2. NO SECOND DESIGN SYSTEM. The old console shipped an indigo/violet
      palette the design system had already retired. These panels use the
      iCONFESS tokens, so the admin surface reads as the same product.
   3. HONEST STATES. Loading, empty, forbidden and unreachable are four
      different sentences, and none of them is a status code.
   ========================================================================= */

import React, { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Icon } from "./ui";
import { useAuth } from "@/lib/auth-context";

/* ------------------------------------------------------------ transport */

export type AdminResult<T> = { ok: true; data: T } | { ok: false; status: number; message: string };

export async function adminApi<T>(
  path: string,
  token: string | null,
  init: { method?: string; body?: unknown; signal?: AbortSignal } = {}
): Promise<AdminResult<T>> {
  if (!token) return { ok: false, status: 401, message: "Sign in with an administrator account." };
  let response: Response;
  try {
    response = await fetch(`/api${path}`, {
      method: init.method || "GET",
      cache: "no-store",
      signal: init.signal,
      headers: {
        accept: "application/json",
        authorization: `Bearer ${token}`,
        ...(init.body ? { "content-type": "application/json" } : {}),
      },
      ...(init.body ? { body: JSON.stringify(init.body) } : {}),
    });
  } catch {
    return { ok: false, status: 0, message: "The API is not reachable from this deployment." };
  }
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    const message =
      typeof (data as { error?: string }).error === "string"
        ? (data as { error: string }).error
        : response.status === 403
          ? "This account does not hold an admin role."
          : response.status === 401
            ? "That session has expired. Sign in again."
            : "That request could not be completed.";
    return { ok: false, status: response.status, message };
  }
  return { ok: true, data: data as T };
}

/** Loads one admin endpoint and keeps its four states apart. */
export function useAdminResource<T>(path: string, enabled = true) {
  const { token } = useAuth();
  const [state, setState] = useState<{ data: T | null; error: string; loading: boolean }>({
    data: null,
    error: "",
    loading: true,
  });

  const reload = useCallback(() => {
    if (!enabled) return;
    const controller = new AbortController();
    setState((s) => ({ ...s, loading: true }));
    void adminApi<T>(path, token, { signal: controller.signal }).then((result) => {
      if (controller.signal.aborted) return;
      setState(
        result.ok ? { data: result.data, error: "", loading: false } : { data: null, error: result.message, loading: false }
      );
    });
    return () => controller.abort();
  }, [path, token, enabled]);

  useEffect(() => reload(), [reload]);
  return { ...state, reload };
}

/* ----------------------------------------------------------------- gate */

export function AdminGate({ children }: { children: React.ReactNode }) {
  const { user, token, loading } = useAuth();

  if (loading) {
    return (
      <div className="adm-note" role="status">
        Checking your session…
      </div>
    );
  }
  if (!token) {
    return (
      <div className="adm-empty">
        <h2>Administrator sign-in required</h2>
        <p>
          The console reads and writes real platform data through the API, which authorises every request
          server-side. Sign in with an account that holds an admin role.
        </p>
        <Link className="btn btn-primary btn-sm" href="/login?next=/admin">
          Sign in
        </Link>
      </div>
    );
  }
  return (
    <>
      <p className="adm-who">
        Signed in as <b>{user?.email}</b>. Every action below is audited and re-authorised by the API.
      </p>
      {children}
    </>
  );
}

/* -------------------------------------------------------------- panels */

const number = (value: unknown) => (typeof value === "number" ? value.toLocaleString() : "—");

export function AdminOverview() {
  const stats = useAdminResource<Record<string, unknown>>("/admin/stats");
  const queue = useAdminResource<{ stats?: Record<string, number>; types?: string[]; durable?: boolean }>("/admin/queue");
  const bible = useAdminResource<Record<string, unknown>>("/admin/bible/overview");

  const statusCounts = (stats.data?.confessions_by_status || {}) as Record<string, number>;

  return (
    <>
      {stats.error && <div className="adm-note error">{stats.error}</div>}

      <div className="adm-stats">
        {[
          ["Categories", stats.data?.categories],
          ["Confessions", stats.data?.confessions],
          ["Published", stats.data?.published],
          ["Voices", stats.data?.voices],
          ["Bible translations", bible.data?.translations],
          ["Rights pending", bible.data?.pending_review],
        ].map(([label, value]) => (
          <div className="adm-stat" key={String(label)}>
            <span>{String(label)}</span>
            <b>{stats.loading && bible.loading ? "…" : number(value)}</b>
          </div>
        ))}
      </div>

      <div className="adm-grid">
        <section className="adm-card">
          <h3>Content lifecycle</h3>
          {Object.keys(statusCounts).length === 0 ? (
            <p className="small">No confessions are in flight.</p>
          ) : (
            <ul className="adm-list">
              {Object.entries(statusCounts).map(([status, count]) => (
                <li key={status}>
                  <span>{status.replace(/_/g, " ")}</span>
                  <b>{count}</b>
                </li>
              ))}
            </ul>
          )}
          <Link className="textlink" href="/admin/content">
            Open content <Icon n="arrow" s={12} />
          </Link>
        </section>

        <section className="adm-card">
          <h3>Background queue</h3>
          {queue.error ? (
            <p className="small">{queue.error}</p>
          ) : (
            <>
              <ul className="adm-list">
                {Object.entries(queue.data?.stats || {}).map(([status, count]) => (
                  <li key={status}>
                    <span>{status}</span>
                    <b>{count as number}</b>
                  </li>
                ))}
              </ul>
              <p className="small">
                {queue.data?.durable ? "Durable queue" : "In-memory queue"} · {(queue.data?.types || []).length} job
                types
              </p>
            </>
          )}
        </section>

        <section className="adm-card">
          <h3>Bible platform</h3>
          {bible.error ? (
            <p className="small">{bible.error}</p>
          ) : (
            <ul className="adm-list">
              {[
                ["Active translations", bible.data?.active],
                ["Published audio", bible.data?.published_audio],
                ["Offline packages", bible.data?.offline_packages],
                ["Failed imports · 7d", bible.data?.failed_imports],
              ].map(([label, value]) => (
                <li key={String(label)}>
                  <span>{String(label)}</span>
                  <b>{number(value)}</b>
                </li>
              ))}
            </ul>
          )}
          <Link className="textlink" href="/admin/bible">
            Bible operations <Icon n="arrow" s={12} />
          </Link>
        </section>
      </div>
    </>
  );
}

type AdminCategory = { id: string; name: string; slug: string; status: string; premium?: boolean; sort_order?: number };
type AdminConfession = { id: string; title: string; status: string; category_id: string; intensity?: number; language?: string };

export function AdminContent() {
  const { token } = useAuth();
  const categories = useAdminResource<AdminCategory[]>("/admin/categories");
  const confessions = useAdminResource<AdminConfession[]>("/admin/confessions");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState("");
  const [filter, setFilter] = useState("");

  const rows = (Array.isArray(confessions.data) ? confessions.data : []).filter((c) =>
    !filter.trim() ? true : c.title.toLowerCase().includes(filter.trim().toLowerCase()) || c.status === filter.trim()
  );

  /* The lifecycle is a forward-only graph the server enforces; an illegal
     transition answers 409 and the console shows that rather than hiding the
     option, because which transitions are legal is the server's rule. */
  const setStatus = async (id: string, status: string) => {
    setBusy(id);
    setMessage("");
    const result = await adminApi(`/admin/confessions/${encodeURIComponent(id)}`, token, {
      method: "PATCH",
      body: { status },
    });
    setBusy("");
    setMessage(result.ok ? `Moved to ${status.replace(/_/g, " ")}.` : result.message);
    if (result.ok) confessions.reload();
  };

  return (
    <>
      {message && <div className="adm-note">{message}</div>}

      <section className="adm-card">
        <h3>Categories</h3>
        {categories.error && <p className="small">{categories.error}</p>}
        {categories.loading && <p className="small">Loading…</p>}
        <div className="adm-chips">
          {(Array.isArray(categories.data) ? categories.data : []).map((c) => (
            <span className={"adm-chip" + (c.status === "published" ? " on" : "")} key={c.id}>
              {c.name}
              <em>{c.status}</em>
            </span>
          ))}
        </div>
      </section>

      <section className="adm-card">
        <div className="adm-card-head">
          <h3>Confessions</h3>
          <input
            className="adm-input"
            value={filter}
            placeholder="Filter by title or status"
            aria-label="Filter confessions"
            onChange={(e) => setFilter(e.target.value)}
          />
        </div>
        {confessions.error && <p className="small">{confessions.error}</p>}
        {confessions.loading && <p className="small">Loading…</p>}
        {!confessions.loading && rows.length === 0 && <p className="small">Nothing matches that filter.</p>}
        <div className="adm-table-wrap">
          <table className="adm-table">
            <thead>
              <tr>
                <th>Title</th>
                <th>Status</th>
                <th>Move to</th>
              </tr>
            </thead>
            <tbody>
              {rows.slice(0, 60).map((c) => (
                <tr key={c.id}>
                  <td>
                    <b>{c.title}</b>
                    <small>
                      {c.language || "en"} · intensity {c.intensity ?? "—"}
                    </small>
                  </td>
                  <td>
                    <span className="adm-status">{c.status.replace(/_/g, " ")}</span>
                  </td>
                  <td>
                    <select
                      aria-label={`Status for ${c.title}`}
                      className="adm-input"
                      value=""
                      disabled={busy === c.id}
                      onChange={(e) => e.target.value && setStatus(c.id, e.target.value)}
                    >
                      <option value="">Change…</option>
                      {["content_review", "theological_review", "audio_production", "audio_qa", "approved", "published", "archived"].map(
                        (s) => (
                          <option key={s} value={s}>
                            {s.replace(/_/g, " ")}
                          </option>
                        )
                      )}
                    </select>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {rows.length > 60 && <p className="small">Showing the first 60 of {rows.length}.</p>}
      </section>
    </>
  );
}

export function AdminModeration() {
  const { token } = useAuth();
  const queue = useAdminResource<Record<string, unknown>>("/admin/moderation/queue");
  const [message, setMessage] = useState("");
  const [notes, setNotes] = useState<Record<string, string>>({});

  /* The queue payload groups by kind; the console renders whatever groups the
     API returns rather than assuming a fixed set. */
  const groups = Object.entries(queue.data || {}).filter(([, value]) => Array.isArray(value)) as [string, Record<string, unknown>[]][];

  const decide = async (id: string, decision: string) => {
    const note = (notes[id] || "").trim();
    if (note.length < 4) {
      setMessage("A short reason is required — the decision is recorded against your account.");
      return;
    }
    const result = await adminApi(`/admin/moderation/user-confessions/${encodeURIComponent(id)}/review`, token, {
      method: "POST",
      body: { decision, note },
    });
    setMessage(result.ok ? `Recorded: ${decision}.` : result.message);
    if (result.ok) queue.reload();
  };

  return (
    <>
      {message && <div className="adm-note">{message}</div>}
      {queue.error && <div className="adm-note error">{queue.error}</div>}
      {queue.loading && <div className="adm-note">Loading the queue…</div>}

      {!queue.loading && groups.length === 0 && (
        <div className="adm-empty">
          <h2>Nothing is waiting</h2>
          <p>The moderation queue is empty. Submitted community content and editorial items appear here.</p>
        </div>
      )}

      {groups.map(([kind, items]) => (
        <section className="adm-card" key={kind}>
          <h3>{kind.replace(/_/g, " ")}</h3>
          {items.length === 0 ? (
            <p className="small">Empty.</p>
          ) : (
            items.slice(0, 25).map((item, i) => {
              const id = String(item.id ?? i);
              return (
                <div className="adm-row" key={id}>
                  <div>
                    <b>{String(item.title || item.body || item.reason || id).slice(0, 140)}</b>
                    <small>
                      {String(item.status || item.state || "pending")} · {String(item.created_at || "").slice(0, 10)}
                    </small>
                  </div>
                  {kind.includes("confession") && (
                    <div className="adm-row-actions">
                      <input
                        className="adm-input"
                        placeholder="Reason (recorded)"
                        value={notes[id] || ""}
                        aria-label={`Reason for ${id}`}
                        onChange={(e) => setNotes({ ...notes, [id]: e.target.value })}
                      />
                      <button className="btn btn-primary btn-sm" onClick={() => decide(id, "approved")}>
                        Approve
                      </button>
                      <button className="btn btn-ghost btn-sm" onClick={() => decide(id, "rejected")}>
                        Reject
                      </button>
                    </div>
                  )}
                </div>
              );
            })
          )}
        </section>
      ))}
    </>
  );
}

type AdminUser = { id: string; email: string; display_name?: string; role?: string; status?: string };

export function AdminUsers() {
  const { token } = useAuth();
  const admins = useAdminResource<AdminUser[]>("/admin/users/admins");
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("admin");
  const [message, setMessage] = useState("");

  const grant = async (method: "POST" | "DELETE") => {
    if (!email.trim()) return;
    const result = await adminApi("/admin/users/role", token, { method, body: { email: email.trim(), role } });
    setMessage(result.ok ? (method === "POST" ? `Granted ${role} to ${email}.` : `Revoked ${role} from ${email}.`) : result.message);
    if (result.ok) {
      setEmail("");
      admins.reload();
    }
  };

  return (
    <>
      {message && <div className="adm-note">{message}</div>}

      <section className="adm-card">
        <h3>Grant or revoke a role</h3>
        <p className="small">
          Roles are re-read from the database on every request, so a change takes effect immediately — including
          revocation of a session already in flight.
        </p>
        <div className="adm-row-actions">
          <input
            className="adm-input"
            type="email"
            placeholder="person@example.com"
            aria-label="Account email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          <select className="adm-input" aria-label="Role" value={role} onChange={(e) => setRole(e.target.value)}>
            {["admin", "super_admin", "audio_producer", "voice_manager"].map((r) => (
              <option key={r} value={r}>
                {r.replace(/_/g, " ")}
              </option>
            ))}
          </select>
          <button className="btn btn-primary btn-sm" onClick={() => grant("POST")}>
            Grant
          </button>
          <button className="btn btn-ghost btn-sm" onClick={() => grant("DELETE")}>
            Revoke
          </button>
        </div>
      </section>

      <section className="adm-card">
        <h3>Administrators</h3>
        {admins.error && <p className="small">{admins.error}</p>}
        {admins.loading && <p className="small">Loading…</p>}
        <div className="adm-table-wrap">
          <table className="adm-table">
            <thead>
              <tr>
                <th>Account</th>
                <th>Role</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {(Array.isArray(admins.data) ? admins.data : []).map((u) => (
                <tr key={u.id}>
                  <td>
                    <b>{u.display_name || u.email}</b>
                    <small>{u.email}</small>
                  </td>
                  <td>
                    <span className="adm-status">{(u.role || "admin").replace(/_/g, " ")}</span>
                  </td>
                  <td>{u.status || "active"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </>
  );
}

export function AdminAudit() {
  const audit = useAdminResource<Record<string, unknown>[]>("/admin/audit");
  const metrics = useAdminResource<Record<string, unknown>>("/admin/metrics");
  const entries = Array.isArray(audit.data) ? audit.data : [];

  return (
    <>
      {audit.error && <div className="adm-note error">{audit.error}</div>}

      <section className="adm-card">
        <h3>Security counters</h3>
        {metrics.error ? (
          <p className="small">{metrics.error}</p>
        ) : (
          <ul className="adm-list">
            {Object.entries(metrics.data || {})
              .filter(([, v]) => typeof v === "number")
              .slice(0, 12)
              .map(([k, v]) => (
                <li key={k}>
                  <span>{k.replace(/_/g, " ")}</span>
                  <b>{v as number}</b>
                </li>
              ))}
          </ul>
        )}
      </section>

      <section className="adm-card">
        <h3>Recent privileged actions</h3>
        {audit.loading && <p className="small">Loading…</p>}
        {!audit.loading && entries.length === 0 && <p className="small">No privileged action has been recorded yet.</p>}
        <div className="adm-table-wrap">
          <table className="adm-table">
            <thead>
              <tr>
                <th>When</th>
                <th>Actor</th>
                <th>Action</th>
                <th>Target</th>
              </tr>
            </thead>
            <tbody>
              {entries.slice(0, 80).map((entry, i) => (
                <tr key={String(entry.id ?? i)}>
                  <td>{String(entry.created_at || entry.at || "").slice(0, 19).replace("T", " ")}</td>
                  <td>{String(entry.actor_email || entry.actor_id || entry.actor || "—")}</td>
                  <td>
                    <span className="adm-status">{String(entry.action || entry.event || "—")}</span>
                  </td>
                  <td>{String(entry.target_id || entry.target || entry.entity_id || "—")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </>
  );
}
