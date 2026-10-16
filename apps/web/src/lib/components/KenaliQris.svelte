<script lang="ts">
	import { getI18n } from '../i18n/i18n.svelte';
	import { lp } from '../i18n/landing';
	// Section edukasi QRIS: ilustrasi pasar tetap gambar, seluruh teks & ornamen HTML/CSS.
	// Emoji 3D dari Fluent Emoji (Microsoft, MIT) — lihat static/img/emoji/LICENSE.txt.
	// Bagian non-teks tiap pertanyaan (gambar & warna); teksnya ada di lib/i18n/landing.ts.
	type Dasar = { img: string; hiasan: [string, string]; latar: string; kartuImg: string[] };
	const em = (n: string) => `/img/emoji/${n}.png`;

	const dasar: Dasar[] = [
		{ img: 'mobile-phone', hiasan: ['check-mark-button', 'light-bulb'], latar: 'from-merah/15 to-emas/25', kartuImg: [] },
		{ img: 'people-hugging', hiasan: ['convenience-store', 'mobile-phone'], latar: 'from-emas/30 to-gading', kartuImg: ['mobile-phone', 'convenience-store', 'globe-showing-asia-australia', 'people-hugging'] },
		{ img: 'alarm-clock', hiasan: ['sun', 'crescent-moon'], latar: 'from-emas/35 to-merah/10', kartuImg: ['sun', 'convenience-store', 'crescent-moon'] },
		{ img: 'globe-showing-asia-australia', hiasan: ['round-pushpin', 'convenience-store'], latar: 'from-merah/10 to-emas/30', kartuImg: ['convenience-store', 'mobile-phone', 'globe-showing-asia-australia'] },
		{ img: 'light-bulb', hiasan: ['check-mark-button', 'party-popper'], latar: 'from-emas/30 to-merah/10', kartuImg: ['mobile-phone', 'convenience-store'] },
		{ img: 'check-mark-button', hiasan: ['mobile-phone', 'party-popper'], latar: 'from-merah/15 to-gading', kartuImg: [] }
	];

	const i18n = getI18n();
	const L = $derived(lp[i18n.lang].kenali);
	const items = $derived(
		dasar.map((d, i) => {
			const t = L.items[i];
			return { ...d, ...t, kartu: t.kartu.length ? t.kartu.map(([jd, ds], n) => ({ img: d.kartuImg[n], t: jd, d: ds })) : undefined, langkah: t.langkah.length ? t.langkah : undefined };
		})
	);

	const ikonUnggulan = [
		'<path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6l8-3z"/><path d="m9 12 2 2 4-4"/>',
		'<path d="M13 2 4 14h7l-1 8 9-12h-7l1-8z"/>',
		'<circle cx="9" cy="8" r="3.5"/><path d="M2.5 20a6.5 6.5 0 0 1 13 0"/><path d="M16 4.6a3.5 3.5 0 0 1 0 6.8M17.5 14.2A6.5 6.5 0 0 1 21.5 20"/>'
	];

	let aktif = $state(0);
	const cur = $derived(items[aktif]);
	const pindah = (d: number) => (aktif = (aktif + d + items.length) % items.length);

	const tabs: HTMLButtonElement[] = [];
	function geser(e: KeyboardEvent) {
		const d = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
		if (!d) return;
		e.preventDefault();
		pindah(d);
		tabs[aktif]?.focus();
	}
</script>

