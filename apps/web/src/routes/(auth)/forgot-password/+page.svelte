<script lang="ts">
	import { enhance } from '$app/forms';
	import { getContext, untrack } from 'svelte';
	import Field from '../../../lib/components/auth/Field.svelte';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';

	let { form } = $props();
	const { t } = getI18n();
	const auth = getContext<{ goyang(): void; pindah(href: string): void }>('auth');
	let email = $state(untrack(() => form?.email ?? ''));
	let busy = $state(false);

	$effect(() => {
		if (form?.error) auth.goyang();
	});
</script>

<svelte:head><title>{t('fp.title')}</title></svelte:head>

<p class="text-sm font-semibold tracking-widest text-merah uppercase">{t('rp.eyebrow')}</p>
<h2 class="font-display mt-1 text-3xl font-extrabold text-merah-gelap">{t('fp.heading')}</h2>

{#if form?.sent}
	<div class="mt-6 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-4 text-sm/6 text-emerald-900" role="status">
		{t('fp.sent', { email: form.email })}
	</div>
{:else}
	<p class="mt-2 text-sm text-neutral-500">{t('fp.sub')}</p>
	<form
		method="POST"
		class="mt-7 space-y-5"
		use:enhance={() => {
			busy = true;
			return async ({ update }) => {
				await update({ reset: false });
				busy = false;
			};
		}}
	>
		{#if form?.error}
			<div class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">{form.error}</div>
		{/if}
		<Field id="email" name="email" label="Email" icon="mail" type="email" autocomplete="username" placeholder={t('fp.emailPh')} bind:value={email} />
		<button
			type="submit"
			disabled={busy}
			class="relative flex w-full items-center justify-center gap-2 rounded-xl bg-gradient-to-b from-merah to-merah-tua px-5 py-3.5 font-bold text-white shadow-lg shadow-merah/40 transition hover:-translate-y-0.5 disabled:opacity-90"
		>{busy ? t('fp.busy') : t('fp.submit')}</button>
	</form>
{/if}

<p class="mt-6 text-center text-sm text-neutral-600">
	<a href="/login" onclick={(e) => { e.preventDefault(); auth.pindah('/login'); }} class="font-bold text-merah hover:underline">{t('fp.back')}</a>
</p>
