<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';

	let { data, form } = $props();
	const { t } = getI18n();
	let busy = $state(false);
</script>

<svelte:head><title>{t('ve.title')}</title></svelte:head>

<p class="text-sm font-semibold tracking-widest text-merah uppercase">{t('ve.eyebrow')}</p>
<h2 class="font-display mt-1 text-3xl font-extrabold text-merah-gelap">{t('ve.heading')}</h2>

{#if form?.done}
	<div class="mt-6 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-4 text-sm/6 text-emerald-900" role="status">
		{t('ve.done')}
	</div>
	<a href={form.loggedIn ? '/dashboard' : '/login'} class="mt-5 block w-full rounded-xl bg-gradient-to-b from-merah to-merah-tua px-5 py-3.5 text-center font-bold text-white shadow-lg shadow-merah/40">{form.loggedIn ? t('ve.toDash') : t('rs.login')}</a>
{:else if !data.token}
	<div class="mt-6 rounded-xl border border-red-200 bg-red-50 px-4 py-4 text-sm/6 text-red-800" role="alert">{t('ve.noToken')}</div>
{:else}
	<p class="mt-2 text-sm text-neutral-500">{t('ve.sub')}</p>
	{#if form?.error}
		<div class="mt-5 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">{form.error}</div>
	{/if}
	<form
		method="POST"
		class="mt-6"
		use:enhance={() => {
			busy = true;
			return async ({ update }) => {
				await update({ reset: false });
				busy = false;
			};
		}}
	>
		<input type="hidden" name="token" value={data.token} />
		<button
			type="submit"
			disabled={busy}
			class="w-full rounded-xl bg-gradient-to-b from-merah to-merah-tua px-5 py-3.5 font-bold text-white shadow-lg shadow-merah/40 transition hover:-translate-y-0.5 disabled:opacity-90"
		>{busy ? t('ve.busy') : t('ve.submit')}</button>
	</form>
{/if}