<section id="qris" class="relative isolate scroll-mt-20 overflow-hidden bg-gading" aria-labelledby="qris-judul">
	<!-- ================= Hero edukasi ================= -->
	<div class="relative">
		<div class="foto relative aspect-[3/2] w-full lg:absolute lg:inset-y-0 lg:right-0 lg:aspect-auto lg:w-[64%]">
			<img
				src="/img/qris-pasar.webp"
				alt={L.alt}
				width="1536"
				height="1024"
				loading="lazy"
				decoding="async"
				class="size-full object-cover object-[55%_center]"
			/>
			<div class="pudar absolute inset-0" aria-hidden="true"></div>

			<ul class="kartu-unggul absolute right-4 bottom-20 hidden w-52 space-y-4 rounded-2xl border border-white/70 bg-white/85 p-5 shadow-xl shadow-merah-gelap/10 backdrop-blur-md sm:block lg:right-8 lg:bottom-32 xl:right-12">
				{#each L.unggulan as [a, b], i}
					<li class="animate-naik flex items-center gap-3" style="animation-delay:{0.4 + i * 0.12}s">
						<span class="grid size-10 shrink-0 place-items-center rounded-full bg-merah text-white shadow-md shadow-merah/30">
							<svg class="size-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24" aria-hidden="true">{@html ikonUnggulan[i]}</svg>
						</span>
						<span class="text-sm leading-tight"><b class="block font-extrabold">{a}</b><span class="text-merah-gelap/70">{b}</span></span>
					</li>
				{/each}
			</ul>
		</div>

		<div class="relative z-10 mx-auto max-w-6xl px-5 pt-2 pb-24 lg:pt-24 lg:pb-40">
			<div class="max-w-xl animate-naik">
				<p class="font-tangan -rotate-3 text-4xl leading-none font-bold text-merah sm:text-5xl">{L.tulisan}</p>
				<h2 id="qris-judul" class="font-display mt-1 text-5xl leading-[1.02] font-extrabold tracking-tight text-merah-gelap sm:text-6xl lg:text-7xl">{L.judul}</h2>
				<p class="mt-4 text-xl font-extrabold text-merah-gelap sm:text-2xl">
					{L.satuQr[0]} <span class="stabilo">{L.satuQr[1]}</span>
				</p>
				<p class="mt-5 max-w-md text-base leading-relaxed text-merah-gelap/75">
					{L.pengantar}
				</p>

				<!-- Sapuan kuas Merah Putih: baris atas di merah, baris bawah di putih. -->
				<div class="kuas font-tangan relative isolate mt-8 inline-flex -rotate-3 flex-col px-8 py-3 text-2xl leading-[1.5] font-bold sm:text-[1.7rem]">
					<svg class="absolute inset-0 -z-10 size-full drop-shadow-[0_6px_10px_rgb(74_5_16/0.18)]" viewBox="0 0 400 100" preserveAspectRatio="none" aria-hidden="true">
						<defs>
							<path id="kuas-bentuk" d="M10 8C70-2 150 4 232-1c64-4 124 0 160 7l-9 14 13 12-8 13 10 13-12 14 8 12c-46 11-120 18-200 20-66 2-124 3-178-3l9-12L4 79l10-13L2 52l12-14L3 24z" />
							<clipPath id="kuas-atas"><rect width="400" height="50" /></clipPath>
							<clipPath id="kuas-bawah"><rect y="50" width="400" height="50" /></clipPath>
						</defs>
						<use href="#kuas-bentuk" fill="#ffffff" clip-path="url(#kuas-bawah)" />
						<use href="#kuas-bentuk" fill="var(--color-merah)" clip-path="url(#kuas-atas)" />
						<path fill="var(--color-merah)" opacity=".12" d="M30 94c80 4 200 0 300-6l40-4c-30 9-120 14-200 16-60 1-110-1-140-6z" />
					</svg>
					<span class="text-white">{L.kuas[0]}</span>
					<span class="text-merah">{L.kuas[1]}</span>
				</div>
			</div>
		</div>

		<!-- Gelombang pemisah -->
		<svg class="absolute inset-x-0 bottom-0 z-20 h-14 w-full text-white sm:h-20 lg:h-28" viewBox="0 0 1440 120" preserveAspectRatio="none" aria-hidden="true">
			<path fill="currentColor" d="M0 70c180-50 360-60 560-20s420 70 620 20c110-28 190-40 260-36v86H0z" />
		</svg>
	</div>

	<!-- ================= 5W + 1H ================= -->
	<div id="qris-5w1h" class="relative scroll-mt-24 bg-white pt-8 pb-28 lg:pb-36">
		<div class="mx-auto max-w-6xl px-5">
			<!-- Judul + maskot -->
			<div class="flex flex-col-reverse items-start justify-between gap-6 sm:flex-row sm:items-end">
				<div>
					<p class="text-xl font-extrabold text-merah sm:text-2xl"><span class="stabilo">{L.pahami[0]}</span> {L.pahami[1]}</p>
					<p class="font-display text-6xl leading-none font-extrabold text-merah-gelap lg:text-7xl">
						5W <span class="text-emas">+</span> 1H
					</p>
					<p class="mt-4 max-w-xl text-base leading-relaxed text-merah-gelap/75">
						{L.pahamiSub}
					</p>
				</div>
				<div class="relative flex shrink-0 items-end gap-2 self-end sm:self-auto" aria-hidden="true">
					<div class="gelembung font-tangan relative mb-16 rounded-2xl rounded-br-sm border-2 border-merah-gelap bg-white px-4 py-2 text-xl leading-tight font-bold text-merah-gelap shadow-[4px_4px_0] shadow-merah-gelap">
						{L.gelembung[0]}<br />{L.gelembung[1]}
					</div>
					<img src={em('thinking-face')} alt="" width="256" height="256" class="maskot size-24 sm:size-28" loading="lazy" />
				</div>
			</div>

			<!-- Pilihan pertanyaan: stiker -->
			<div
				class="-mx-5 mt-10 flex snap-x gap-3 overflow-x-auto px-5 pt-3 pb-4 sm:mx-0 sm:grid sm:grid-cols-3 sm:overflow-visible sm:px-0 lg:grid-cols-6"
				role="tablist"
				aria-label={L.tabsAria}
				tabindex="-1"
				onkeydown={geser}
			>
				{#each items as it, i}
					<button
						bind:this={tabs[i]}
						type="button"
						role="tab"
						id="qris-tab-{i}"
						aria-selected={aktif === i}
						aria-controls="qris-panel"
						tabindex={aktif === i ? 0 : -1}
						onclick={() => (aktif = i)}
						class="stiker group relative flex w-36 shrink-0 snap-start flex-col items-center rounded-2xl border-2 px-3 pt-3 pb-4 text-center transition duration-300 focus-visible:ring-4 focus-visible:ring-emas/60 sm:w-auto
							{aktif === i
							? 'aktif border-merah-gelap bg-merah text-white shadow-[5px_5px_0] shadow-merah-gelap'
							: 'border-merah-gelap/15 bg-gading text-merah-gelap hover:-translate-y-1 hover:border-merah-gelap/40'}"
					>
						<span class="absolute -top-3 left-3 rounded-full px-2 py-0.5 text-[10px] font-extrabold {aktif === i ? 'bg-emas text-merah-gelap' : 'bg-white text-merah-gelap/60 ring-1 ring-merah-gelap/10'}">{i + 1}/6</span>
						<img src={em(it.img)} alt="" width="256" height="256" loading="lazy" class="size-14 transition duration-300 group-hover:scale-110 group-hover:-rotate-6" />
						<span class="mt-2 font-display text-xl font-extrabold">{it.k}</span>
						<span class="mt-0.5 text-[11px] leading-snug font-semibold {aktif === i ? 'text-white/85' : 'text-merah-gelap/60'}">{it.tanya}</span>
					</button>
				{/each}
			</div>

			<!-- Panel jawaban -->
			<div id="qris-panel" role="tabpanel" aria-labelledby="qris-tab-{aktif}" class="relative mt-6 overflow-hidden rounded-[2rem] border-2 border-merah-gelap bg-white shadow-[8px_8px_0] shadow-merah-gelap/90">
				{#key aktif}
					<div class="grid lg:grid-cols-[.85fr_1.15fr]">
						<!-- Visual -->
						<div class="relative isolate flex min-h-64 items-center justify-center overflow-hidden bg-gradient-to-br {cur.latar} p-8 lg:min-h-full">
							<div class="pola-titik absolute inset-0 -z-10" aria-hidden="true"></div>
							<span class="font-display pointer-events-none absolute -bottom-10 -left-3 -z-10 text-[11rem] leading-none font-extrabold text-white/70 select-none" aria-hidden="true">{cur.k.slice(0, 2)}</span>
							<span class="absolute top-5 left-5 rounded-full border-2 border-merah-gelap bg-white px-3 py-1 text-xs font-extrabold text-merah-gelap">{L.pertanyaan.replace('{n}', String(aktif + 1)).replace('{total}', '6')}</span>
							<div class="relative" aria-hidden="true">
								<span class="absolute inset-6 -z-10 rounded-full bg-white/80 blur-2xl"></span>
								<img src={em(cur.img)} alt="" width="256" height="256" class="muncul size-40 drop-shadow-xl sm:size-48" />
								<img src={em(cur.hiasan[0])} alt="" width="256" height="256" class="animate-terbang absolute -top-4 -right-10 size-16 rotate-12" />
								<img src={em(cur.hiasan[1])} alt="" width="256" height="256" class="animate-terbang absolute -bottom-2 -left-10 size-14 -rotate-12 [animation-delay:-3s]" />
							</div>
						</div>

						<!-- Isi -->
						<div class="naik-isi p-6 sm:p-8 lg:p-10">
							<p class="font-tangan text-2xl leading-none font-bold text-merah">{cur.k}?</p>
							<h3 class="font-display mt-1 text-3xl leading-tight font-extrabold text-merah-gelap sm:text-4xl">{cur.tanya}</h3>
							<p class="mt-4 text-[15px] leading-relaxed text-merah-gelap/80">{cur.isi}</p>

							{#if cur.fakta.length}
								<dl class="mt-5 flex flex-wrap gap-2">
									{#each cur.fakta as [l, v]}
										<div class="rounded-xl border border-merah-gelap/10 bg-gading px-3 py-2">
											<dt class="text-[10px] font-bold tracking-wider text-merah-gelap/55 uppercase">{l}</dt>
											<dd class="text-sm font-extrabold text-merah-gelap">{v}</dd>
										</div>
									{/each}
								</dl>
							{/if}

							{#if cur.kartu}
								<ul class="mt-5 grid gap-3 {cur.kartu.length === 2 ? 'sm:grid-cols-2' : cur.kartu.length === 3 ? 'sm:grid-cols-3' : 'sm:grid-cols-2'}">
									{#each cur.kartu as c}
										<li class="flex items-start gap-3 rounded-2xl border border-merah-gelap/10 bg-gading/70 p-3 transition hover:-translate-y-0.5 hover:border-merah/30 hover:bg-gading">
											<img src={em(c.img)} alt="" width="256" height="256" loading="lazy" class="size-10 shrink-0" />
											<span class="text-sm leading-snug"><b class="block font-extrabold text-merah-gelap">{c.t}</b><span class="text-merah-gelap/70">{c.d}</span></span>
										</li>
									{/each}
								</ul>
							{/if}

							{#if cur.langkah}
								<ol class="relative mt-5 space-y-3">
									<span class="absolute top-4 bottom-4 left-[17px] w-0.5 border-l-2 border-dashed border-merah/30" aria-hidden="true"></span>
									{#each cur.langkah as l, n}
										<li class="relative flex items-center gap-3">
											<span class="grid size-9 shrink-0 place-items-center rounded-full border-2 border-merah-gelap bg-emas font-display text-base font-extrabold text-merah-gelap">{n + 1}</span>
											<span class="flex-1 rounded-xl bg-gading px-3 py-2 text-sm font-semibold text-merah-gelap">{l}</span>
											{#if n === cur.langkah.length - 1}<img src={em('party-popper')} alt="" width="256" height="256" class="size-8" />{/if}
										</li>
									{/each}
								</ol>
							{/if}

							<aside class="mt-6 flex gap-3 rounded-2xl border-2 border-dashed border-emas bg-emas/10 p-4">
								<img src={em('light-bulb')} alt="" width="256" height="256" loading="lazy" class="size-9 shrink-0" />
								<p class="text-sm leading-relaxed text-merah-gelap/85"><b class="font-extrabold text-merah-gelap">{L.tahu}</b> {cur.tahu}</p>
							</aside>

							<div class="mt-7 flex flex-wrap items-center justify-between gap-4 border-t border-merah-gelap/10 pt-5">
								<div class="flex items-center gap-1.5" aria-hidden="true">
									{#each items as _, i}
										<span class="h-2 rounded-full transition-all {i === aktif ? 'w-6 bg-merah' : 'w-2 bg-merah-gelap/15'}"></span>
									{/each}
								</div>
								<div class="flex gap-2">
									<button type="button" onclick={() => pindah(-1)} class="grid size-10 place-items-center rounded-full border-2 border-merah-gelap text-merah-gelap transition hover:bg-gading focus-visible:ring-4 focus-visible:ring-emas/60" aria-label={L.prev}>
										<svg class="size-4" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" viewBox="0 0 24 24" aria-hidden="true"><path d="m15 6-6 6 6 6" /></svg>
									</button>
									<button type="button" onclick={() => pindah(1)} class="flex items-center gap-2 rounded-full border-2 border-merah-gelap bg-merah px-4 text-sm font-bold text-white shadow-[3px_3px_0] shadow-merah-gelap transition hover:translate-x-px hover:translate-y-px hover:shadow-[2px_2px_0] focus-visible:ring-4 focus-visible:ring-emas/60">
										{aktif === items.length - 1 ? L.ulangi : L.lanjut.replace('{k}', items[aktif + 1].k)}
										<svg class="size-4" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 6 6 6-6 6" /></svg>
									</button>
								</div>
							</div>
						</div>
					</div>
				{/key}
			</div>

			<a href="/register" class="mt-8 inline-flex items-center gap-2 text-sm font-bold text-merah transition hover:gap-3 hover:text-merah-tua">
				{L.cta}
				<svg class="size-4" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 12h14m-6-6 6 6-6 6" /></svg>
			</a>
		</div>
	</div>
</section>

<style>
	.font-tangan { font-family: 'Caveat', 'Segoe Script', cursive; }

	/* Ilustrasi melebur ke latar gading: bawah di mobile, kiri & bawah di desktop. */
	.pudar { background: linear-gradient(to bottom, transparent 55%, var(--color-gading) 100%); }
	@media (min-width: 1024px) {
		.pudar {
			background:
				linear-gradient(to right, var(--color-gading) 0%, color-mix(in srgb, var(--color-gading) 70%, transparent) 14%, transparent 34%),
				linear-gradient(to top, var(--color-gading) 0%, transparent 22%);
		}
	}

	/* Stabilo emas di bawah teks. */
	.stabilo {
		background: linear-gradient(transparent 58%, color-mix(in srgb, var(--color-emas) 55%, transparent) 58%, color-mix(in srgb, var(--color-emas) 55%, transparent) 92%, transparent 92%);
		padding-inline: 0.12em;
		-webkit-box-decoration-break: clone;
		box-decoration-break: clone;
	}

	.pola-titik {
		background-image: radial-gradient(color-mix(in srgb, var(--color-merah-gelap) 16%, transparent) 1.2px, transparent 1.6px);
		background-size: 18px 18px;
	}

	/* Elemen playful 5W + 1H. */
	.maskot { animation: miring 4s ease-in-out infinite; transform-origin: 50% 90%; }
	@keyframes miring { 0%, 100% { transform: rotate(-6deg); } 50% { transform: rotate(6deg) translateY(-4px); } }
	.gelembung { animation: terbang 5s ease-in-out infinite; }
	.stiker.aktif { transform: translateY(-4px) rotate(-2deg); }
	.stiker.aktif img { animation: lompat .6s cubic-bezier(.3, 1.6, .5, 1) both; }
	.muncul { animation: lompat .7s cubic-bezier(.3, 1.6, .5, 1) both, terbang 6s ease-in-out .7s infinite; }
	@keyframes lompat { from { opacity: 0; transform: scale(.4) rotate(-20deg); } to { opacity: 1; transform: none; } }
	.naik-isi > :global(*) { animation: naik .55s cubic-bezier(.2, .7, .2, 1) both; }
	.naik-isi > :global(*:nth-child(2)) { animation-delay: .05s; }
	.naik-isi > :global(*:nth-child(3)) { animation-delay: .1s; }
	.naik-isi > :global(*:nth-child(4)) { animation-delay: .15s; }
	.naik-isi > :global(*:nth-child(n + 5)) { animation-delay: .2s; }

	@media (prefers-reduced-motion: reduce) {
		.maskot, .gelembung, .muncul, .stiker.aktif img, .naik-isi > :global(*) { animation: none; }
	}
</style>
