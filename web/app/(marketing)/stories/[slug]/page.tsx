import { Stories } from "@/components/marketing";
export const metadata = { title: "Stories" };
export default async function Page({ params }: { params: Promise<{ slug: string }> }) { const { slug } = await params; return <Stories slug={slug} />; }
