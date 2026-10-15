<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import { aktif, nav, visible } from './nav';

	let {
		user,
		menus
	}: { user: { merchantId: string; permissions?: string[] }; menus?: string[] | null } = $props();

	const { t } = getI18n();
	const KEY = 'erapay_dash_tabs';
	const HOME = '/dashboard';

	// Satu tab per menu; `url` = halaman terakhir yang dibuka di tab itu (mis. detail transaksi).
	type Tab = { href: string; url: string };
	let tabs = $state<Tab[]>([]);
	let ready = false;

	const allowed = $derived(new Set(visible(user, menus).map((n) => n.href)));
	const item = (href: string) => nav.find((n) => n.href === href);
	const current = $derived(aktif(page.url.pathname).href);

	function load(): Tab[] {
		try {
			const raw = JSON.parse(sessionStorage.getItem(KEY) ?? '[]');
			return Array.isArray(raw)
				? raw.filter((x): x is Tab => typeof x?.href === 'string' && typeof x?.url === 'string')
				: [];
		} catch {
			return [];
		}
	}

	// Pulihkan saat mount, lalu pastikan halaman aktif punya tab (sidebar, tautan di halaman, back/forward).
	$effect(() => {
		const href = current;
		const url = page.url.pathname + page.url.search;
		const ok = allowed.has(href);
		untrack(() => {
			if (!ready) {
				tabs = load().filter((x) => allowed.has(x.href));
				ready = true;
			}
			if (!ok) return;
			const i = tabs.findIndex((x) => x.href === href);
			if (i < 0) tabs = [...tabs, { href, url }];
			else if (tabs[i].url !== url) tabs[i].url = url;
		});
	});

	$effect(() => {
		if (!ready) return;
		try {
			sessionStorage.setItem(KEY, JSON.stringify(tabs));
		} catch {
			/* penyimpanan diblokir: tab tetap jalan tanpa pemulihan */
		}
	});

	const buka = (tab: Tab) => tab.href !== current && goto(tab.url);

	function tutup(href: string) {
		const i = tabs.findIndex((x) => x.href === href);
		if (i < 0) return;
		const sisa = tabs.filter((x) => x.href !== href);
		tabs = sisa;
		if (href === current) {
			const next = sisa[i] ?? sisa[i - 1];
			goto(next ? next.url : HOME);
		}
	}

	function tutupSemua() {
		tabs = [];
		if (current !== HOME) goto(HOME);
		else tabs = [{ href: HOME, url: HOME }];
	}

	// Menu klik kanan ala browser: tutup tab ini / lainnya / kanan / kiri / semua.
	let menu = $state<{ x: number; y: number; href: string } | null>(null);

	function sisakan(keep: (i: number) => boolean) {
		const sisa = tabs.filter((_, i) => keep(i));
		if (sisa.some((x) => x.href === current)) tabs = sisa;
		else if (sisa.length) {
			tabs = sisa;
			goto(sisa[sisa.length - 1].url);
		} else tutupSemua();
	}

	function aksi(kind: 'close' | 'others' | 'right' | 'left' | 'all') {
		const at = tabs.findIndex((x) => x.href === menu?.href);
		menu = null;
		if (at < 0) return;
		if (kind === 'close') tutup(tabs[at].href);
		else if (kind === 'others') sisakan((i) => i === at);
		else if (kind === 'right') sisakan((i) => i <= at);
		else if (kind === 'left') sisakan((i) => i >= at);
		else tutupSemua();
	}

	function bukaMenu(e: MouseEvent, tab: Tab) {
		e.preventDefault();
		// Jaga agar menu tidak keluar layar.
		menu = { x: Math.min(e.clientX, innerWidth - 220), y: Math.min(e.clientY, innerHeight - 200), href: tab.href };
	}

	$effect(() => {
		if (!menu) return;
		const tutupMenu = () => (menu = null);
		const esc = (e: KeyboardEvent) => e.key === 'Escape' && tutupMenu();
		addEventListener('pointerdown', tutupMenu);
		addEventListener('blur', tutupMenu);
		addEventListener('resize', tutupMenu);
		addEventListener('keydown', esc);
		return () => {
			removeEventListener('pointerdown', tutupMenu);
			removeEventListener('blur', tutupMenu);
			removeEventListener('resize', tutupMenu);
			removeEventListener('keydown', esc);
		};
	});

	const menuIdx = $derived(tabs.findIndex((x) => x.href === menu?.href));

	function onKey(e: KeyboardEvent, tab: Tab) {
		if (e.key === 'Delete' || (e.key === 'w' && e.altKey)) {
			e.preventDefault();
			tutup(tab.href);
		}
	}
