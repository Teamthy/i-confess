import { AppPage } from "@/components/app";
export const metadata = { title: "Category" };
export default async function Page({ params }: { params: Promise<{ slug: string }> }) { const { slug } = await params; return <AppPage kind="category" slug={slug} />; }
