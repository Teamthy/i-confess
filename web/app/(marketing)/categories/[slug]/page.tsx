import { CategoryDetail } from "@/components/marketing";
import { getLibrary } from "@/lib/library-server";
export const metadata = { title: "Category" };
export default async function Page({ params }: { params: Promise<{ slug: string }> }) { const { slug } = await params; const lib = await getLibrary(); return <CategoryDetail slug={slug} library={lib.confessions} />; }