</script>

{#if tabs.length}
	<div class="flex items-stretch gap-2 border-t border-ink/10 bg-surface px-3 pt-2 lg:px-6">
		<div class="flex min-w-0 flex-1 items-end gap-1 overflow-x-auto" role="tablist" aria-label={t('tabs.label')}>
			{#each tabs as tab (tab.href)}
				{@const it = item(tab.href)}
				{@const on = tab.href === current}
				{#if it}
					<div
						role="presentation"
						oncontextmenu={(e) => bukaMenu(e, tab)}
						class="group flex shrink-0 items-center rounded-t-xl border border-b-0 text-xs font-semibold {on
							? 'border-ink/10 bg-page text-ink'
							: 'border-transparent text-ink/60 hover:bg-ink/5 hover:text-ink'}"
					>
						<button
							type="button"
							role="tab"
							aria-selected={on}
							onclick={() => buka(tab)}
							onkeydown={(e) => onKey(e, tab)}
							class="max-w-44 truncate py-2 pr-1 pl-3.5 focus-visible:ring-2 focus-visible:ring-emas/60"
						>
							{t(it.label)}
						</button>
						{#if tab.href !== HOME || tabs.length > 1}
							<button
								type="button"
								onclick={() => tutup(tab.href)}
								aria-label={t('tabs.close', { name: t(it.label) })}
								class="mr-1.5 grid size-5 place-items-center rounded-full text-ink/50 hover:bg-ink/10 hover:text-ink focus-visible:ring-2 focus-visible:ring-emas/60"
							>
								<svg viewBox="0 0 24 24" class="size-3" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18" /></svg>
							</button>
						{/if}
					</div>
				{/if}
			{/each}
		</div>
		<button
			type="button"
			onclick={tutupSemua}
			class="mb-1 shrink-0 self-center rounded-full border border-ink/10 px-3 py-1.5 text-[11px] font-semibold text-ink/70 hover:bg-ink/5 hover:text-ink focus-visible:ring-2 focus-visible:ring-emas/60"
		>
			{t('tabs.closeAll')}
		</button>
	</div>
{/if}
{#if menu}
	<div
		role="menu"
		tabindex="-1"
		class="fixed z-50 w-52 rounded-xl border border-ink/15 bg-surface p-1 text-xs font-semibold shadow-xl"
		style="left:{menu.x}px;top:{menu.y}px"
		onpointerdown={(e) => e.stopPropagation()}
		oncontextmenu={(e) => e.preventDefault()}
	>
		<button type="button" role="menuitem" class="block w-full rounded-lg px-3 py-2 text-left hover:bg-ink/5" onclick={() => aksi('close')}>{t('tabs.ctx.close')}</button>
		<button type="button" role="menuitem" class="block w-full rounded-lg px-3 py-2 text-left hover:bg-ink/5 disabled:opacity-40 disabled:hover:bg-transparent" disabled={tabs.length < 2} onclick={() => aksi('others')}>{t('tabs.ctx.others')}</button>
		<button type="button" role="menuitem" class="block w-full rounded-lg px-3 py-2 text-left hover:bg-ink/5 disabled:opacity-40 disabled:hover:bg-transparent" disabled={menuIdx >= tabs.length - 1} onclick={() => aksi('right')}>{t('tabs.ctx.right')}</button>
		<button type="button" role="menuitem" class="block w-full rounded-lg px-3 py-2 text-left hover:bg-ink/5 disabled:opacity-40 disabled:hover:bg-transparent" disabled={menuIdx <= 0} onclick={() => aksi('left')}>{t('tabs.ctx.left')}</button>
		<div class="my-1 border-t border-ink/10"></div>
		<button type="button" role="menuitem" class="block w-full rounded-lg px-3 py-2 text-left hover:bg-ink/5" onclick={() => aksi('all')}>{t('tabs.closeAll')}</button>
	</div>
{/if}
