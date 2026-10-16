<script lang="ts">
	import { reveal } from '../utils/reveal';
	import { formatIDR } from '../utils/money';
	import { getI18n } from '../i18n/i18n.svelte';
	import { lp } from '../i18n/landing';

	const i18n = getI18n();
	const L = $derived(lp[i18n.lang].tentang);

	// Contoh jurnal satu pembayaran QRIS Rp100.000 (tarif UMKM: MDR 0,7%, Biaya Jasa 0,3%). Ilustrasi, bukan data nyata.
	const angka = [[99_300, 0], [0, 99_000], [0, 300]];
	const jurnal = $derived(angka.map(([debit, kredit], i) => ({ akun: L.ledger.akunJurnal[i], debit, kredit })));
	const totalD = angka.reduce((s, j) => s + j[0], 0);
	const totalK = angka.reduce((s, j) => s + j[1], 0);

	const status = [
		['created', 'netral'],
		['pending', 'tunggu'],
		['succeeded', 'ok'],
		['failed', 'gagal'],
		['expired', 'netral'],
		['cancelled', 'netral'],
		['unknown', 'tunggu']
	];
	const warnaStatus: Record<string, string> = {
		netral: 'bg-merah-gelap/5 text-merah-gelap/70 ring-merah-gelap/10',
		tunggu: 'bg-amber-50 text-amber-800 ring-amber-200',
		ok: 'bg-green-50 text-green-800 ring-green-200',
		gagal: 'bg-red-50 text-red-800 ring-red-200'
	};

</script>

