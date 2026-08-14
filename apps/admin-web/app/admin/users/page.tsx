"use client";

import { useEffect, useState } from "react";
import { Alert, Badge, Button, Select } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";

/** Assumed shape of GET /admin/users entries (spec §13.6). */
interface AdminUser {
  id: string;
  email?: string;
  phone?: string;
  roles?: string[];
  status?: string;
  created_at?: string;
}

// Placeholder role list until GET /admin/roles exists (spec §13.6).
const ROLE_OPTIONS = [
  "tourist",
  "guide",
  "operations",
  "verifier",
  "finance",
  "content_admin",
  "admin",
  "super_admin",
];

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

function parseUsers(data: unknown): AdminUser[] {
  if (Array.isArray(data)) return data as AdminUser[];
  if (data !== null && typeof data === "object" && "users" in data) {
    const users = (data as { users: unknown }).users;
    if (Array.isArray(users)) return users as AdminUser[];
  }
  return [];
}

function formatDate(value?: string): string {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString();
}

export default function AdminUsersPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [savingId, setSavingId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  async function load() {
    setState("loading");
    setLoadError(null);
    try {
      const data = await api<unknown>("/admin/users");
      setUsers(parseUsers(data));
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthorized");
      } else if (err instanceof ApiError && err.status === 403) {
        setState("forbidden");
      } else {
        setLoadError(errorMessage(err, "Could not load users. Please retry."));
        setState("error");
      }
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function changeRole(user: AdminUser, role: string) {
    const label = user.email ?? user.phone ?? user.id;
    // Confirmation dialog for privileged actions (spec §18.4).
    if (!window.confirm(`Set ${label}'s role to "${role}"?`)) return;
    setSavingId(user.id);
    setActionError(null);
    setNotice(null);
    try {
      await api(`/admin/users/${user.id}/roles`, {
        method: "PATCH",
        body: { roles: [role] },
      });
      setUsers((prev) =>
        prev.map((u) => (u.id === user.id ? { ...u, roles: [role] } : u)),
      );
      setNotice(`Updated ${label} to role "${role}".`);
    } catch (err) {
      if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
        setState(err.status === 401 ? "unauthorized" : "forbidden");
        return;
      }
      setActionError(errorMessage(err, `Could not update ${label}.`));
    } finally {
      setSavingId(null);
    }
  }

  if (state === "unauthorized" || state === "forbidden") {
    return (
      <div className="stack">
        <h1>Users & roles</h1>
        <Unauthorized forbidden={state === "forbidden"} />
      </div>
    );
  }

  return (
    <div className="stack">
      <section aria-labelledby="users-heading">
        <h1 id="users-heading">Users & roles</h1>
        <p className="muted">
          Manage platform accounts and their roles. Every change is audited by
          the backend.
        </p>
      </section>

      {state === "error" ? (
        <>
          <Alert tone="error" title="Something went wrong">
            <p>{loadError}</p>
          </Alert>
          <div>
            <Button type="button" onClick={() => void load()}>
              Retry
            </Button>
          </div>
        </>
      ) : null}

      {actionError ? (
        <Alert tone="error" title="Role update failed">
          <p>{actionError}</p>
        </Alert>
      ) : null}

      {notice ? (
        <Alert tone="success" title="Saved">
          <p>{notice}</p>
        </Alert>
      ) : null}

      {state === "loading" ? (
        <div className="stack" aria-busy="true" aria-label="Loading users">
          {Array.from({ length: 5 }, (_, i) => (
            <div key={i} className="gg-skeleton" style={{ height: "3rem" }} />
          ))}
        </div>
      ) : null}

      {state === "ready" && users.length === 0 ? (
        <Alert tone="info" title="No users found">
          <p>The directory is empty or the API returned no accounts.</p>
        </Alert>
      ) : null}

      {state === "ready" && users.length > 0 ? (
        <div className="gg-table-scroll">
          <table className="gg-table">
            <thead>
              <tr>
                <th scope="col">Email</th>
                <th scope="col">Roles</th>
                <th scope="col">Status</th>
                <th scope="col">Created</th>
                <th scope="col">Change role</th>
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td>{user.email ?? user.phone ?? "—"}</td>
                  <td>
                    {(user.roles ?? []).length === 0
                      ? "—"
                      : (user.roles ?? []).map((role) => (
                          <Badge key={role} tone="neutral">
                            {role}
                          </Badge>
                        ))}
                  </td>
                  <td>
                    <Badge
                      tone={user.status === "active" ? "success" : "neutral"}
                    >
                      {user.status ?? "unknown"}
                    </Badge>
                  </td>
                  <td>{formatDate(user.created_at)}</td>
                  <td>
                    <Select
                      label={`Change role for ${user.email ?? user.phone ?? user.id}`}
                      value={user.roles?.[0] ?? ""}
                      disabled={savingId === user.id}
                      onChange={(e) => void changeRole(user, e.target.value)}
                    >
                      <option value="" disabled>
                        Select role
                      </option>
                      {ROLE_OPTIONS.map((role) => (
                        <option key={role} value={role}>
                          {role}
                        </option>
                      ))}
                    </Select>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}
