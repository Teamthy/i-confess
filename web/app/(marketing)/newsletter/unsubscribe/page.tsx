import { NewsletterUnsubscribe } from "@/components/islands";
export const metadata = { title: "Unsubscribe", robots: { index: false } };
export default function Page() {
  return (
    <section className="section container" style={{ maxWidth: 640, textAlign: "center" }}>
      <NewsletterUnsubscribe />
    </section>
  );
}
