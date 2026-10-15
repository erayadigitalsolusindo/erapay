<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t, tx } = i18n;
	let open = $state<string | null>(null);
	let act = $state<string | null>(null);
	const fmt = (s: string) => new Date(s).toLocaleDateString(i18n.locale, { day: '2-digit', month: 'short', year: 'numeric' });
</script>

<svelte:head><title>{t('pf.tn.pageTitle')}</title></svelte:head>

<div class="space-y-6">
	<div class="card p-6">
		<h1 class="text-xl font-bold">{t('pf.nav.tenants')}</h1>
		<p class="muted mt-1 text-sm">
			{t('pf.tn.sub')}
			<a href="/platform/audit" class="underline">{t('pf.tn.subLink')}</a>
		</p>
		<form method="GET" class="mt-4 flex gap-2">
			<input name="q" value={data.q} placeholder={t('pf.tn.searchPh')} class="w-full max-w-sm rounded-xl border border-ink/15 bg-surface px-3 py-2 text-sm" />
			<button class="rounded-full bg-merah px-4 py-2 text-xs font-bold text-white">{t('pf.tn.search')}</button>
		</form>
	</div>

	{#if data.error}<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{data.error}</p>{/if}

	<div class="card divide-y divide-ink/10">
		{#each data.merchants as m (m.id)}
			<div class="p-4">
				<div class="flex flex-wrap items-center justify-between gap-3">
					<div class="min-w-0">
						<div class="truncate text-sm font-bold">{m.display_name}</div>
						<div class="muted text-xs">{m.legal_name} · {tx(`onb.st.${m.onboarding_status}`, m.onboarding_status)} · {fmt(m.created_at)} · <code class="font-mono text-[11px]">{m.id}</code></div>
					</div>
					<div class="flex gap-2">
						<button
							type="button"
							onclick={() => (act = act === m.id ? null : m.id)}
							class="rounded-full border border-ink/30 px-3.5 py-1.5 text-xs font-bold transition hover:bg-ink hover:text-white"
						>
							{t('pf.tn.activate')}
						</button>
						<button
							type="button"
							onclick={() => (open = open === m.id ? null : m.id)}
							class="rounded-full border border-merah px-3.5 py-1.5 text-xs font-bold text-merah transition hover:bg-merah hover:text-white"
						>
							{t('pf.tn.enter')}
						</button>
					</div>
				</div>
				{#if act === m.id || (form?.merchantId === m.id && (form?.activateError || form?.activated))}
					<form method="POST" action="?/activate" use:enhance autocomplete="off" class="mt-3 space-y-2">
						<input type="hidden" name="merchant_id" value={m.id} />
						<p class="muted text-xs">{t('pf.tn.activateHint')}</p>
						<div class="flex flex-wrap items-start gap-2">
							<input name="password" type="password" minlength="12" maxlength="128" autocomplete="new-password" placeholder={t('pf.tn.pwPh')} class="min-w-64 flex-1 rounded-xl border border-ink/15 bg-surface px-3 py-2 text-sm" />
							<input name="note" required minlength="3" maxlength="500" placeholder={t('pf.tn.notePh')} class="min-w-64 flex-1 rounded-xl border border-ink/15 bg-surface px-3 py-2 text-sm" />
						</div>
						<div class="flex flex-wrap gap-2">
							<button name="mode" value="password" class="rounded-full bg-merah px-4 py-2 text-xs font-bold text-white">{t('pf.tn.setPw')}</button>
							<button name="mode" value="verify" formnovalidate class="rounded-full border border-ink/30 px-4 py-2 text-xs font-bold">{t('pf.tn.verifyOnly')}</button>
						</div>
						{#if form?.merchantId === m.id && form.activateError}
							<p class="text-xs font-semibold text-red-700" role="alert">{form.activateError}</p>
						{:else if form?.merchantId === m.id && form.activated}
							<p class="text-xs font-semibold text-green-700">{t('pf.tn.done')}</p>
						{/if}
					</form>
				{/if}
				{#if open === m.id || form?.merchantId === m.id}
					<form method="POST" action="?/impersonate" use:enhance class="mt-3 flex flex-wrap items-start gap-2">
						<input type="hidden" name="merchant_id" value={m.id} />
						<textarea
							name="reason"
							required
							minlength="10"
							maxlength="500"
							rows="2"
							placeholder={t('pf.tn.reasonPh')}
							class="min-w-64 flex-1 rounded-xl border border-ink/15 bg-surface px-3 py-2 text-sm">{form?.merchantId === m.id ? form.reason : ''}</textarea>
						<button class="rounded-full bg-merah px-4 py-2 text-xs font-bold text-white">{t('pf.tn.submit')}</button>
						{#if form?.merchantId === m.id && form.message}
							<p class="w-full text-xs font-semibold text-red-700" role="alert">{form.message}</p>
						{/if}
					</form>
				{/if}
			</div>
		{:else}
			<p class="muted p-6 text-sm">{t('pf.tn.none')}</p>
		{/each}
	</div>
</div>
