import { AdminGate, AdminQueue } from "@/components/admin";

export const metadata = { title: "Queue" };

export default function Page() {
  return (
    <>
      <h1 className="adm-title">Queue</h1>
      <p className="adm-lede">
        What the background queue is holding: counts by status, the dead-letter release for jobs that gave up, and the
        job types this process runs. Parking a job is only useful if a human can release it.
      </p>
      <AdminGate>
        <AdminQueue />
      </AdminGate>
    </>
  );
}
