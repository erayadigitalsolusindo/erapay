<script lang="ts">
	import { page } from '$app/state';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import { aktif, visible } from './nav';
	import Logo from '../../../lib/components/auth/Logo.svelte';

	let { user }: { user: { email: string; merchantId: string; permissions?: string[] } } = $props();
	const { t } = getI18n();
	const items = $derived(visible(user));
	const current = $derived(aktif(page.url.pathname));
</script>

<aside class="sidebar-batik relative flex flex-col overflow-hidden p-4 text-gading lg:sticky lg:top-0 lg:h-screen">
	<a href="/dashboard" class="mb-4 flex flex-col items-center px-2 py-3 lg:mb-7 lg:items-start" aria-label="EraPay">
		<Logo class="h-16 lg:h-12" />
		<div class="mt-2 text-[10px] tracking-widest text-white/75">PAYMENT OPERATIONS</div>
	</a>

	<div class="mb-2 hidden px-3 text-[10px] font-bold tracking-[.16em] text-white/70 uppercase lg:block">{t('nav.workspace')}</div>
	<nav class="flex gap-1 overflow-x-auto pb-1 lg:flex-col lg:overflow-visible lg:pb-0" aria-label={t('nav.main')}>
		{#each items as n (n.href)}
			<a
				href={n.href}
				aria-current={current.href === n.href ? 'page' : undefined}
				class="flex shrink-0 items-center gap-3 rounded-xl px-3 py-2.5 text-[13px] font-semibold text-gading/85 transition hover:bg-white/15 hover:text-white aria-[current=page]:bg-black/20 aria-[current=page]:text-white aria-[current=page]:shadow-[inset_3px_0_#e8b84a]"
			>
				<svg class="size-[18px] shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24" aria-hidden="true">{@html n.icon}</svg>
				{t(n.label)}
			</a>
		{/each}
	</nav>

</aside>
