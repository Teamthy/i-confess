import { AdminGate, AdminModeration } from "@/components/admin";

export const metadata = { title: "Moderation" };

export default function Page() {
  return (
    <>
      <h1 className="adm-title">Moderation</h1>
      <p className="adm-lede">
        Community submissions and editorial items awaiting a decision. Every decision is recorded against your
        account with the reason you give.
      </p>
      <AdminGate>
        <AdminModeration />
      </AdminGate>
    </>
  );
}
