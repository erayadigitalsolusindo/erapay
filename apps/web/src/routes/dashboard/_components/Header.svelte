<script lang="ts">
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import { LANGS } from '../../../lib/i18n/messages';
	import { visible } from './nav';

	type Theme = 'light' | 'dark' | 'system';
	let {
		user,
		theme = $bindable()
	}: { user: { email: string; merchantId: string; permissions?: string[] }; theme: Theme } = $props();

	const i18n = getI18n();
	const { t } = i18n;
	const pintas = $derived(visible(user));
	const nama = $derived(user.email.split('@')[0]);
	const inisial = $derived(nama.slice(0, 2).toUpperCase());

	let menu = $state<'' | 'pintas' | 'profil'>('');
	let pintasMenu = $state<HTMLDivElement | null>(null);
	let profilMenu = $state<HTMLDivElement | null>(null);
	const toggle = (m: 'pintas' | 'profil') => (e: Event) => {
		e.stopPropagation();
		menu = menu === m ? '' : m;
	};

	// Preferensi disimpan di cookie supaya server bisa merender tema/bahasa yang benar tanpa kedip.
	const simpan = (k: string, v: string) => {
		document.cookie = `${k}=${v}; path=/; max-age=31536000; samesite=lax`;
	};
	// Di SSR (tanpa matchMedia) mode system dianggap terang; sisi klien menyesuaikan saat hydrate.
	const gelapSekarang = () =>
		theme === 'dark' || (theme === 'system' && typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches);
	function gantiTema() {
		theme = gelapSekarang() ? 'light' : 'dark';
		simpan('erapay_theme', theme);
	}
	function gantiBahasa() {
		i18n.lang = LANGS[(LANGS.indexOf(i18n.lang) + 1) % LANGS.length];
		simpan('erapay_lang', i18n.lang);
	}
	function layarPenuh() {
		if (document.fullscreenElement) document.exitFullscreen();
		else document.documentElement.requestFullscreen?.();
	}
	const tombolBulat =
		'grid size-10 place-items-center rounded-full bg-ink/5 hover:bg-ink/10 focus-visible:ring-4 focus-visible:ring-emas/60';
</script>

<!-- Tutup hanya saat klik di luar: klik di dalam menu (mis. tombol Keluar) harus tetap jalan. -->
<svelte:window
	onclick={(e) => {
		const t = e.target as Node | null;
		if (t && (pintasMenu?.contains(t) || profilMenu?.contains(t))) return;
		menu = '';
	}}
	onkeydown={(e) => e.key === 'Escape' && (menu = '')}
/>

<header class="relative bg-surface">
	<div class="flex items-center justify-between gap-3 px-5 py-3.5 lg:px-8">
		<div class="relative">
			<button type="button" onclick={toggle('pintas')} aria-expanded={menu === 'pintas'} class="flex items-center gap-2.5 rounded-full border border-ink/10 bg-surface px-4 py-2.5 text-sm font-semibold tracking-wide uppercase shadow-sm hover:bg-ink/5 focus-visible:ring-4 focus-visible:ring-emas/60">
				<svg class="size-4 text-merah" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></svg>
				{t('hdr.shortcuts')}
				<svg class="size-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>
			</button>
			{#if menu === 'pintas'}
				<div bind:this={pintasMenu} class="card absolute top-full left-0 z-30 mt-2 w-56 p-1.5 shadow-lg">
					{#each pintas as n (n.href)}
						<a href={n.href} class="block rounded-lg px-3 py-2 text-xs font-semibold hover:bg-ink/5">{t(n.label)}</a>
					{/each}
					{#if user.merchantId}<a href="/dashboard/transactions/new" class="block rounded-lg px-3 py-2 text-xs font-semibold hover:bg-ink/5">{t('hdr.newQris')}</a>{/if}
				</div>
			{/if}
		</div>

		<div class="flex items-center gap-3">
			<button
				type="button"
				onclick={gantiTema}
				title={gelapSekarang() ? t('hdr.theme.light') : t('hdr.theme.dark')}
				aria-label={gelapSekarang() ? t('hdr.theme.light') : t('hdr.theme.dark')}
				class="grid size-11 place-items-center rounded-full bg-emas/20 text-amber-600 hover:bg-emas/30 focus-visible:ring-4 focus-visible:ring-emas/60"
			>
				{#if gelapSekarang()}
					<svg class="size-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><path d="M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z"/></svg>
				{:else}
					<svg class="size-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>
				{/if}
			</button>

			<div class="hidden items-center gap-1 rounded-full border border-ink/10 bg-surface p-1 shadow-sm sm:flex">
				<button type="button" onclick={gantiBahasa} title={t('hdr.language')} aria-label="{t('hdr.language')}: {i18n.lang.toUpperCase()}" class="{tombolBulat} relative">
					<svg class="size-[18px] text-merah" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5h8M8 3v2m3 0c-.5 4-3 7-6 8m2-4c1 2 3 4 5 5M13 20l4-9 4 9m-1.5-3h-5"/></svg>
					<span class="absolute -right-0.5 -bottom-0.5 rounded-full bg-merah px-1 text-[8px] leading-4 font-bold text-white">{i18n.lang.toUpperCase()}</span>
				</button>
				<button type="button" onclick={layarPenuh} title={t('hdr.fullscreen')} aria-label={t('hdr.fullscreen')} class={tombolBulat}>
					<svg class="size-[18px]" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5"/></svg>
				</button>
			</div>

			<div class="relative">
				<button type="button" onclick={toggle('profil')} aria-expanded={menu === 'profil'} class="flex items-center gap-3 rounded-full border border-ink/10 bg-surface py-1.5 pr-4 pl-1.5 shadow-sm hover:bg-ink/5 focus-visible:ring-4 focus-visible:ring-emas/60">
					<span class="grid size-10 place-items-center rounded-full bg-merah text-sm font-bold text-white">{inisial}</span>
					<span class="hidden min-w-0 text-left md:block">
						<span class="block max-w-44 truncate text-sm font-semibold">{nama}</span>
						<span class="muted block max-w-44 truncate text-xs">{user.merchantId ? t('hdr.merchant', { id: user.merchantId }) : t('hdr.platform')}</span>
					</span>
					<svg class="size-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>
				</button>
				{#if menu === 'profil'}
					<div bind:this={profilMenu} class="card absolute top-full right-0 z-30 mt-2 w-60 p-1.5 shadow-lg">
						<div class="muted truncate px-3 py-2 text-[11px]">{user.email}</div>
						<form method="POST" action="/logout">
							<button type="submit" class="w-full rounded-lg px-3 py-2 text-left text-xs font-semibold text-red-700 hover:bg-red-500/10">{t('hdr.logout')}</button>
						</form>
					</div>
				{/if}
			</div>
		</div>
	</div>
	<div class="garis-emas h-px"></div>
</header>
