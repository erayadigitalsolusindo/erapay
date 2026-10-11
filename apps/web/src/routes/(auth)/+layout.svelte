<script lang="ts">
	import '../../app.css';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { setContext } from 'svelte';
	import Backdrop from '../../lib/components/auth/Backdrop.svelte';
	import Hero from '../../lib/components/auth/Hero.svelte';
	import { kurangGerak, semburUang, transisiAwan } from '../../lib/utils/transisi';

	let { children } = $props();

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

<div class="auth isolate font-sans min-h-screen overflow-x-hidden bg-merah-gelap text-gading antialiased">
	<Backdrop />

	<main class="mx-auto grid min-h-screen max-w-6xl items-center gap-10 px-5 py-10 lg:grid-cols-2 lg:gap-16">
		<Hero />

		<section class="animate-naik mx-auto w-full max-w-md [animation-delay:.15s]">
			<div class="relative">
				<div class="absolute -inset-1 rounded-[2rem] bg-gradient-to-br from-emas via-white/40 to-merah opacity-70 blur-lg"></div>
				<div bind:this={kartu} class="relative overflow-clip rounded-[1.75rem] bg-gading p-7 text-neutral-800 shadow-2xl shadow-black/40 sm:p-9">
					<div
						aria-hidden="true"
						class="pola-kawung absolute -top-10 -right-10 size-40 rotate-12 opacity-80 [filter:invert(1)_sepia(1)_saturate(6)_hue-rotate(-30deg)]"
					></div>
					<div class="garis-emas animate-kilau absolute inset-x-0 top-0 h-1.5"></div>
					<div class="relative">{@render children()}</div>
				</div>
			</div>
			<p class="mt-6 text-center text-xs text-gading/75">© 2026 ERAPAY · Merdeka bertransaksi</p>
		</section>
	</main>

	<!-- Lapisan transisi awan & uang -->
	<div bind:this={awanKiri} aria-hidden="true" class="pointer-events-none fixed top-0 left-0 z-[60] will-change-transform" style="display:none;filter:drop-shadow(0 0 22px rgba(20,60,110,.35))"></div>
	<div bind:this={awanKanan} aria-hidden="true" class="pointer-events-none fixed top-0 right-0 z-[60] will-change-transform" style="display:none;filter:drop-shadow(0 0 22px rgba(20,60,110,.35))"></div>
	<div bind:this={lapUang} aria-hidden="true" class="pointer-events-none fixed inset-0 z-50 overflow-hidden"></div>
</div>
