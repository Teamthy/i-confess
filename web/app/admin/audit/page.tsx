"use client";

import { useState } from "react";
import { AdminGate, AdminAudit, adminApi } from "@/components/admin";
import { useAuth } from "@/lib/auth-context";

export default function Page() {
  const { token } = useAuth();
  const [exporting, setExporting] = useState(false);
  const [message, setMessage] = useState("");

  const exportAudit = async (format: "json" | "csv") => {
    if (!token) return;
    setExporting(true);
    try {
      const res = await fetch(`/api/admin/audit/export?format=${format}&limit=1000`, {
        headers: { authorization: `Bearer ${token}` },
      });
      if (!res.ok) throw new Error("Export failed");
      if (format === "csv") {
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = "audit-export.csv";
        a.click();
        URL.revokeObjectURL(url);
        setMessage("CSV exported.");
      } else {
        const data = await res.json();
        const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = "audit-export.json";
        a.click();
        URL.revokeObjectURL(url);
        setMessage(`Exported ${data.count} entries as JSON.`);
      }
    } catch (e) {
      setMessage(e instanceof Error ? e.message : "Export failed");
    } finally {
      setExporting(false);
    }
  };

  return (
    <>
      <h1 className="adm-title">Audit Log</h1>
      <p className="adm-lede">
        Security counters and recent privileged actions. Every admin action is recorded with actor, role, IP, and
        result. Super admin can export logs as JSON or CSV.
      </p>
      {message && <div className="adm-note">{message}</div>}
      <div className="adm-toolbar">
        <button className="btn btn-primary btn-sm" onClick={() => exportAudit("json")} disabled={exporting}>
          Export JSON
        </button>
        <button className="btn btn-ghost btn-sm" onClick={() => exportAudit("csv")} disabled={exporting}>
          Export CSV
        </button>
        <span className="adm-pill blue">RBAC-audited · immutable</span>
      </div>
      <AdminGate>
        <AdminAudit />
      </AdminGate>
    </>
  );
}
