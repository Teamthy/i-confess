import { ConfessionDetail } from "@/components/marketing";
import { getLibrary } from "@/lib/library-server";
export const metadata = { title: "Confession" };
export default async function Page({ params }: { params: Promise<{ slug: string }> }) { const { slug } = await params; const lib = await getLibrary(); return <ConfessionDetail slug={slug} library={lib.confessions} />; }
