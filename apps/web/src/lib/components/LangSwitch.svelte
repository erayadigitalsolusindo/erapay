<script lang="ts">
	// Pemilih bahasa (daftar pilihan). Pilihan disimpan di cookie supaya server merender bahasa yang sama.
	import { getI18n } from '../i18n/i18n.svelte';
	import { LANGS, LANG_NAMES, type Lang } from '../i18n/messages';

	let { btnClass = '', badgeClass = 'bg-merah text-white', align = 'right' }: { btnClass?: string; badgeClass?: string; align?: 'left' | 'right' } = $props();
	const i18n = getI18n();
	let open = $state(false);
	let root = $state<HTMLDivElement | null>(null);

	function pilih(l: Lang) {
		i18n.lang = l;
		document.cookie = `erapay_lang=${l}; path=/; max-age=31536000; samesite=lax`;
		open = false;
	}
</script>

<svelte:window
	onclick={(e) => {
		if (root && !root.contains(e.target as Node)) open = false;
	}}
	onkeydown={(e) => e.key === 'Escape' && (open = false)}
/>

<div class="relative" bind:this={root}>
	<button
		type="button"
		onclick={() => (open = !open)}
		aria-haspopup="listbox"
		aria-expanded={open}
		title={i18n.t('lang.pick')}
		aria-label="{i18n.t('hdr.language')}: {LANG_NAMES[i18n.lang]}"
		class="relative grid place-items-center {btnClass}"
	>
		<svg class="size-[18px]" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5h8M8 3v2m3 0c-.5 4-3 7-6 8m2-4c1 2 3 4 5 5M13 20l4-9 4 9m-1.5-3h-5" /></svg>
		<span class="absolute -right-1 -bottom-1 rounded-full px-1 text-[8px] leading-4 font-bold {badgeClass}">{i18n.lang.toUpperCase()}</span>
	</button>
	{#if open}
		<ul role="listbox" aria-label={i18n.t('lang.pick')} class="card absolute top-full z-40 list-none m-0 mt-2 w-44 p-1.5 text-neutral-800 shadow-lg {align === 'right' ? 'right-0' : 'left-0'}">
			{#each LANGS as l (l)}
				<li role="option" aria-selected={i18n.lang === l}>
					<button
						type="button"
						onclick={() => pilih(l)}
						class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-xs font-semibold hover:bg-ink/5 {i18n.lang === l ? 'text-merah' : ''}"
					>
						{LANG_NAMES[l]}
						{#if i18n.lang === l}<span aria-hidden="true">✓</span>{/if}
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>
