<script lang="ts">
	import '../app.css';
	import { formatIDR } from '../lib/utils/money';
	import KenaliQris from '../lib/components/KenaliQris.svelte';
	import TentangEraPay from '../lib/components/TentangEraPay.svelte';
	import CaraKerja from '../lib/components/CaraKerja.svelte';
	import FiturPantai from '../lib/components/FiturPantai.svelte';
	import Ombak from '../lib/components/Ombak.svelte';
	import DasarLaut from '../lib/components/DasarLaut.svelte';
	import { reveal } from '../lib/utils/reveal';
	import { createI18n, setI18n } from '../lib/i18n/i18n.svelte';
	import { lp } from '../lib/i18n/landing';
	import LangSwitch from '../lib/components/LangSwitch.svelte';

	let { data } = $props();
	// svelte-ignore state_referenced_locally
	const i18n = createI18n(data.lang);
	setI18n(i18n);
	const L = $derived(lp[i18n.lang]);
	$effect(() => {
		document.documentElement.lang = i18n.lang;
	});

	// ---------- Data ----------
	const navHref = ['#qris', '#fitur', '#cara-kerja'];

	// ---------- Kalkulator hero ----------
	// Semua hitungan integer (basis point). Tarif default sesuai pricing; tarif final mengikuti kontrak.
	let mulai = $state(100_000);
	const mdrBps = 70;
	const jasaBps = 30;
	const potong = (g: number, bps: number) => Math.round((g * bps) / 10_000);
	const hMdr = $derived(potong(mulai, mdrBps));
	const hJasa = $derived(potong(mulai, jasaBps));
	const hNet = $derived(mulai - hMdr - hJasa);

	// Input nominal bertitik ribuan (xxx.xxx.xxx), maksimal 9 digit. Kursor dijaga setelah format ulang.
	const NOMINAL_MAKS = 999_999_999;
	const ribuan = (n: number) => (n ? n.toLocaleString('id-ID') : '');
	function ketikNominal(e: Event & { currentTarget: HTMLInputElement }) {
		const el = e.currentTarget;
		const digitSebelumKursor = el.value.slice(0, el.selectionStart ?? el.value.length).replace(/\D/g, '').length;
		const digit = el.value.replace(/\D/g, '').replace(/^0+/, '').slice(0, 9);
		mulai = Math.min(NOMINAL_MAKS, Number(digit || 0));
		el.value = ribuan(mulai);
		let pos = 0;
		for (let d = 0; pos < el.value.length && d < Math.min(digitSebelumKursor, digit.length); pos++) if (/\d/.test(el.value[pos])) d++;
		el.setSelectionRange(pos, pos);
	}

	// Headline: frasa pertama tetap, frasa sesudahnya berganti dengan efek berbeda.
	const efek = ['e-naik', 'e-ketik', 'e-putar', 'e-zoom', 'e-geser'];
	const frasa = $derived(L.hero.frasa);
	let kata = $state(0);
	// Indeks huruf berurutan lintas baris (untuk jeda animasi ketik).
	const posisi = (bi: number, wi: number, ci: number) =>
		frasa[kata].slice(0, bi).join('').length + frasa[kata][bi].split(' ').slice(0, wi).join(' ').length + (wi ? 1 : 0) + ci;
	$effect(() => {
		const t = setInterval(() => (kata = (kata + 1) % frasa.length), 3200);
		return () => clearInterval(t);
	});

	let menu = $state(false);

	// Anchor internal: gulir halus ke section tanpa menambah #hash di URL.
	function gulir(e: MouseEvent) {
		const href = (e.currentTarget as HTMLAnchorElement).getAttribute('href');
		const el = href?.startsWith('#') ? document.querySelector(href) : null;
		if (!el) return;
		e.preventDefault();
		menu = false;
		el.scrollIntoView({ behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth', block: 'start' });
		if (location.hash) history.replaceState(null, '', location.pathname + location.search);
	}
</script>

<svelte:head>
	<title>{L.meta.title}</title>
	<meta name="description" content={L.meta.desc} />
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
	<link
		href="https://fonts.googleapis.com/css2?family=Caveat:wght@600;700&family=Dela+Gothic+One&family=Lilita+One&family=Fraunces:opsz,wght@9..144,600;9..144,800&family=Plus+Jakarta+Sans:wght@400;500;600;700;800&display=swap"
		rel="stylesheet"
	/>
</svelte:head>

<div class="lp font-sans min-h-screen overflow-x-hidden bg-white text-merah-gelap antialiased">
	<!-- ================= Header ================= -->
	<header class="sticky top-0 z-50 border-b border-white/10 bg-merah-gelap/95 text-gading backdrop-blur">
		<div class="mx-auto flex h-[72px] max-w-6xl items-center justify-between gap-4 px-5">
			<a href="/" class="flex items-center" aria-label="EraPay">
				<img src="/erapay-logo-outline.svg" alt="EraPay" class="h-9 w-auto" draggable="false" />
			</a>

			<nav class="hidden items-center gap-8 text-sm font-semibold md:flex" aria-label={L.hdr.navAria}>
				{#each navHref as href, i}
					<a {href} onclick={gulir} class="transition hover:text-emas">{L.hdr.nav[i]}</a>
				{/each}
			</nav>

			<div class="flex items-center gap-2">
				<LangSwitch btnClass="size-9 rounded-full border border-white/25 bg-white/10 text-gading transition hover:bg-white/20" badgeClass="bg-emas text-merah-gelap" />
				<a href="/login" class="hidden rounded-full px-4 py-2 text-sm font-semibold transition hover:text-emas sm:block">{L.hdr.masuk}</a>
				<a href="/register" class="rounded-full bg-emas px-5 py-2.5 text-sm font-bold text-merah-gelap transition hover:bg-white focus-visible:ring-4 focus-visible:ring-emas/60">{L.hdr.daftar}</a>
				<button
					type="button"
					class="grid size-10 place-items-center rounded-full border border-white/25 md:hidden"
					aria-label={L.hdr.menu}
					aria-expanded={menu}
					onclick={() => (menu = !menu)}
				>
					<svg class="size-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true">
						{#if menu}<path d="M6 6l12 12M18 6 6 18" />{:else}<path d="M4 7h16M4 12h16M4 17h16" />{/if}
					</svg>
				</button>
			</div>
		</div>
		{#if menu}
			<nav class="border-t border-white/10 px-5 pb-4 md:hidden" aria-label={L.hdr.navSel}>
				{#each navHref as href, i}
					<a {href} onclick={gulir} class="block border-b border-white/10 py-3 text-sm font-semibold">{L.hdr.nav[i]}</a>
				{/each}
				<a href="/login" class="block py-3 text-sm font-semibold">{L.hdr.masuk}</a>
			</nav>
		{/if}
	</header>

	<!-- ================= Hero ================= -->
	<section class="relative isolate overflow-hidden bg-merah-gelap text-gading">
		<div class="pola-kawung absolute inset-0 -z-10 opacity-60" aria-hidden="true"></div>
		<div class="absolute -top-40 -right-32 -z-10 size-[34rem] rounded-full bg-merah/40 blur-3xl" aria-hidden="true"></div>
		<div class="absolute -bottom-48 -left-32 -z-10 size-[30rem] rounded-full bg-emas/15 blur-3xl" aria-hidden="true"></div>
		<svg class="animate-denyut absolute top-28 left-[46%] size-6 text-emas" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 0c.8 6.5 4.5 10.2 12 12-7.5 1.8-11.2 5.5-12 12-.8-6.5-4.5-10.2-12-12C7.500 10.200 11.200 6.500 12 0z" /></svg>
		<svg class="animate-denyut absolute top-40 right-[10%] size-4 text-white/70 [animation-delay:-1.5s]" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 0c.8 6.5 4.5 10.2 12 12-7.5 1.8-11.2 5.5-12 12-.8-6.5-4.5-10.2-12-12C7.500 10.200 11.200 6.500 12 0z" /></svg>

				<div class="mx-auto grid max-w-6xl items-center gap-12 px-5 pt-16 pb-24 lg:grid-cols-[1.1fr_.9fr] lg:pt-24 lg:pb-32">
			<div class="animate-naik">
				<a href="#cara-kerja" onclick={gulir} class="inline-flex items-center gap-2 rounded-full border border-white/20 bg-white/10 px-4 py-1.5 text-xs font-bold backdrop-blur">
					<span class="size-2 rounded-full bg-emas"></span> {L.hero.badge}
				</a>
				<h1 class="font-display mt-6 text-4xl leading-[1.05] font-extrabold sm:text-5xl lg:text-6xl">
					{L.hero.h1}
					<span class="mt-2 block min-h-[2.15em] text-[.8em] leading-[1.1] whitespace-nowrap text-emas" aria-live="polite">
						{#key kata + i18n.lang}
							<span class="kata {efek[kata]}">
								{#each frasa[kata] as baris, bi}<span class="block">{#each baris.split(' ') as w, wi}<span class="inline-block whitespace-nowrap">{#each w.split('') as c, ci}<span class="huruf inline-block" style="--i:{posisi(bi, wi, ci)}">{c}</span>{/each}</span>{' '}{/each}</span>{/each}
							</span>
						{/key}
					</span>
				</h1>
				<p class="mt-6 max-w-xl text-base leading-relaxed text-gading/80 sm:text-lg">
					{L.hero.sub}
				</p>
				<div class="mt-8 flex flex-wrap items-center gap-3">
					<a href="/register" class="tombol-kilau relative overflow-hidden rounded-full bg-emas px-7 py-3.5 text-sm font-bold text-merah-gelap transition hover:bg-white focus-visible:ring-4 focus-visible:ring-emas/60">{L.hero.mulai}</a>
					<a href="#fitur" onclick={gulir} class="rounded-full border border-white/30 px-7 py-3.5 text-sm font-bold transition hover:bg-white/10">{L.hero.fitur}</a>
				</div>
				<dl class="mt-10 flex flex-wrap gap-x-10 gap-y-4 text-sm">
					{#each L.hero.stat as [k, v]}
						<div><dt class="text-gading/60">{k}</dt><dd class="font-display text-2xl font-extrabold">{v}</dd></div>
					{/each}
				</dl>			</div>

			<!-- Kalkulator mini -->
			<div class="animate-naik relative [animation-delay:.2s]">
				<div class="absolute -inset-1 rounded-[2rem] bg-gradient-to-br from-emas via-white/30 to-merah opacity-60 blur-lg"></div>
				<div class="relative overflow-hidden rounded-[1.75rem] bg-gading p-7 text-merah-gelap shadow-2xl shadow-black/40">
					<div class="garis-emas animate-kilau absolute inset-x-0 top-0 h-1.5"></div>
					<h2 class="font-display text-2xl font-extrabold">{L.hero.kalk.judul}</h2>
					<p class="mt-1 text-xs text-merah-gelap/60">{L.hero.kalk.ket}</p>

					<label for="nominal-mini" class="mt-6 block text-xs font-bold">{L.hero.kalk.nominal}</label>
					<div class="mt-2 flex items-center rounded-xl border border-merah-gelap/15 bg-white focus-within:border-merah focus-within:ring-4 focus-within:ring-merah/15">
						<span class="pl-4 text-sm font-bold text-merah-gelap/60">Rp</span>
						<input
							id="nominal-mini"
							type="text"
							inputmode="numeric"
							autocomplete="off"
							maxlength="11"
							placeholder="0"
							value={ribuan(mulai)}
							oninput={ketikNominal}
							class="w-full px-3 py-3 text-lg font-bold tabular-nums outline-none"
						/>
					</div>
					<input type="range" min="1000" max="5000000" step="1000" bind:value={mulai} aria-label={L.hero.kalk.geser} class="mt-4 w-full accent-merah" />

					<dl class="mt-5 space-y-2 text-sm">
						<div class="flex justify-between"><dt class="text-merah-gelap/70">{L.hero.kalk.mdr}</dt><dd class="font-semibold">− {formatIDR(hMdr)}</dd></div>
						<div class="flex justify-between"><dt class="text-merah-gelap/70">{L.hero.kalk.jasa}</dt><dd class="font-semibold">− {formatIDR(hJasa)}</dd></div>
						<div class="flex items-end justify-between border-t border-merah-gelap/10 pt-4">
							<dt class="font-bold">{L.hero.kalk.diterima}</dt>
							<dd class="font-display text-3xl font-extrabold text-merah">{formatIDR(hNet)}</dd>
						</div>
					</dl>
				</div>

				<div class="animate-terbang absolute -top-5 -right-3 flex items-center gap-3 rounded-2xl border-2 border-emas bg-gading px-4 py-3 text-merah-gelap shadow-xl sm:-right-6">
					<span class="grid size-8 place-items-center rounded-full bg-green-600 text-white">
						<svg class="size-4" fill="none" stroke="currentColor" stroke-width="3" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5 9-10" /></svg>
					</span>
					<div class="text-xs leading-tight"><div class="font-bold">{L.hero.kalk.berhasil}</div><div class="font-display text-sm font-extrabold text-merah">Rp25.000</div></div>
				</div>
			</div>
		</div>
	</section>

	<!-- ================= Strip fitur ================= -->
	<section class="overflow-hidden bg-emas py-4 text-merah-gelap" aria-label="Ringkasan fitur">
		<div class="flex w-max animate-[marquee_38s_linear_infinite] gap-10 whitespace-nowrap text-sm font-bold">
			{#each [0, 1] as _}
				<div class="flex shrink-0 items-center gap-10" aria-hidden={_ === 1}>
					{#each L.strip as s}
						<span class="flex items-center gap-10">
							{s}
							<svg class="size-4 shrink-0" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 0c.8 6.5 4.5 10.2 12 12-7.5 1.8-11.2 5.5-12 12-.8-6.5-4.5-10.2-12-12C7.500 10.200 11.200 6.500 12 0z" /></svg>
						</span>
					{/each}
				</div>
			{/each}
		</div>
	</section>

	<KenaliQris />

	<TentangEraPay />

	<CaraKerja />

	<FiturPantai />

	<!-- ================= CTA akhir ================= -->
	<div class="relative">
	<Ombak />
	<section class="relative isolate min-h-[34rem] overflow-hidden bg-[linear-gradient(#0b4f6c_0,#0b4f6c_4.6rem,#073b57_45%,#04243a)] py-24 text-white lg:py-32">
		<DasarLaut />
		<div use:reveal class="relative mx-auto max-w-3xl px-5 text-center">
			<h2 class="judul-laut text-balance text-3xl leading-[1.12] sm:text-4xl lg:text-6xl">{L.cta.judul}</h2>
			<p class="mx-auto mt-5 max-w-xl text-base font-semibold text-cyan-50/90 sm:text-lg">{L.cta.isi}</p>
			<div class="mt-8 flex flex-wrap justify-center gap-3">
				<a href="/register" class="rounded-full bg-emas px-7 py-3.5 text-sm font-bold text-merah-gelap transition hover:bg-white focus-visible:ring-4 focus-visible:ring-emas/60">{L.cta.daftar}</a>
				<a href="/login" class="rounded-full border border-white/40 px-7 py-3.5 text-sm font-bold transition hover:bg-white/10">{L.cta.masuk}</a>
			</div>
		</div>
	</section>
	</div>

</div>

<style>
	/* Judul CTA: huruf tebal bergaya laut, isian gradien air yang bergerak pelan. */
	.judul-laut {
		font-family: 'Lilita One', 'Dela Gothic One', 'Plus Jakarta Sans', sans-serif;
		font-weight: 400;
		letter-spacing: 0.01em;
		background: linear-gradient(100deg, #ffffff 0%, #bff3ff 25%, #5fd4f0 50%, #bff3ff 75%, #ffffff 100%) 0 0 / 220% 100%;
		-webkit-background-clip: text;
		background-clip: text;
		-webkit-text-fill-color: transparent;
		color: transparent;
		filter: drop-shadow(0 3px 0 #04465f) drop-shadow(0 8px 14px rgb(0 20 40 / 0.45));
		animation: air-judul 7s ease-in-out infinite alternate;
	}
	@keyframes air-judul { to { background-position: 100% 0; } }
	@media (prefers-reduced-motion: reduce) { .judul-laut { animation: none; } }
	.kata { display: block; }
	.huruf { animation-fill-mode: both; animation-duration: .7s; animation-timing-function: cubic-bezier(.2,.7,.2,1); animation-delay: calc(var(--i) * 28ms); }
	.e-naik .huruf { animation-name: h-naik; }
	.e-ketik .huruf { animation-name: h-ketik; animation-duration: .01s; animation-delay: calc(var(--i) * 55ms); }
	.huruf { position: relative; }
	.e-ketik .huruf::after { content: ''; position: absolute; right: -.04em; top: .12em; width: .06em; height: .85em; background: currentColor; opacity: 0; animation: h-kursor 55ms steps(1) backwards; animation-delay: calc(var(--i) * 55ms); }
	@keyframes h-kursor { from, to { opacity: 1; } }
	.e-putar .huruf { animation-name: h-putar; transform-origin: 50% 100%; }
	.e-zoom .huruf { animation-name: h-zoom; }
	.e-geser .huruf { animation-name: h-geser; animation-delay: calc(var(--i) * 22ms); }
	@keyframes h-naik { from { opacity: 0; transform: translateY(.7em); filter: blur(8px); } to { opacity: 1; transform: none; filter: none; } }
	@keyframes h-ketik { from { opacity: 0; } to { opacity: 1; } }
	@keyframes h-putar { from { opacity: 0; transform: perspective(400px) rotateX(-90deg); } to { opacity: 1; transform: none; } }
	@keyframes h-zoom { from { opacity: 0; transform: scale(2.4); filter: blur(10px); } to { opacity: 1; transform: none; filter: none; } }
	@keyframes h-geser { from { opacity: 0; transform: translateX(-.8em) skewX(-18deg); } to { opacity: 1; transform: none; } }
	@keyframes kedip { 50% { opacity: 0; } }
</style>
