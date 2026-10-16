<script lang="ts">
	import { reveal } from '../utils/reveal';
	import { getI18n } from '../i18n/i18n.svelte';
	import { lp } from '../i18n/landing';

	const i18n = getI18n();
	const L = $derived(lp[i18n.lang].fitur);

	// Ikhtisar fitur bertema pantai. emoji: Fluent Emoji (MIT). Lihat static/img/CREDITS.txt.
	const em = (n: string) => `/img/emoji/${n}.png`;

	const dasar = [
		['water-wave', 'payments'],
		['sun-behind-small-cloud', 'dashboard'],
		['anchor', 'reconciliation'],
		['sailboat', 'payout'],
		['ring-buoy', 'refund'],
		['beach-with-umbrella', 'merchant']
	];
	const fitur = $derived(L.daftar.map((d, i) => ({ img: dasar[i][0], modul: dasar[i][1], ...d })));
	const ringkas = $derived(L.ringkas);
</script>

<section class="relative isolate overflow-hidden bg-gradient-to-b from-sky-100 via-sky-50 to-white pt-20 pb-28 lg:pt-28 lg:pb-36" aria-labelledby="fitur-pantai-judul">
	<!-- Awan & matahari dekoratif: hanya di tepi, tidak di belakang teks -->
	<div class="awan absolute top-16 left-[4%] -z-10 hidden h-10 w-32 rounded-full bg-white/90 lg:block" aria-hidden="true"></div>
	<div class="awan absolute top-36 left-[10%] -z-10 hidden h-7 w-20 rounded-full bg-white/70 [animation-delay:-6s] lg:block" aria-hidden="true"></div>
	<div class="matahari absolute top-10 right-[5%] -z-10 hidden size-28 rounded-full bg-amber-200/70 blur-2xl lg:block" aria-hidden="true"></div>
	<img src={em('sun-behind-small-cloud')} alt="" width="256" height="256" loading="lazy" class="mengapung absolute top-14 right-[6%] hidden size-20 lg:block" aria-hidden="true" />

	<div class="mx-auto max-w-6xl px-5">
		<div use:reveal class="mx-auto max-w-2xl text-center">
			<p class="inline-flex items-center gap-2 rounded-full bg-white px-3 py-1 text-xs font-extrabold tracking-[.2em] text-sky-700 uppercase shadow-sm ring-1 ring-sky-200">
				{L.tag}
			</p>
			<h2 id="fitur-pantai-judul" class="font-display mt-4 text-3xl leading-tight font-extrabold text-merah-gelap sm:text-4xl lg:text-5xl">{L.judul}</h2>
			<p class="mt-4 text-base leading-relaxed text-merah-gelap/70">{L.sub}</p>
		</div>

		<!-- Ringkasan -->
		<div use:reveal class="relative mt-12">
			<dl class="relative z-10 mx-auto grid max-w-2xl grid-cols-3 divide-x divide-sky-100 rounded-2xl bg-white px-2 py-4 shadow-lg shadow-sky-900/10 ring-1 ring-sky-100 sm:py-5">
				{#each ringkas as [n, l]}
					<div class="px-2 text-center">
						<dt class="sr-only">{l}</dt>
						<dd class="font-display text-3xl font-extrabold text-sky-700 sm:text-4xl">{n}</dd>
						<dd class="mt-0.5 text-[11px] font-semibold text-merah-gelap/60 sm:text-xs">{l}</dd>
					</div>
				{/each}
			</dl>
		</div>

		<!-- Kartu fitur -->
		<ul class="mt-14 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
			{#each fitur as f, i}
				<li use:reveal class="kartu group relative flex flex-col rounded-3xl bg-white p-6 pt-5 shadow-sm ring-1 ring-sky-900/10 transition duration-300 hover:-translate-y-1.5 hover:shadow-xl hover:shadow-sky-900/10" style="transition-delay:{(i % 3) * 70}ms">
					<span class="absolute inset-x-6 top-0 h-1 rounded-b-full bg-gradient-to-r from-sky-400 via-cyan-400 to-teal-300" aria-hidden="true"></span>
					<div class="flex items-center justify-between gap-3">
						<span class="grid size-16 place-items-center rounded-2xl bg-gradient-to-br from-sky-50 to-cyan-100 ring-1 ring-sky-100">
							<img src={em(f.img)} alt="" width="256" height="256" loading="lazy" class="size-11 transition duration-300 group-hover:-translate-y-1 group-hover:rotate-6" />
						</span>
						<span class="text-right">
							<span class="block font-display text-2xl font-extrabold text-sky-200 transition group-hover:text-sky-400">{String(i + 1).padStart(2, '0')}</span>
							<code class="font-mono text-[10px] font-bold text-sky-700/70">{f.modul}</code>
						</span>
					</div>
					<h3 class="mt-5 text-lg font-extrabold text-merah-gelap">{f.t}</h3>
					<p class="mt-2 text-sm leading-relaxed text-merah-gelap/70">{f.d}</p>
					<ul class="mt-4 space-y-2 border-t border-dashed border-sky-200 pt-4 text-sm">
						{#each f.poin as p}
							<li class="flex gap-2.5 text-merah-gelap/80">
								<svg class="mt-0.5 size-4 shrink-0 text-teal-500" fill="none" stroke="currentColor" stroke-width="3" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5 9-10" /></svg>{p}
							</li>
						{/each}
					</ul>
				</li>
			{/each}
		</ul>
	</div>
</section>

<style>
	.awan { animation: awan-geser 18s ease-in-out infinite alternate; filter: drop-shadow(18px 8px 0 rgb(255 255 255 / 0.8)); }
	@keyframes awan-geser { to { transform: translateX(40px); } }
	.mengapung { animation: apung 6s ease-in-out infinite; }
	@keyframes apung { 50% { transform: translateY(-10px) rotate(-4deg); } }
	@media (prefers-reduced-motion: reduce) {
		.awan, .mengapung { animation: none; }
	}
</style>
