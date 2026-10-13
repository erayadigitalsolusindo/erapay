<script lang="ts">
	import { page } from '$app/state';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import { aktif, visible } from './nav';
	import Logo from '../../../lib/components/auth/Logo.svelte';
	import Sea from './Sea.svelte';

	let { user, menus }: { user: { email: string; merchantId: string; permissions?: string[] }, menus?: string[] | null } = $props();
	const { t } = getI18n();
	const items = $derived(visible(user, menus));
	const current = $derived(aktif(page.url.pathname));
</script>

<!--
	Sidebar admin: latar terang, motif batik samar, dan laut beranimasi di bagian bawah.
	Laut digambar mulai 64% tinggi (lihat Sea.svelte), jadi area navigasi tetap lega.

	Aksen merah EraPay pada titik penanda dan halaman aktif. Warna teks memakai token
	tema (text-ink / text-ink-soft) supaya sidebar ikut terang-gelap, bukan hardcode.
	Status halaman aktif ditandai titik merah + lat bawah + garis kiri, jadi tidak hanya
	berbeda warna (PRD §4.11). Laut dan bintang bersifat dekoratif: aria-hidden, tidak
	menerima klik, dan diam total bila pengguna memilih reduced motion.
-->
<aside class="sidebar-laut-shell relative flex flex-col overflow-hidden p-4 lg:sticky lg:top-0 lg:h-screen">
	<Sea />

	<div class="relative z-10 flex flex-col lg:min-h-0 lg:flex-1">
		<a href="/dashboard" class="mb-5 flex flex-col items-center px-2 py-2 lg:mb-6 lg:items-start" aria-label="EraPay">
			<Logo class="h-14 lg:h-11" />
			<div class="mt-1.5 text-[10px] tracking-widest text-ink-soft">PAYMENT OPERATIONS</div>
		</a>

		<div class="mb-2 hidden px-3 text-[10px] font-bold tracking-[.16em] text-ink-soft uppercase lg:block">{t('nav.workspace')}</div>
		<nav class="flex gap-1 overflow-x-auto pb-1 lg:flex-1 lg:flex-col lg:overflow-y-auto lg:pb-0" aria-label={t('nav.main')}>
			{#each items as n (n.href)}
				<a
					href={n.href}
					aria-current={current.href === n.href ? 'page' : undefined}
					class="sidebar-item group flex shrink-0 items-center gap-3 rounded-xl px-3 py-2.5 text-[13px] font-semibold text-ink transition hover:bg-merah/10 dark:hover:bg-amber-900/20 aria-[current=page]:bg-emas/15 dark:aria-[current=page]:bg-emas/20 aria-[current=page]:text-emas aria-[current=page]:shadow-[inset_3px_0_0_var(--color-emas)]"
				>
					<!-- Titik penanda: border merah saat tidak aktif; hover merah (light) / amber (dark); active emas. -->
					<span
						aria-current={current.href === n.href ? 'page' : undefined}
						class="size-2 shrink-0 rounded-full border-2 border-merah/60 transition-colors group-hover:border-merah dark:group-hover:border-amber-400 aria-[current=page]:border-emas aria-[current=page]:bg-emas"
						aria-hidden="true"
					></span>
					<svg class="size-[17px] shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24" aria-hidden="true">{@html n.icon}</svg>
					{t(n.label)}
				</a>
			{/each}
		</nav>
	</div>
</aside>