<script lang="ts">
	import '../../../app.css';
	import { page } from '$app/state';
	import { createI18n, setI18n, getI18n } from '../../../lib/i18n/i18n.svelte';
	import Logo from '../../../lib/components/auth/Logo.svelte';

	let { data, children } = $props();
	// svelte-ignore state_referenced_locally
	const i18n = createI18n(data.lang);
	setI18n(i18n);
	const { t } = getI18n();

	const items = [
		{ href: '/platform', label: 'Ringkasan' },
		{ href: '/platform/tenants', label: 'Masuk sebagai tenant' },
		{ href: '/platform/audit', label: 'Audit log' },
		{ href: '/platform/access', label: 'Hak akses (RBAC)' }
	];
	const current = $derived(items.find((n) => page.url.pathname === n.href || page.url.pathname.startsWith(n.href === '/platform' ? '/platform/' : n.href + '/')));
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

<div data-theme={data.theme} class="adm min-h-screen bg-page font-sans text-ink antialiased">
	<header class="border-b border-ink/10 bg-surface">
		<div class="mx-auto flex max-w-6xl items-center justify-between gap-3 px-5 py-3">
			<div class="flex items-center gap-3">
				<Logo class="h-9" />
				<div class="leading-tight">
					<div class="text-sm font-extrabold tracking-wide text-merah-gelap">PANEL PLATFORM</div>
					<div class="-mt-0.5 text-[9px] tracking-[.2em] text-ink-soft uppercase">DEWA · Platform Admin</div>
				</div>
			</div>
			<nav class="flex items-center gap-1" aria-label="Menu panel">
				{#each items as n (n.href)}
					<a
						href={n.href}
						aria-current={current?.href === n.href ? 'page' : undefined}
						class="rounded-full px-3.5 py-2 text-xs font-bold text-ink-soft transition hover:bg-ink/5 hover:text-ink aria-[current=page]:bg-merah aria-[current=page]:text-white"
					>
						{n.label}
					</a>
				{/each}
			</nav>
			<form method="POST" action="/platform/logout" class="flex items-center gap-2">
				<span class="hidden text-right md:block">
					<span class="block max-w-44 truncate text-xs font-bold text-ink">{nama}</span>
					<span class="block text-[10px] text-ink-soft">DEWA · {data.user.email}</span>
				</span>
				<span class="grid size-9 place-items-center rounded-full bg-merah text-xs font-bold text-white">{inisial}</span>
				<button type="submit" class="rounded-full border border-ink/10 px-3 py-2 text-xs font-bold text-ink-soft transition hover:bg-red-50 hover:text-red-700">
					{t('hdr.logout')}
				</button>
			</form>
		</div>
	</header>
	<main class="mx-auto max-w-6xl p-5 lg:p-8">
		{@render children()}
		<footer class="muted mt-10 text-[10px]">{t('footer.tagline')}</footer>
	</main>
</div>
