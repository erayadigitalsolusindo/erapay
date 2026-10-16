<script lang="ts">
	// Pemilih bahasa berupa pop-up (modal) dengan bendera. Pilihan disimpan di cookie supaya server merender bahasa yang sama.
	// Bendera memakai SVG, bukan emoji: Windows tidak menggambar emoji bendera (hanya tampil huruf "ID", "GB").
	import { tick } from 'svelte';
	import { getI18n } from '../i18n/i18n.svelte';
	import { LANGS, LANG_NAMES, type Lang } from '../i18n/messages';

	let { btnClass = '', badgeClass = 'bg-merah text-white' }: { btnClass?: string; badgeClass?: string; align?: 'left' | 'right' } = $props();
	const i18n = getI18n();
	let open = $state(false);
	let trigger = $state<HTMLButtonElement | null>(null);
	let dialog = $state<HTMLDivElement | null>(null);

	const NATIVE: Record<Lang, string> = { id: 'Indonesia', en: 'English', ja: '日本' };

	function pilih(l: Lang) {
		i18n.lang = l;
		document.cookie = `erapay_lang=${l}; path=/; max-age=31536000; samesite=lax`;
		tutup();
	}
	async function buka() {
		open = true;
		await tick();
		dialog?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus();
	}
	function tutup() {
		open = false;
		trigger?.focus();
	}
	// Pindahkan ke <body> supaya position:fixed tidak terjebak induk yang punya transform/filter (sidebar, header blur).
	function portal(node: HTMLElement) {
		document.body.appendChild(node);
		return { destroy: () => node.remove() };
	}
	// Tab berputar di dalam dialog.
	function trap(e: KeyboardEvent) {
		if (e.key === 'Escape') return tutup();
		if (e.key !== 'Tab' || !dialog) return;
		const f = [...dialog.querySelectorAll<HTMLElement>('button')];
		if (!f.length) return;
		const first = f[0];
		const last = f[f.length - 1];
		if (e.shiftKey && document.activeElement === first) (e.preventDefault(), last.focus());
		else if (!e.shiftKey && document.activeElement === last) (e.preventDefault(), first.focus());
	}
</script>

{#snippet flag(l: Lang)}
	<svg viewBox="0 0 60 40" class="h-8 w-12 shrink-0 rounded-md shadow-sm ring-1 ring-black/15" aria-hidden="true">
		{#if l === 'id'}
			<rect width="60" height="20" fill="#e70011" /><rect y="20" width="60" height="20" fill="#fff" />
		{:else if l === 'ja'}
			<rect width="60" height="40" fill="#fff" /><circle cx="30" cy="20" r="11" fill="#bc002d" />
		{:else}
			<clipPath id="ls-uk"><rect width="60" height="40" /></clipPath>
			<g clip-path="url(#ls-uk)">
				<rect width="60" height="40" fill="#012169" />
				<path d="M0 0 60 40M60 0 0 40" stroke="#fff" stroke-width="8" />
				<path d="M0 0 60 40M60 0 0 40" stroke="#c8102e" stroke-width="3" />
				<path d="M30 0v40M0 20h60" stroke="#fff" stroke-width="13" />
				<path d="M30 0v40M0 20h60" stroke="#c8102e" stroke-width="7" />
			</g>
		{/if}
	</svg>
{/snippet}

<button
	bind:this={trigger}
	type="button"
	onclick={buka}
	aria-haspopup="dialog"
	aria-expanded={open}
	title={i18n.t('lang.pick')}
	aria-label="{i18n.t('hdr.language')}: {LANG_NAMES[i18n.lang]}"
	class="relative grid place-items-center {btnClass}"
>
	<svg class="size-[18px]" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5h8M8 3v2m3 0c-.5 4-3 7-6 8m2-4c1 2 3 4 5 5M13 20l4-9 4 9m-1.5-3h-5" /></svg>
	<span class="absolute -right-1 -bottom-1 rounded-full px-1 text-[8px] leading-4 font-bold {badgeClass}">{i18n.lang.toUpperCase()}</span>
</button>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div use:portal class="fixed inset-0 z-[100] grid place-items-center bg-black/55 p-4 backdrop-blur-sm motion-safe:animate-[lsfade_.15s_ease-out]" onmousedown={(e) => e.target === e.currentTarget && tutup()} onkeydown={trap}>
		<div bind:this={dialog} role="dialog" aria-modal="true" aria-label={i18n.t('lang.pick')} class="w-full max-w-sm overflow-hidden rounded-3xl bg-white text-neutral-800 shadow-2xl motion-safe:animate-[lspop_.2s_cubic-bezier(.2,1.2,.4,1)]">
			<div class="h-1.5 bg-linear-to-r from-merah via-emas to-merah-tua"></div>
			<div class="p-5">
				<div class="flex items-start justify-between gap-3">
					<div>
						<h2 class="font-display text-lg font-extrabold text-merah-gelap">{i18n.t('lang.pick')}</h2>
						<p class="mt-0.5 text-[11px] text-neutral-500">{i18n.t('hdr.language')}</p>
					</div>
					<button type="button" onclick={tutup} aria-label="Close" class="grid size-8 place-items-center rounded-full text-neutral-500 transition hover:bg-neutral-100">
						<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><path d="M6 6l12 12M18 6 6 18" /></svg>
					</button>
				</div>
				<div role="radiogroup" aria-label={i18n.t('lang.pick')} class="mt-4 space-y-2">
					{#each LANGS as l (l)}
						{@const on = i18n.lang === l}
						<button
							type="button"
							role="radio"
							aria-checked={on}
							onclick={() => pilih(l)}
							class="flex w-full items-center gap-3.5 rounded-2xl border-2 px-3.5 py-3 text-left transition hover:-translate-y-px hover:shadow-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emas {on ? 'border-merah bg-merah/5' : 'border-neutral-200 hover:border-merah/40'}"
						>
							{@render flag(l)}
							<span class="min-w-0 flex-1">
								<span class="block text-sm font-extrabold {on ? 'text-merah' : ''}">{LANG_NAMES[l]}</span>
								<span class="block text-[10px] font-bold tracking-widest text-neutral-400 uppercase">{l} · {NATIVE[l]}</span>
							</span>
							<span class="grid size-6 place-items-center rounded-full text-xs font-extrabold {on ? 'bg-merah text-white' : 'border border-neutral-300 text-transparent'}" aria-hidden="true">✓</span>
						</button>
					{/each}
				</div>
			</div>
		</div>
	</div>
{/if}

<style>
	@keyframes -global-lsfade {
		from {
			opacity: 0;
		}
	}
	@keyframes -global-lspop {
		from {
			opacity: 0;
			transform: translateY(8px) scale(0.94);
		}
	}
</style>
