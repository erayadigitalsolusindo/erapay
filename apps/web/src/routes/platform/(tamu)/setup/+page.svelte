<script lang="ts">
	import { enhance } from '$app/forms';
	import Field from '../../../../lib/components/auth/Field.svelte';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';

	let { data, form } = $props();
	const { t } = getI18n();
	let busy = $state(false);
	let token = $state('');
	let email = $state('');
	let password = $state('');
	const status = $derived(data.status);
	const outcome = $derived(form?.status);
</script>

<svelte:head><title>{t('pf.setup.pageTitle')}</title></svelte:head>

{#if outcome === 'created'}
	<div class="text-center">
		<div class="mx-auto grid size-14 place-items-center rounded-full bg-emerald-100 text-emerald-700">✓</div>
		<h2 class="mt-4 text-lg font-bold text-neutral-800">{t('pf.setup.created')}</h2>
		<p class="mt-2 text-sm text-neutral-600">
			{t('pf.setup.createdNote')}
			<a href="/platform/login" class="font-bold text-merah underline">{t('pf.setup.goLogin')}</a>
		</p>
	</div>
{:else if status === null}
	<p class="text-sm text-red-700">{t('pf.setup.loadFail')}</p>
{:else if status.done}
	<div class="text-center">
		<div class="mx-auto grid size-14 place-items-center rounded-full bg-emerald-100 text-emerald-700">✓</div>
		<h2 class="mt-4 text-lg font-bold text-neutral-800">{t('pf.setup.done')}</h2>
		<p class="mt-2 text-sm text-neutral-600">
			{t('pf.setup.doneNote')} <a href="/platform/login" class="font-bold text-merah underline">{t('pf.setup.goLogin')}</a>
		</p>
	</div>
{:else if !status.enabled}
	<p class="text-sm text-neutral-600">
		{t('pf.setup.disabled')}
		<a href="/platform/login" class="font-bold text-merah underline">{t('pf.setup.goLogin')}</a>
	</p>
{:else}
	<div>
		{#if form?.message}
			<div class="mb-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">{form.message}</div>
		{/if}
		<p class="mb-4 text-sm text-neutral-600">{t('pf.setup.intro')}</p>
		<form method="POST" novalidate class="space-y-4" use:enhance={() => {
			busy = true;
			return async () => { busy = false; };
		}}>
			<Field id="token" name="token" label={t('pf.setup.token')} icon="lock" autocomplete="off" placeholder="ERAPAY_PLATFORM_SETUP_TOKEN" bind:value={token} />
			<Field id="email" name="email" label={t('pf.setup.email')} icon="mail" type="email" autocomplete="off" placeholder="dewa@erapay.test" bind:value={email} />
			<Field id="password" name="password" label={t('login.pw')} icon="lock" type="password" autocomplete="new-password" placeholder={t('pf.setup.pwPh')} bind:value={password} />
			<button
				type="submit"
				disabled={busy}
				class="w-full rounded-xl bg-gradient-to-b from-merah to-merah-tua px-5 py-3.5 font-bold text-white shadow-lg shadow-merah/40 transition hover:-translate-y-0.5 disabled:opacity-90"
			>
				{t('pf.setup.submit')}
			</button>
		</form>
	</div>
{/if}
