<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import type { Key } from '../../../lib/i18n/messages';

	let { data, form } = $props();
	const { t } = getI18n();
	const st = $derived(data.status);
	const state = $derived(!st?.enabled ? 'off' : st.active ? 'sandbox' : 'live');
	const err = $derived(form?.error === 'notApproved' ? t('sb.err.notApproved') : form?.error ? t('sb.err.generic') : '');
	const sim = `POST /v1/sandbox/payments/{id}/simulate
Authorization: Bearer epk_sbx_…
{"outcome": "paid"}   // paid | expired | failed`;
</script>

<h1 class="text-xl font-extrabold">{t('sb.title')}</h1>
<p class="muted mt-1 mb-4 max-w-2xl text-xs">{t('sb.sub')}</p>

{#if st === null}
	<p class="card p-5 text-xs font-semibold text-red-700" role="alert">{t('sb.loadFail')}</p>
{:else if !st.available}
	<p class="card p-5 text-xs font-semibold" role="status">{t('sb.unavailable')}</p>
{:else}
	<section class="card mb-4 p-5">
		<div class="flex flex-wrap items-center justify-between gap-4">
			<div>
				<div class="text-[10px] font-bold tracking-widest uppercase muted">{t('sb.state')}</div>
				<div class="mt-1 flex items-center gap-2 text-sm font-extrabold">
					<span class="inline-block size-2 rounded-full {state === 'sandbox' ? 'bg-emas' : state === 'live' ? 'bg-green-600' : 'bg-ink/30'}" aria-hidden="true"></span>
					{t(`sb.state.${state}` as Key)}
				</div>
			</div>
			<div class="flex flex-wrap gap-2">
				{#if state === 'off' && data.canEnable}
					<form method="POST" action="?/enable" use:enhance>
						<button class="btn rounded-xl bg-merah px-4 py-2 text-xs font-bold text-white hover:bg-merah-tua focus-visible:ring-2 focus-visible:ring-emas/60">{t('sb.enable')}</button>
					</form>
				{:else if state === 'live'}
					<form method="POST" action="?/switch">
						<input type="hidden" name="mode" value="sandbox" />
						<button class="btn rounded-xl bg-emas px-4 py-2 text-xs font-bold text-merah-gelap focus-visible:ring-2 focus-visible:ring-emas/60">{t('sb.enter')}</button>
					</form>
				{:else if state === 'sandbox'}
					<form method="POST" action="?/switch">
						<input type="hidden" name="mode" value="live" />
						<button class="btn rounded-xl bg-merah px-4 py-2 text-xs font-bold text-white hover:bg-merah-tua focus-visible:ring-2 focus-visible:ring-emas/60">{t('sb.leave')}</button>
					</form>
				{/if}
			</div>
		</div>
		{#if state === 'off' && data.canEnable}<p class="muted mt-3 text-xs">{t('sb.enable.hint')}</p>{/if}
		{#if err}<p class="mt-3 text-xs font-semibold text-red-700" role="alert">{err}</p>{/if}
	</section>

	<section class="card mb-4 p-5">
		<h2 class="mb-3 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">{t('sb.how.title')}</h2>
		<ol class="list-decimal space-y-1.5 pl-5 text-xs">
			<li>{t('sb.how.1')}</li>
			<li>{t('sb.how.2')}</li>
			<li>{t('sb.how.3')}</li>
		</ol>
	</section>

	{#if st.simulator}
		<section class="card mb-4 p-5">
			<h2 class="mb-2 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">{t('sb.sim.title')}</h2>
			<p class="mb-3 text-xs">{t('sb.sim.body')}</p>
			<pre class="overflow-x-auto rounded-xl bg-merah-gelap p-3 text-[11px] leading-relaxed text-white">{sim}</pre>
		</section>
	{/if}

	<section class="card p-5">
		<h2 class="mb-2 border-b border-dashed border-ink/20 pb-2 text-base font-extrabold">{t('sb.limits.title')}</h2>
		<p class="text-xs">{t('sb.limits.body')}</p>
	</section>
{/if}
