import { AdminGate } from "@/components/admin";
import { BillingConsole } from "@/components/super-admin";

export const metadata = { title: "Billing" };

export default function Page() {
  return (
    <AdminGate>
      <BillingConsole />
    </AdminGate>
  );
}
