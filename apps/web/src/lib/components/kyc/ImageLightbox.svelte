<script lang="ts">
	import { getI18n } from '../../i18n/i18n.svelte';
	const { t } = getI18n();
	// Pratinjau gambar layar penuh: klik latar / tombol tutup / Esc untuk menutup.
	let { src, alt = '', onclose }: { src: string; alt?: string; onclose: () => void } = $props();
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<div
	class="fixed inset-0 z-50 grid place-items-center bg-black/80 p-4"
	role="dialog"
	aria-modal="true"
	aria-label={alt || t('ky.light.preview')}
	tabindex="-1"
	onclick={onclose}
	onkeydown={(e) => e.key === 'Enter' && onclose()}
>
	<button
		type="button"
		aria-label={t('c.close')}
		class="absolute right-4 top-4 grid size-9 place-items-center rounded-full bg-white/90 text-[#1A2530] hover:bg-white"
		onclick={onclose}
	>
		<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M18 6 6 18M6 6l12 12" /></svg>
	</button>
	<!-- svelte-ignore a11y_no_noninteractive_element_interactions, a11y_click_events_have_key_events -->
	<img {src} {alt} class="max-h-[88vh] max-w-[94vw] rounded-md bg-white object-contain shadow-2xl" onclick={(e) => e.stopPropagation()} />
</div>
