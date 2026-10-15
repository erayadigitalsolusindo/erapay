<script lang="ts">
	import '../../app.css';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { setContext } from 'svelte';
	import { createI18n, setI18n } from '../../lib/i18n/i18n.svelte';
	import LangSwitch from '../../lib/components/LangSwitch.svelte';
	import Backdrop from '../../lib/components/auth/Backdrop.svelte';
	import Hero from '../../lib/components/auth/Hero.svelte';
	import { kurangGerak, semburUang, transisiAwan } from '../../lib/utils/transisi';

	let { data, children } = $props();

	// Nilai awal dari cookie (server); perubahan ditulis balik ke cookie yang sama dengan dasbor.
	// svelte-ignore state_referenced_locally
	const i18n = createI18n(data.lang);
	setI18n(i18n);
	// svelte-ignore state_referenced_locally
	let theme = $state<'light' | 'dark' | 'system'>(data.theme);
	const simpan = (k: string, v: string) => {
		document.cookie = `${k}=${v}; path=/; max-age=31536000; samesite=lax`;
	};
	// Di SSR (tanpa matchMedia) mode system dianggap terang; sisi klien menyesuaikan saat hydrate.
	const gelap = $derived(
		theme === 'dark' || (theme === 'system' && typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches)
	);
	function gantiTema() {
		theme = gelap ? 'light' : 'dark';
		simpan('erapay_theme', theme);
	}
	$effect(() => {
		document.documentElement.lang = i18n.lang;
	});

	let kartu: HTMLElement;
	let awanKiri: HTMLElement;
	let awanKanan: HTMLElement;
	let lapUang: HTMLElement;
	let pindahBerjalan = false;

	/** Dipakai halaman Masuk/Daftar lewat getContext('auth'). */
	setContext('auth', {
		goyang() {
			kartu.classList.remove('animate-getar');
			void kartu.offsetWidth; // mulai ulang animasi
			kartu.classList.add('animate-getar');
		},
		uang() {
			if (!kurangGerak()) semburUang(lapUang, -1, 26);
		},
		async pindah(href: string) {
			if (pindahBerjalan || page.url.pathname === href) return;
			if (kurangGerak()) return goto(href);
			pindahBerjalan = true;
			try {
				await transisiAwan(awanKiri, awanKanan, () => goto(href));
			} finally {
				pindahBerjalan = false;
			}
		}
	});
</script>

<svelte:head>
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
	<link
		href="https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,600;9..144,800&family=Plus+Jakarta+Sans:wght@400;500;600;700&display=swap"
		rel="stylesheet"
	/>
</svelte:head>

<div class:auth-gelap={gelap} class="auth isolate font-sans min-h-screen overflow-x-hidden bg-merah-gelap text-gading antialiased">
	<Backdrop />
	<div aria-hidden="true" class="pointer-events-none fixed inset-0 -z-10 bg-[#12060a]/65 transition-opacity duration-500 {gelap ? 'opacity-100' : 'opacity-0'}"></div>

	<main class="mx-auto grid min-h-screen max-w-6xl items-center gap-10 px-5 py-10 lg:grid-cols-2 lg:gap-16">
		<Hero />

		<section class="animate-naik mx-auto w-full max-w-md [animation-delay:.15s]">
			<div class="relative">
				<div class="absolute -inset-1 rounded-[2rem] bg-gradient-to-br from-emas via-white/40 to-merah opacity-70 blur-lg"></div>
				<div bind:this={kartu} class="kartu relative overflow-clip rounded-[1.75rem] bg-gading p-7 text-neutral-800 shadow-2xl shadow-black/40 sm:p-9">
					<div
						aria-hidden="true"
						class="pola-kawung absolute -top-10 -right-10 size-40 rotate-12 opacity-80 [filter:invert(1)_sepia(1)_saturate(6)_hue-rotate(-30deg)]"
					></div>
					<div class="garis-emas animate-kilau absolute inset-x-0 top-0 h-1.5"></div>
					<div class="relative">{@render children()}</div>
				</div>
			</div>
			<div class="mt-6 flex flex-wrap items-center justify-center gap-2 text-gading">
				<span class="rounded-full border border-white/25 bg-white/10 px-3 py-1.5 text-[11px] font-semibold backdrop-blur xl:text-xs">{i18n.t('footer.tagline')}</span>
				<button
					type="button"
					onclick={gantiTema}
					title={gelap ? i18n.t('hdr.theme.light') : i18n.t('hdr.theme.dark')}
					aria-label={gelap ? i18n.t('hdr.theme.light') : i18n.t('hdr.theme.dark')}
					class="grid size-8 place-items-center rounded-full border border-white/25 bg-white/10 backdrop-blur transition hover:bg-white/20"
				>
					{#if gelap}
						<svg class="size-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><path d="M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z"/></svg>
					{:else}
						<svg class="size-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>
					{/if}
				</button>
				<LangSwitch btnClass="size-8 rounded-full border border-white/25 bg-white/10 backdrop-blur transition hover:bg-white/20" badgeClass="bg-emas text-merah-gelap" />
			</div>
		</section>
	</main>

	<!-- Lapisan transisi awan & uang -->
	<div bind:this={awanKiri} aria-hidden="true" class="pointer-events-none fixed top-0 left-0 z-[60] will-change-transform" style="display:none;filter:drop-shadow(0 0 22px rgba(20,60,110,.35))"></div>
	<div bind:this={awanKanan} aria-hidden="true" class="pointer-events-none fixed top-0 right-0 z-[60] will-change-transform" style="display:none;filter:drop-shadow(0 0 22px rgba(20,60,110,.35))"></div>
	<div bind:this={lapUang} aria-hidden="true" class="pointer-events-none fixed inset-0 z-50 overflow-hidden"></div>
</div>