<section id="fitur" class="relative scroll-mt-20 overflow-hidden bg-white py-20 lg:py-28">
	<div class="grid-teknis absolute inset-0 -z-0" aria-hidden="true"></div>
	<div class="relative mx-auto max-w-6xl px-5">
		<div use:reveal class="grid gap-6 lg:grid-cols-[1.2fr_.8fr] lg:items-end">
			<div>
				<p class="inline-flex items-center gap-2 rounded-full border border-merah-gelap/10 bg-gading px-3 py-1 font-mono text-[11px] font-bold tracking-wider text-merah uppercase">
					<span class="size-1.5 rounded-full bg-merah"></span> {L.tag}
				</p>
				<h2 class="font-display mt-4 text-3xl leading-tight font-extrabold text-merah-gelap sm:text-4xl lg:text-5xl">
					{L.judul[0]}<span class="text-merah">{L.judul[1]}</span>{L.judul[2]}
				</h2>
			</div>
			<p class="text-base leading-relaxed text-merah-gelap/70">
				{L.sub}
			</p>
		</div>

		<!-- Bento -->
		<div class="mt-12 grid gap-4 lg:grid-cols-6">
			<!-- Ledger -->
			<article use:reveal class="kartu-gelap relative overflow-hidden rounded-3xl bg-merah-gelap p-6 text-gading sm:p-8 lg:col-span-4">
				<div class="flex flex-wrap items-start justify-between gap-4">
					<div>
						<p class="font-mono text-[11px] font-bold tracking-wider text-emas uppercase">{L.ledger.tag}</p>
						<h3 class="mt-2 text-2xl font-extrabold">{L.ledger.judul}</h3>
						<p class="mt-2 max-w-md text-sm leading-relaxed text-gading/70">
							{L.ledger.isi}
						</p>
					</div>
					<span class="inline-flex items-center gap-1.5 rounded-full bg-green-500/15 px-3 py-1 text-xs font-bold text-green-300 ring-1 ring-green-400/30">
						<svg class="size-3.5" fill="none" stroke="currentColor" stroke-width="3" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5 9-10" /></svg>
						Σ {L.ledger.debit} = Σ {L.ledger.kredit}
					</span>
				</div>

				<div class="mt-6 overflow-x-auto rounded-2xl border border-white/10 bg-black/20">
					<table class="w-full min-w-[26rem] font-mono text-xs">
						<caption class="border-b border-white/10 px-4 py-2.5 text-left text-[11px] text-gading/55">
							{L.ledger.jurnal} {formatIDR(100_000)} <span class="text-gading/35">{L.ledger.ilustrasi}</span>
						</caption>
						<thead class="text-[10px] tracking-wider text-gading/45 uppercase">
							<tr><th class="px-4 py-2 text-left font-semibold">{L.ledger.akun}</th><th class="px-4 py-2 text-right font-semibold">{L.ledger.debit}</th><th class="px-4 py-2 text-right font-semibold">{L.ledger.kredit}</th></tr>
						</thead>
						<tbody class="divide-y divide-white/5">
							{#each jurnal as j, i}
								<tr class="baris" style="animation-delay:{i * 0.15}s">
									<td class="px-4 py-2.5 text-gading/90">{j.akun}</td>
									<td class="px-4 py-2.5 text-right tabular-nums {j.debit ? 'text-emas' : 'text-gading/25'}">{j.debit ? j.debit.toLocaleString('id-ID') : '—'}</td>
									<td class="px-4 py-2.5 text-right tabular-nums {j.kredit ? 'text-emas' : 'text-gading/25'}">{j.kredit ? j.kredit.toLocaleString('id-ID') : '—'}</td>
								</tr>
							{/each}
						</tbody>
						<tfoot class="border-t border-white/15 font-bold">
							<tr><td class="px-4 py-2.5 text-gading/60">{L.ledger.total}</td><td class="px-4 py-2.5 text-right tabular-nums">{totalD.toLocaleString('id-ID')}</td><td class="px-4 py-2.5 text-right tabular-nums">{totalK.toLocaleString('id-ID')}</td></tr>
						</tfoot>
					</table>
				</div>
				<p class="mt-3 font-mono text-[11px] text-gading/45">MDR {formatIDR(700)} {L.ledger.mdr}</p>
			</article>

			<!-- Keamanan -->
			<article use:reveal class="rounded-3xl border border-merah-gelap/10 bg-white p-6 shadow-sm sm:p-8 lg:col-span-2">
				<span class="grid size-11 place-items-center rounded-xl bg-merah text-white">
					<svg class="size-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="10" width="16" height="11" rx="2" /><path d="M8 10V7a4 4 0 0 1 8 0v3" /><path d="M12 14v3" /></svg>
				</span>
				<p class="mt-5 font-mono text-[11px] font-bold tracking-wider text-merah uppercase">{L.aman.tag}</p>
				<h3 class="mt-1 text-xl font-extrabold text-merah-gelap">{L.aman.judul}</h3>
				<ul class="mt-4 space-y-2.5">
					{#each L.aman.daftar as [k, d]}
						<li class="flex gap-2.5 text-sm">
							<svg class="mt-0.5 size-4 shrink-0 text-green-600" fill="none" stroke="currentColor" stroke-width="3" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5 9-10" /></svg>
							<span><code class="font-mono text-xs font-bold text-merah-gelap">{k}</code><span class="block text-xs text-merah-gelap/60">{d}</span></span>
						</li>
					{/each}
				</ul>
			</article>

			<!-- Idempotency -->
			<article use:reveal class="rounded-3xl border border-merah-gelap/10 bg-gading p-6 sm:p-8 lg:col-span-2">
				<p class="font-mono text-[11px] font-bold tracking-wider text-merah uppercase">{L.idem.tag}</p>
				<h3 class="mt-1 text-xl font-extrabold text-merah-gelap">{L.idem.judul}</h3>
				<p class="mt-2 text-sm leading-relaxed text-merah-gelap/65">{L.idem.isi}</p>
				<div class="mt-5 space-y-2 font-mono text-[11px]">
					<div class="rounded-xl bg-white p-3 ring-1 ring-merah-gelap/10">
						<span class="font-bold text-merah">POST</span> /v1/payments<br />
						<span class="text-merah-gelap/50">Idempotency-Key: 7c1f…e92a</span>
						<div class="mt-1.5 flex items-center gap-2"><span class="rounded bg-green-100 px-1.5 font-bold text-green-800">201</span><span class="text-merah-gelap/70">pay_8Hq2…</span></div>
					</div>
					<div class="rounded-xl bg-white p-3 ring-1 ring-merah-gelap/10">
						<span class="font-bold text-merah">POST</span> /v1/payments <span class="text-merah-gelap/40">· {L.idem.retry}</span><br />
						<span class="text-merah-gelap/50">Idempotency-Key: 7c1f…e92a</span>
						<div class="mt-1.5 flex items-center gap-2"><span class="rounded bg-green-100 px-1.5 font-bold text-green-800">200</span><span class="text-merah-gelap/70">pay_8Hq2… <span class="text-merah-gelap/40">{L.idem.sama}</span></span></div>
					</div>
				</div>
			</article>

			<!-- State machine -->
			<article use:reveal class="rounded-3xl border border-merah-gelap/10 bg-white p-6 shadow-sm sm:p-8 lg:col-span-2">
				<p class="font-mono text-[11px] font-bold tracking-wider text-merah uppercase">{L.state.tag}</p>
				<h3 class="mt-1 text-xl font-extrabold text-merah-gelap">{L.state.judul}</h3>
				<div class="mt-5 flex flex-wrap gap-1.5">
					{#each status as [s, w]}
						<code class="rounded-md px-2 py-1 font-mono text-[11px] font-bold ring-1 {warnaStatus[w]}">{s}</code>
					{/each}
				</div>
				<p class="mt-4 text-sm leading-relaxed text-merah-gelap/65">
					{L.state.isi1[0]}<code class="font-mono text-xs font-bold text-amber-800">unknown</code>{L.state.isi1[1]}
				</p>
				<p class="mt-3 text-xs text-merah-gelap/50">{L.state.ket}</p>
			</article>

			<!-- Adapter -->
			<article use:reveal class="rounded-3xl border border-merah-gelap/10 bg-white p-6 shadow-sm sm:p-8 lg:col-span-2">
				<p class="font-mono text-[11px] font-bold tracking-wider text-merah uppercase">{L.adapter.tag}</p>
				<h3 class="mt-1 text-xl font-extrabold text-merah-gelap">{L.adapter.judul}</h3>
				<div class="mt-5 space-y-1.5 font-mono text-[11px] font-bold" aria-label={L.adapter.aria}>
					{#each L.adapter.lapis as l, i}
						{@const c = ['bg-gading text-merah-gelap', 'bg-merah text-white', 'bg-merah-gelap text-gading', 'bg-gading text-merah-gelap'][i]}
						{#if i}<div class="ml-5 h-3 border-l-2 border-dashed border-merah/40" aria-hidden="true"></div>{/if}
						<div class="rounded-lg px-3 py-2 {c}">{l}</div>
					{/each}
				</div>
				<p class="mt-4 text-sm leading-relaxed text-merah-gelap/65">{L.adapter.isi}</p>
			</article>
		</div>

		<!-- Stack -->
		<div use:reveal class="mt-8 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-2xl border border-merah-gelap/10 bg-white/80 px-5 py-4 backdrop-blur">
			<span class="font-mono text-[11px] font-bold tracking-wider text-merah-gelap/50 uppercase">{L.stack.label}</span>
			{#each L.stack.daftar as s}
				<span class="rounded-full bg-gading px-3 py-1 font-mono text-xs font-semibold text-merah-gelap ring-1 ring-merah-gelap/10">{s}</span>
			{/each}
		</div>
	</div>
</section>

<style>
	.grid-teknis {
		background-image:
			linear-gradient(color-mix(in srgb, var(--color-merah-gelap) 5%, transparent) 1px, transparent 1px),
			linear-gradient(90deg, color-mix(in srgb, var(--color-merah-gelap) 5%, transparent) 1px, transparent 1px);
		background-size: 40px 40px;
		mask-image: radial-gradient(ellipse 80% 60% at 50% 0%, #000, transparent 75%);
	}
	.kartu-gelap::before {
		content: '';
		position: absolute;
		inset: -40% -20% auto auto;
		width: 22rem;
		height: 22rem;
		border-radius: 9999px;
		background: radial-gradient(var(--color-merah) 0%, transparent 65%);
		opacity: 0.45;
		pointer-events: none;
	}
	.baris { animation: baris 0.6s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	@keyframes baris { from { opacity: 0; transform: translateX(-10px); } to { opacity: 1; transform: none; } }
	@media (prefers-reduced-motion: reduce) { .baris { animation: none; } }
</style>
