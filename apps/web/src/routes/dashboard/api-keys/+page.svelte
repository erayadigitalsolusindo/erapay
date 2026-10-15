<script lang="ts">
	import Combobox from '../../../lib/components/Combobox.svelte';
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	const i18n = getI18n();
	const { t } = i18n;
	let { data, form } = $props();
	let copied = $state(false);

	const fmt = (iso: string | null) => (iso ? new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' }) : '—');
	const status = (k: { revoked_at: string | null; expires_at: string | null }) =>
		k.revoked_at ? 'revoked' : k.expires_at && new Date(k.expires_at) < new Date() ? 'expired' : 'active';

	async function copy(text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = true;
		} catch {
			copied = false;
		}
	}
</script>

<p class="muted mb-5 max-w-2xl text-xs">{t('keys.note')}</p>

{#if form?.created}
	<section class="card mb-5 max-w-2xl space-y-3 border-emas bg-surface p-6" role="status">
		<h2 class="text-sm font-bold">{t('keys.copyNow')}</h2>
		<p class="muted text-xs">Setelah halaman ini ditutup, secret tidak bisa dilihat lagi.</p>
		<code class="block rounded-xl border border-ink/10 bg-page p-3 text-xs break-all select-all">{form.created.secret}</code>
		<button type="button" class="btn btn-light" onclick={() => copy(form.created.secret)}>{copied ? t('keys.copied') : t('keys.copy')}</button>
		<p class="muted text-[11px]">{t('keys.meta', { env: form.created.environment, scopes: form.created.scopes.join(', ') })}</p>
	</section>
{/if}
{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}

<form method="POST" action="?/create" class="card mb-6 max-w-md space-y-4 p-6" use:enhance>
	<h2 class="text-sm font-bold">{t('keys.create')}</h2>
	<div>
		<label class="lbl mb-1.5" for="name">{t('keys.name')}</label>
		<input id="name" class="inp" name="name" maxlength="80" required placeholder={t('keys.namePh')} value={form?.name ?? ''} />
	</div>
	<fieldset class="rounded-xl border border-ink/10 px-3 py-2">
		<legend class="px-1 text-xs font-bold">{t('keys.scopes')}</legend>
		{#each data.scopes as s (s.id)}
			<label class="flex items-center gap-2 py-1 text-xs"><input type="checkbox" class="accent-merah" name="scopes" value={s.id} checked={s.id !== 'report:read'} /> {s.label} <span class="muted text-[11px]">({s.id})</span></label>
		{/each}
	</fieldset>
	<button type="submit" class="btn btn-red">{t('keys.createBtn')}</button>
</form>

<div class="card overflow-hidden">
	<div class="p-5"><h2 class="text-sm font-bold">{t('keys.list')}</h2></div>
	{#if data.keys === null}
		<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('keys.loadFail')}</p>
	{:else if data.keys.length === 0}
		<p class="muted px-5 pb-5 text-xs">{t('keys.none')}</p>
	{:else}
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>{t('keys.col.name')}</th><th>{t('keys.col.key')}</th><th>{t('keys.col.scopes')}</th><th>{t('keys.col.status')}</th><th>{t('keys.col.lastUsed')}</th><th></th></tr></thead>
				<tbody>
					{#each data.keys as k (k.id)}
						<tr>
							<td class="font-bold">{k.name}</td>
							<td><code>epk_{k.environment === 'production' ? 'prd' : 'sbx'}_{k.prefix}_…</code></td>
							<td>{k.scopes.join(', ')}</td>
							<td>
								<span class="pill {status(k) === 'active' ? 'bg-emerald-100 text-emerald-800' : status(k) === 'revoked' ? 'bg-rose-100 text-rose-800' : 'bg-ink/10 text-ink'}">{t(`keys.st.${status(k)}` as const)}</span>
							</td>
							<td class="muted">{fmt(k.last_used_at)}</td>
							<td>
								{#if status(k) === 'active'}
									<div class="flex flex-wrap items-center gap-2">
										<form method="POST" action="?/rotate" use:enhance class="flex items-center gap-2">
											<input type="hidden" name="id" value={k.id} />
											<div class="muted flex items-center gap-1 text-[11px]"><span>{t('keys.grace')}</span>
												<Combobox id={`grace-${k.id}`} name="grace" class="w-28" value="3600" options={[{ value: '0', label: '0' }, { value: '3600', label: t('keys.grace.1h') }, { value: '86400', label: t('keys.grace.24h') }]} />
											</div>
											<button type="submit" class="btn btn-light">{t('keys.rotate')}</button>
										</form>
										<form
											method="POST"
											action="?/revoke"
											use:enhance={({ cancel }) => {
												if (!confirm(t('keys.revokeConfirm', { name: k.name }))) cancel();
											}}
										>
											<input type="hidden" name="id" value={k.id} />
											<button type="submit" class="btn btn-danger">{t('keys.revoke')}</button>
										</form>
									</div>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
