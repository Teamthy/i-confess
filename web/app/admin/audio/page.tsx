import { AdminGate } from "@/components/admin";
import { AudioConsole } from "@/components/super-admin";

export const metadata = { title: "Audio" };

export default function Page() {
  return (
    <AdminGate>
      <AudioConsole />
    </AdminGate>
  );
}
