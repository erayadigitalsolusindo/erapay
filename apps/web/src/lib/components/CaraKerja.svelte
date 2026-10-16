<script lang="ts">
	import { reveal } from '../utils/reveal';
	import { getI18n } from '../i18n/i18n.svelte';
	import { lp } from '../i18n/landing';

	const i18n = getI18n();
	const L = $derived(lp[i18n.lang].cara);

	// Alur satu pembayaran QRIS Rp25.000 dari POS sampai payout. Kode & log ilustrasi (sandbox), bukan data nyata.
	// Teks tiap tahap ada di lib/i18n/landing.ts; di sini hanya node diagram yang menyala per tahap.
	const nodeAktif = [[0, 1], [2], [2, 1], [3, 0], [3, 4]];
	const langkah = $derived(L.langkah.map((l, i) => ({ ...l, node: nodeAktif[i], judul: ['POST /v1/payments', 'payment pay_8Hq2… · log', 'inbound webhook · verifier', 'postgres · outbox worker', 'reconciliation · payout'][i] })));

	const warna: Record<string, string> = {
		k: 'text-emas font-bold',
		m: 'text-gading/55',
		ok: 'text-green-300',
		w: 'text-amber-300',
		e: 'text-white',
		c: 'text-gading/35 italic'
	};

	const DURASI = 6000;
	let aktif = $state(0);
	let jeda = $state(false);
	let kunci = $state(0); // memicu ulang animasi progres

	function pilih(i: number) {
		aktif = i;
		kunci++;
	}

	$effect(() => {
		if (jeda || matchMedia('(prefers-reduced-motion: reduce)').matches) return;
		kunci;
		const t = setTimeout(() => pilih((aktif + 1) % langkah.length), DURASI);
		return () => clearTimeout(t);
	});

	const cur = $derived(langkah[aktif]);
</script>

