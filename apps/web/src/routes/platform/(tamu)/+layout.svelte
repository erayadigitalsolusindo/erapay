<script lang="ts">
	import '../../../app.css';
	import { createI18n, setI18n } from '../../../lib/i18n/i18n.svelte';
	import Logo from '../../../lib/components/auth/Logo.svelte';

	let { data, children } = $props();
	// svelte-ignore state_referenced_locally
	const i18n = createI18n(data.lang);
	setI18n(i18n);
</script>

<svelte:head>
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
	<link
		href="https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,600;9..144,800&family=Plus+Jakarta+Sans:wght@400;500;600;700;800&display=swap"
		rel="stylesheet"
	/>
</svelte:head>

<div data-theme="light" class="adm min-h-screen bg-merah-gelap font-sans text-gading antialiased">
	<div class="pola relative flex min-h-screen items-center justify-center px-5 py-10">
		<main class="card w-full max-w-md rounded-2xl bg-gading p-7 text-neutral-800 shadow-2xl shadow-black/40">
			<div class="flex flex-col items-center text-center">
				<Logo class="h-12" />
				<h1 class="font-display mt-5 text-2xl font-extrabold text-merah-gelap">Panel Platform · DEWA</h1>
				<p class="muted mt-1 text-xs">EraPay · Platform Admin</p>
			</div>
			<div class="mt-6">{@render children()}</div>
		</main>
		<p class="pointer-events-none fixed top-4 left-5 text-[10px] tracking-widest text-gading/40">ERAPAY · /platform</p>
	</div>
</div>
