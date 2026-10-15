<script lang="ts">
	import { getI18n } from '../../../lib/i18n/i18n.svelte';

	let { data } = $props();
	const { t } = getI18n();
</script>

<div class="card mb-5 p-5">
	{#if data.profile === null}
		<p class="text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else}
		<dl class="grid max-w-xl gap-3 text-sm sm:grid-cols-2">
			<div><dt class="muted text-xs">{t('mm.legalName')}</dt><dd class="font-bold">{data.profile.legal_name}</dd></div>
			<div><dt class="muted text-xs">{t('mm.displayName')}</dt><dd class="font-bold">{data.profile.display_name}</dd></div>
			<div><dt class="muted text-xs">{t('c.status')}</dt><dd class="font-bold">{data.profile.onboarding_status}</dd></div>
		</dl>
		{#if data.profile.review_note}<p class="muted mt-3 text-xs">{data.profile.review_note}</p>{/if}
	{/if}
</div>

<div class="card overflow-hidden">
	<div class="p-5"><h2 class="text-sm font-bold">{t('mm.outlets')}</h2></div>
	{#if data.outlets === null}
		<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else if data.outlets.length === 0}
		<p class="muted px-5 pb-5 text-xs">{t('mm.noOutlets')}</p>
	{:else}
		<ul class="px-5 pb-5 text-sm">
			{#each data.outlets as o (o.id)}<li class="py-1 font-semibold">{o.name}</li>{/each}
		</ul>
	{/if}
</div>
