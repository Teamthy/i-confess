import { AdminGate } from "@/components/admin";
import { ContentConsole } from "@/components/super-admin";

export const metadata = { title: "Content" };

export default function Page() {
  return (
    <AdminGate>
      <ContentConsole />
    </AdminGate>
  );
}
