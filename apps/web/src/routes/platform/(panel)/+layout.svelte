<script lang="ts">
	import '../../../app.css';
	import { page } from '$app/state';
	import { createI18n, setI18n, getI18n } from '../../../lib/i18n/i18n.svelte';
	import Logo from '../../../lib/components/auth/Logo.svelte';
	import LangSwitch from '../../../lib/components/LangSwitch.svelte';

	let { data, children } = $props();
	// svelte-ignore state_referenced_locally
	const i18n = createI18n(data.lang);
	setI18n(i18n);
	const { t } = getI18n();

	$effect(() => {
		document.documentElement.lang = i18n.lang;
	});

	const items = [
		{ href: '/platform', label: 'pf.nav.overview', icon: 'M3 12 12 3l9 9M5 10v10h5v-6h4v6h5V10' },
		{ href: '/platform/tenants', label: 'pf.nav.tenants', icon: 'M3 21h18M5 21V7l7-4 7 4v14M9 9h1m4 0h1M9 13h1m4 0h1M9 17h1m4 0h1' },
		{ href: '/platform/gateways', label: 'pf.nav.gateways', icon: 'M15 7a4 4 0 1 1-3.5 6L4 20.5V22h3v-2h2v-2h2l1.5-1.5' },
		{ href: '/platform/audit', label: 'pf.nav.audit', icon: 'M9 5h10M9 12h10M9 19h10M4 5h.01M4 12h.01M4 19h.01' },
		{ href: '/platform/access', label: 'pf.nav.access', icon: 'M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6l-8-3Z' }
	] as const;
	// Menu aktif = href terpanjang yang cocok; kalau tidak, "/platform" cocok untuk semua halaman di bawahnya.
	const current = $derived(
		items
			.filter((n) => page.url.pathname === n.href || page.url.pathname.startsWith(n.href + '/'))
			.sort((a, b) => b.href.length - a.href.length)[0]
	);
	const nama = $derived(data.user.email.split('@')[0]);
	const inisial = $derived(nama.slice(0, 2).toUpperCase());
</script>

<svelte:head>
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
	<link
		href="https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,600;9..144,800&family=Plus+Jakarta+Sans:wght@400;500;600;700;800&display=swap"
		rel="stylesheet"
	/>
</svelte:head>

<div data-theme={data.theme} class="adm flex min-h-screen flex-col bg-page font-sans text-ink antialiased lg:flex-row">
	<!-- Sidebar: sticky di layar lebar, bar horizontal yang bisa digeser di HP -->
	<aside class="z-20 flex shrink-0 flex-col bg-merah-gelap text-white lg:sticky lg:top-0 lg:h-screen lg:w-64">
		<div class="flex items-center gap-3 px-4 py-3 lg:px-5 lg:py-6">
			<span class="grid shrink-0 place-items-center rounded-xl bg-white/95 px-2 py-1"><Logo class="h-8" /></span>
			<div class="min-w-0 leading-tight">
				<div class="truncate text-sm font-extrabold tracking-wide">{t('pf.brand')}</div>
				<div class="-mt-0.5 text-[9px] tracking-[.2em] text-white/60 uppercase">DEWA · Platform Admin</div>
			</div>
		</div>

		<nav class="flex gap-1 overflow-x-auto px-3 pb-3 lg:flex-1 lg:flex-col lg:overflow-y-auto lg:overflow-x-visible lg:pb-0" aria-label={t('pf.menu')}>
			{#each items as n (n.href)}
				<a
					href={n.href}
					aria-current={current?.href === n.href ? 'page' : undefined}
					class="flex shrink-0 items-center gap-3 rounded-xl px-3.5 py-2.5 text-[13px] font-bold whitespace-nowrap text-white/70 transition hover:bg-white/10 hover:text-white aria-[current=page]:bg-merah aria-[current=page]:text-white aria-[current=page]:shadow-sm"
				>
					<svg viewBox="0 0 24 24" class="size-[18px] shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d={n.icon} /></svg>
					{t(n.label)}
				</a>
			{/each}
		</nav>

		<div class="hidden border-t border-white/10 p-4 lg:block">
			<div class="flex items-center gap-3">
				<span class="grid size-9 shrink-0 place-items-center rounded-full bg-merah text-xs font-bold text-white">{inisial}</span>
				<span class="min-w-0 leading-tight">
					<span class="block truncate text-xs font-bold">{nama}</span>
					<span class="block truncate text-[10px] text-white/55">DEWA · {data.user.email}</span>
				</span>
			</div>
			<div class="mt-3 flex items-center gap-2">
				<LangSwitch btnClass="size-9 rounded-full border border-white/20 text-white/80 hover:bg-white/10" badgeClass="bg-emas text-merah-gelap" />
				<form method="POST" action="/platform/logout" class="flex-1">
					<button type="submit" class="w-full rounded-full border border-white/20 px-3 py-2 text-xs font-bold text-white/80 transition hover:bg-red-500/30 hover:text-white">
						{t('hdr.logout')}
					</button>
				</form>
			</div>
		</div>

		<!-- HP: akun dan keluar ikut di bar atas supaya tidak hilang -->
		<div class="flex items-center justify-between gap-2 border-t border-white/10 px-4 py-2 lg:hidden">
			<span class="truncate text-[11px] text-white/70">{data.user.email}</span>
			<LangSwitch btnClass="size-8 shrink-0 rounded-full border border-white/20 text-white/80 hover:bg-white/10" badgeClass="bg-emas text-merah-gelap" />
			<form method="POST" action="/platform/logout">
				<button type="submit" class="rounded-full border border-white/20 px-3 py-1.5 text-[11px] font-bold text-white/80">{t('hdr.logout')}</button>
			</form>
		</div>
	</aside>

	<div class="flex min-w-0 flex-1 flex-col">
		<main class="mx-auto w-full max-w-6xl flex-1 p-5 lg:p-8">
			{@render children()}
		</main>
		<footer class="muted border-t border-ink/10 px-5 py-3 text-center text-[10px]">{t('footer.tagline')}</footer>
	</div>
</div>