<section id="cara-kerja" class="relative isolate scroll-mt-20 overflow-hidden bg-merah-gelap py-20 text-gading lg:py-28">
	<div class="blueprint absolute inset-0 -z-10" aria-hidden="true"></div>
	<div class="absolute -top-40 left-1/2 -z-10 size-[40rem] -translate-x-1/2 rounded-full bg-merah/30 blur-3xl" aria-hidden="true"></div>

	<div class="mx-auto max-w-6xl px-5">
		<div use:reveal class="flex flex-col justify-between gap-6 lg:flex-row lg:items-end">
			<div class="max-w-2xl">
				<p class="inline-flex items-center gap-2 rounded-full border border-white/15 bg-white/5 px-3 py-1 font-mono text-[11px] font-bold tracking-wider text-emas uppercase">
					<span class="size-1.5 animate-pulse rounded-full bg-green-400"></span> {L.tag}
				</p>
				<h2 class="font-display mt-4 text-3xl leading-tight font-extrabold sm:text-4xl lg:text-5xl">{L.judul}</h2>
				<p class="mt-4 max-w-xl text-base leading-relaxed text-gading/70">{L.sub}</p>
			</div>
			<a href="/register" class="w-fit shrink-0 rounded-full bg-emas px-7 py-3.5 text-sm font-bold text-merah-gelap transition hover:bg-white focus-visible:ring-4 focus-visible:ring-emas/60">{L.daftar}</a>
		</div>

		<!-- Diagram alur -->
		<ol use:reveal class="mt-12 grid grid-cols-5 gap-2 sm:gap-3" aria-label={L.nodeAria}>
			{#each L.node as n, i}
				<li class="relative">
					{#if i}
						<span class="alur absolute top-1/2 -left-2 h-0.5 w-2 -translate-y-1/2 sm:-left-3 sm:w-3 {cur.node.includes(i) && cur.node.includes(i - 1) ? 'nyala' : ''}" aria-hidden="true"></span>
					{/if}
					<div
						class="rounded-xl border px-1.5 py-3 text-center font-mono text-[10px] leading-tight font-bold transition duration-500 sm:px-3 sm:text-xs
						{cur.node.includes(i) ? 'border-emas bg-emas/15 text-emas shadow-[0_0_24px] shadow-emas/25' : 'border-white/10 bg-white/5 text-gading/45'}"
					>
						{n}
					</div>
				</li>
			{/each}
		</ol>

		<div class="mt-8 grid gap-6 lg:grid-cols-[.9fr_1.1fr]">
			<!-- Stepper -->
			<div
				use:reveal
				class="space-y-2"
				role="tablist"
				aria-label={L.tabAria}
				tabindex="-1"
				onmouseenter={() => (jeda = true)}
				onmouseleave={() => (jeda = false)}
				onfocusin={() => (jeda = true)}
				onfocusout={() => (jeda = false)}
			>
				{#each langkah as l, i}
					<button
						type="button"
						role="tab"
						id="ck-tab-{i}"
						aria-selected={aktif === i}
						aria-controls="ck-panel"
						onclick={() => pilih(i)}
						class="relative w-full overflow-hidden rounded-2xl border p-4 text-left transition focus-visible:ring-4 focus-visible:ring-emas/60
						{aktif === i ? 'border-white/20 bg-white/10' : 'border-transparent hover:bg-white/5'}"
					>
						<div class="flex items-start gap-4">
							<span class="grid size-9 shrink-0 place-items-center rounded-lg font-mono text-sm font-bold transition {aktif === i ? 'bg-emas text-merah-gelap' : 'bg-white/10 text-gading/60'}">{String(i + 1).padStart(2, '0')}</span>
							<div class="min-w-0">
								<h3 class="font-extrabold {aktif === i ? 'text-white' : 'text-gading/75'}">{l.t}</h3>
								{#if aktif === i}
									<p class="mt-1 text-sm leading-relaxed text-gading/70">{l.d}</p>
									<div class="mt-3 flex flex-wrap gap-1.5">
										{#each l.tag as g}<span class="rounded-md bg-black/25 px-2 py-0.5 font-mono text-[10px] font-semibold text-emas ring-1 ring-emas/20">{g}</span>{/each}
									</div>
								{/if}
							</div>
						</div>
						{#if aktif === i}
							{#key kunci}
								<span class="progres absolute inset-x-0 bottom-0 h-0.5 origin-left bg-emas {jeda ? 'diam' : ''}" style="--durasi:{DURASI}ms" aria-hidden="true"></span>
							{/key}
						{/if}
					</button>
				{/each}
			</div>

			<!-- Terminal -->
			<div use:reveal id="ck-panel" role="tabpanel" aria-labelledby="ck-tab-{aktif}" class="flex min-h-[26rem] flex-col overflow-hidden rounded-2xl border border-white/10 bg-[#1d0207] shadow-2xl shadow-black/40">
				<div class="flex items-center gap-3 border-b border-white/10 bg-white/5 px-4 py-3">
					<span class="flex gap-1.5" aria-hidden="true"><i class="size-3 rounded-full bg-[#ff5f57]"></i><i class="size-3 rounded-full bg-[#febc2e]"></i><i class="size-3 rounded-full bg-[#28c840]"></i></span>
					<span class="truncate font-mono text-xs text-gading/50">{cur.judul}</span>
					<span class="ml-auto shrink-0 rounded bg-emas/15 px-2 py-0.5 font-mono text-[10px] font-bold text-emas">sandbox</span>
				</div>
				{#key aktif}
					<pre class="flex-1 overflow-x-auto p-5 font-mono text-[12px] leading-[1.75]"><code>{#each cur.kode as [teks, w], i}<span class="ketik block {warna[w ?? 'e'] ?? ''}" style="animation-delay:{i * 0.07}s">{teks || ' '}</span>{/each}<span class="kursor ketik inline-block h-4 w-2 translate-y-0.5 bg-emas" style="animation-delay:{cur.kode.length * 0.07}s"></span></code></pre>
				{/key}
				<p class="border-t border-white/10 px-5 py-2.5 font-mono text-[10px] text-gading/35">{L.contoh}</p>
			</div>
		</div>
	</div>
</section>

<style>
	.blueprint {
		background-image:
			linear-gradient(rgb(255 255 255 / 0.045) 1px, transparent 1px),
			linear-gradient(90deg, rgb(255 255 255 / 0.045) 1px, transparent 1px);
		background-size: 32px 32px;
		mask-image: linear-gradient(to bottom, #000, transparent 90%);
	}
	.alur { background: rgb(255 255 255 / 0.15); }
	.alur.nyala {
		background: repeating-linear-gradient(90deg, var(--color-emas) 0 4px, transparent 4px 7px);
		background-size: 14px 100%;
		animation: aliran 0.6s linear infinite;
	}
	@keyframes aliran { to { background-position: 14px 0; } }
	.progres { animation: progres var(--durasi) linear both; }
	.progres.diam { animation-play-state: paused; }
	@keyframes progres { from { transform: scaleX(0); } to { transform: scaleX(1); } }
	.ketik { animation: ketik 0.35s ease-out both; }
	@keyframes ketik { from { opacity: 0; transform: translateX(-6px); } to { opacity: 1; transform: none; } }
	.kursor { animation: ketik 0.35s ease-out both, kedip 1s steps(1) infinite; }
	@keyframes kedip { 50% { opacity: 0; } }
	@media (prefers-reduced-motion: reduce) {
		.alur.nyala, .progres, .ketik, .kursor { animation: none; }
	}
</style>
