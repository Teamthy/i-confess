import { Confessions } from "@/components/marketing";
import { getLibrary } from "@/lib/library-server";
export const metadata = { title: "Confession Library" };
export default async function Page() { const lib = await getLibrary(); return <Confessions library={lib.confessions} />; }
