<script lang="ts">
	let copied = $state('');
	async function copy(id: string, text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = id;
			setTimeout(() => (copied = ''), 1800);
		} catch {
			copied = '';
		}
	}

	// Lompat ke bagian tanpa menambah #hash di URL.
	function go(e: MouseEvent, id: string) {
		e.preventDefault();
		document.getElementById(id)?.scrollIntoView({ behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
	}

	const BASE = 'https://<host-api-erapay>/v1';
	const create = `curl -X POST ${BASE}/payments \\
  -H "Authorization: Bearer epk_sbx_<secret>" \\
  -H "Idempotency-Key: ord-1001-try1" \\
  -H "Content-Type: application/json" \\
  -d '{"merchant_reference":"ORD-1001","amount":100000,"currency":"IDR","payment_method":"QRIS"}'`;
	const created = `{ "id": "…", "merchant_reference": "ORD-1001", "amount": 100000, "status": "pending",
  "fee": { "bearer": "merchant", "mdr_fee": 700, "platform_fee": 1000, "merchant_net": 98300 },
  "qr_code": { "type": "provider_defined", "value": "00020101…" }, "expires_at": "…" }`;
	const get = `curl ${BASE}/payments/<id> -H "Authorization: Bearer epk_sbx_<secret>"`;
	const sig = `X-EraPay-Signature: t=<unix>,v1=<hex>
v1 = HMAC_SHA256(secret, "<t>." + raw_body)`;
	const node = `import crypto from "node:crypto";
export function verify(secret, header, rawBody, tol = 300) {
  const p = Object.fromEntries(header.split(",").map(s => s.trim().split("=")));
  if (Math.abs(Date.now() / 1000 - Number(p.t)) > tol) return false;
  const mac = crypto.createHmac("sha256", secret).update(\`\${p.t}.\`).update(rawBody).digest("hex");
  return p.v1?.length === mac.length && crypto.timingSafeEqual(Buffer.from(mac), Buffer.from(p.v1));
}`;

	const toc = [
		['overview', 'Ringkasan'],
		['prereq', 'Prasyarat'],
		['auth', 'Autentikasi'],
		['create', 'Buat pembayaran'],
		['qr', 'Tampilkan QR'],
		['status', 'Baca status'],
		['webhook', 'Terima webhook'],
		['machine', 'Mesin status'],
		['test', 'Uji di sandbox'],
		['checklist', 'Checklist integrasi']
	] as const;
	const states = [
		['pending', 'QR terbit, menunggu pembayar'],
		['succeeded', 'Dibayar (final)'],
		['failed', 'Ditolak gateway (final)'],
		['expired', 'QR kedaluwarsa (final)'],
		['cancelled', 'Dibatalkan (final)'],
		['unknown', 'Gateway tidak menjawab pasti — bukan gagal; jangan buat pembayaran baru']
	];
	const checklist = [
		'API key disimpan di secret manager server, tidak pernah di browser/aplikasi klien',
		'Setiap create payment mengirim Idempotency-Key; retry memakai key yang sama',
		'Pesanan ditandai lunas hanya dari status succeeded (API/webhook), bukan dari redirect pelanggan',
		'Webhook memverifikasi tanda tangan atas raw body + toleransi waktu 5 menit',
		'Webhook membalas 2xx cepat dan membuang event duplikat berdasarkan id',
		'Status unknown ditangani: tunggu webhook / baca ulang, jangan buat pembayaran baru',
		'Sudah diuji end-to-end di sandbox (epk_sbx_…) sebelum memakai key produksi'
	];
</script>

{#snippet code(id: string, label: string, text: string)}
	<div class="my-3 overflow-hidden rounded-xl border border-ink/20 bg-[#0b1020] font-mono text-xs text-slate-200">
		<div class="flex items-center justify-between border-b border-white/10 px-3 py-1.5">
			<span class="text-[11px] font-bold uppercase tracking-wide text-slate-400">{label}</span>
			<button type="button" class="rounded-md border border-white/20 px-2 py-0.5 text-[11px] font-bold hover:bg-white/10" onclick={() => copy(id, text)}>{copied === id ? 'Tersalin' : 'Salin'}</button>
		</div>
		<pre class="overflow-x-auto p-3 leading-relaxed"><code>{text}</code></pre>
	</div>
{/snippet}

<header class="mb-6 max-w-3xl">
	<h1 class="text-xl font-extrabold">Tutorial integrasi</h1>
	<p class="muted mt-1 text-sm">Panduan server-to-server untuk menyambungkan produk Anda ke EraPay. Selaras dengan backend; rujukan lengkap: <code>docs/API-INTEGRATOR.md</code>.</p>
</header>

<div class="grid gap-6 lg:grid-cols-[13rem_1fr]">
	<nav aria-label="Di halaman ini" class="card h-fit p-4 text-xs lg:sticky lg:top-4">
		<p class="muted mb-2 font-bold uppercase tracking-wide">Di halaman ini</p>
		<ol class="list-none space-y-0.5 p-0">
			{#each toc as [id, label], i (id)}
				<li><a href={`#${id}`} onclick={(e) => go(e, id)} class="flex items-baseline gap-2 rounded-md px-2 py-1.5 font-semibold hover:bg-ink/5"><span class="muted w-5 shrink-0 text-right tabular-nums">{i + 1}</span>{label}</a></li>
			{/each}
		</ol>
	</nav>

	<div class="max-w-3xl space-y-5 text-sm leading-relaxed">
		<section id="overview" class="card scroll-mt-4 p-5">
			<h2 class="mb-3 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">1 · Ringkasan</h2>
			<div class="space-y-2">
			
			<p>Produk Anda (POS/aplikasi) memanggil <strong>EraPay saja</strong>. EraPay membuat QRIS lewat gateway, lalu memberi tahu server Anda lewat webhook saat pembayar selesai atau QR kedaluwarsa.</p>
			<ul class="mt-2 list-disc space-y-1 pl-5">
				<li><strong>API</strong> — <code>/v1/*</code>, dipanggil server Anda dengan API key.</li>
				<li><strong>Webhook</strong> — EraPay → URL Anda, bertanda tangan HMAC.</li>
				<li><strong>Dashboard</strong> — untuk manusia (kelola key, webhook, payout, refund); bukan untuk runtime produk.</li>
			</ul>
			<p class="muted mt-2">Uang selalu integer rupiah. Pembayar membayar nominal transaksi tanpa biaya tambahan; <code>fee</code> adalah potongan atas merchant.</p>
		</div>
		</section>
		<section id="prereq" class="card scroll-mt-4 p-5">
			<h2 class="mb-3 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">2 · Prasyarat</h2>
			<div class="space-y-2">
			
			<ol class="list-decimal space-y-1 pl-5">
				<li>Merchant sudah <strong>aktif</strong> (onboarding/KYC disetujui).</li>
				<li>Buat API key di <a class="font-bold underline" href="/dashboard/integrasi?tab=keys">Integrasi → API Keys</a> dengan scope <code>payment:create</code>, <code>payment:read</code> (dan <code>report:read</code> bila perlu).</li>
				<li>Daftarkan URL webhook HTTPS di <a class="font-bold underline" href="/dashboard/integrasi?tab=webhooks">Integrasi → Webhook</a>; simpan secret <code>whsec_…</code>.</li>
			</ol>
			<p class="mt-3 rounded-lg border border-amber-400/60 bg-amber-400/10 p-3 text-xs"><strong>Secret tampil sekali.</strong> Salin saat dibuat/dirotasi; yang tersimpan hanya hash. Key bocor → cabut segera.</p>
		</div>
		</section>
		<section id="auth" class="card scroll-mt-4 p-5">
			<h2 class="mb-3 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">3 · Autentikasi</h2>
			<div class="space-y-2">
			
			<p>Kirim key di setiap request. Key <code>epk_sbx_…</code> hanya sandbox, <code>epk_live_…</code> hanya produksi.</p>
			{@render code('auth', 'Header', 'Authorization: Bearer epk_sbx_<secret>')}
			<p class="muted">Uang keluar (payout, refund, rekening bank) tidak bisa dengan API key — hanya lewat sesi dashboard.</p>
		</div>
		</section>
		<section id="create" class="card scroll-mt-4 p-5">
			<h2 class="mb-3 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">4 · Buat pembayaran</h2>
			<div class="space-y-2">
			
			<p><code>POST /v1/payments</code> — wajib <code>Idempotency-Key</code> (8–128 karakter). Key sama + body sama mengembalikan hasil pertama; body beda → <code>409</code>. Timeout? <strong>Ulangi dengan key yang sama.</strong></p>
			{@render code('create', 'curl', create)}
			<p>Body hanya boleh: <code>merchant_reference</code>, <code>amount</code>, <code>currency</code> (IDR), <code>payment_method</code>, <code>description</code>, <code>outlet_id</code>, <code>expires_at</code>. Fee, status, dan gateway ditentukan server.</p>
			{@render code('created', 'Respons 201', created)}
		</div>
		</section>
		<section id="qr" class="card scroll-mt-4 p-5">
			<h2 class="mb-3 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">5 · Tampilkan QR</h2>
			<div class="space-y-2">
			
			<p><code>qr_code.value</code> adalah payload QRIS mentah (bukan URL gambar). Render sebagai QR di aplikasi Anda. Selama status <code>pending</code>, QR bisa diambil lagi lewat <code>GET /v1/payments/&#123;id&#125;</code>.</p>
		</div>
		</section>
		<section id="status" class="card scroll-mt-4 p-5">
			<h2 class="mb-3 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">6 · Baca status</h2>
			<div class="space-y-2">
			
			<p><code>GET /v1/payments/&#123;id&#125;</code> (scope <code>payment:read</code>). Daftar: <code>GET /v1/payments?status=&amp;from=&amp;to=&amp;limit=&amp;cursor=</code>.</p>
			{@render code('get', 'curl', get)}
			<p class="muted">Jangan menandai pesanan lunas dari layar/redirect pelanggan; pakai <code>status == "succeeded"</code>.</p>
		</div>
		</section>
		<section id="webhook" class="card scroll-mt-4 p-5">
			<h2 class="mb-3 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">7 · Terima webhook</h2>
			<div class="space-y-2">
			
			<p>Event: <code>payment.succeeded</code>, <code>payment.failed</code>, <code>payment.expired</code>, <code>payment.cancelled</code>, <code>refund.*</code>, <code>payout.*</code>. Header: <code>X-EraPay-Signature</code>, <code>X-EraPay-Event-Id</code>, <code>X-EraPay-Delivery-Attempt</code>.</p>
			{@render code('sig', 'Tanda tangan', sig)}
			{@render code('node', 'Verifikasi (Node.js)', node)}
			<ul class="list-disc space-y-1 pl-5">
				<li>Hitung ulang atas <strong>raw body</strong>, bandingkan constant-time, tolak bila <code>t</code> lebih dari 5 menit.</li>
				<li>Balas <strong>2xx</strong> secepatnya (timeout 10 dtk). Gagal diulang hingga 8 kali lalu dead-letter; kirim ulang manual dari dashboard.</li>
				<li><strong>At-least-once</strong>: simpan <code>id</code> event dan abaikan duplikat. Urutan tidak dijamin; pakai <code>status</code> dan <code>final_at</code>.</li>
			</ul>
		</div>
		</section>
		<section id="machine" class="card scroll-mt-4 p-5">
			<h2 class="mb-3 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">8 · Mesin status</h2>
			<div class="space-y-2">
			
			<table class="w-full text-left text-xs">
				<tbody>
					{#each states as [s, d] (s)}
						<tr class="border-t border-ink/10 first:border-0"><td class="py-1.5 pr-3"><code class="font-bold">{s}</code></td><td>{d}</td></tr>
					{/each}
				</tbody>
			</table>
			<p class="muted mt-2">Status pembayaran berbeda dari <code>settlement_status</code> (dana sudah diterima EraPay) dan <code>refunded_amount</code> (refund tidak mengubah <code>status</code>).</p>
		</div>
		</section>
		<section id="test" class="card scroll-mt-4 p-5">
			<h2 class="mb-3 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">9 · Uji di sandbox</h2>
			<div class="space-y-2">
			
			<ol class="list-decimal space-y-1 pl-5">
				<li>Buat key <code>epk_sbx_…</code> dan webhook ke URL uji Anda.</li>
				<li>Buat pembayaran (langkah 4), tampilkan QR, bayar lewat simulator gateway sandbox.</li>
				<li>Pastikan webhook <code>payment.succeeded</code> tiba dan tanda tangan lolos; cek juga di <a class="font-bold underline" href="/dashboard/transactions">Transaksi</a>.</li>
				<li>Ulangi request yang sama dengan Idempotency-Key sama: tidak boleh membuat pembayaran kedua.</li>
			</ol>
			<p class="muted mt-2">Payout di luar produksi adalah dry run — tidak ada transfer nyata.</p>
		</div>
		</section>
		<section id="checklist" class="card scroll-mt-4 p-5">
			<h2 class="mb-3 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">10 · Checklist integrasi</h2>
			<div class="space-y-2">
			
			<ul class="space-y-1.5">
				{#each checklist as c (c)}<li class="flex gap-2"><span aria-hidden="true">☐</span><span>{c}</span></li>{/each}
			</ul>
		</div>
		</section>
	</div>
</div>
