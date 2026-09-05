// Shareable template — https://iconfess.app/t/{token}
// Public if template is_public, otherwise 403.

type Params = { params: { token: string } };

export const metadata = {
  title: "Shared session — I CONFESS",
};

export default async function SharedTemplatePage({ params }: Params) {
  const token = params.token;
  // In production, fetch from API: GET /v1/t/{token}
  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
  let template: any = null;
  let error = "";
  try {
    const res = await fetch(`${api}/v1/t/${token}`, { cache: "no-store" });
    if (res.ok) template = await res.json();
    else error = `Template not found or private (${res.status})`;
  } catch (e: any) {
    error = "Unable to load template";
  }

  if (error || !template) {
    return (
      <main className="min-h-screen bg-[#0f1220] text-[#e8eaf6] flex items-center justify-center px-6">
        <div className="max-w-xl w-full bg-[#1c2138] border border-[#2d3350] rounded-2xl p-8 text-center">
          <p className="text-[#9aa1c0] text-sm">{error || "Not found"}</p>
          <a href="/" className="text-[#7c8cf8] underline text-sm mt-4 inline-block">
            Back to I CONFESS
          </a>
        </div>
      </main>
    );
  }

  const deepLink = `iconfess://t/${token}`;
  return (
    <main className="min-h-screen bg-[#0f1220] text-[#e8eaf6] flex items-center justify-center px-6">
      <div className="max-w-xl w-full bg-[#1c2138] border border-[#2d3350] rounded-2xl p-8">
        <p className="text-[11px] tracking-[0.14em] uppercase text-[#9aa1c0]">Shared session</p>
        <h1 className="font-serif text-2xl mt-2">{template.name}</h1>
        {template.description && <p className="text-[#9aa1c0] mt-2 text-sm">{template.description}</p>}
        <p className="text-xs text-[#6b7280] mt-3">
          {template.category_ids?.join(" • ")} {template.voice_id ? `· ${template.voice_id}` : ""}
        </p>
        <div className="mt-6 flex flex-col gap-3">
          <a href={deepLink} className="bg-[#7c8cf8] text-[#0b0e1c] font-semibold rounded-lg px-5 py-3 text-sm text-center">
            Open in I CONFESS
          </a>
          <a
            href={`${api}/v1/templates/${template.id}/start`}
            className="border border-[#2d3350] rounded-lg px-5 py-3 text-sm text-center"
          >
            Preview session
          </a>
        </div>
        <script dangerouslySetInnerHTML={{ __html: `setTimeout(function(){ window.location.href=${JSON.stringify(deepLink)}; }, 600);` }} />
      </div>
    </main>
  );
}
