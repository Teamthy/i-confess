import { CategoryDetail } from "@/components/marketing";
export const metadata = { title: "Category" };
export default async function Page({ params }: { params: Promise<{ slug: string }> }) { const { slug } = await params; return <CategoryDetail slug={slug} />; }
