<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../lib/components/StateBadge.svelte';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t } = i18n;
	let copied = $state(false);
	const fmt = (iso: string | null) => (iso ? new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' }) : '—');
	const urlOf = (id: string) => data.endpoints?.find((e) => e.id === id)?.url ?? id.slice(0, 8);
	const filters = ['', 'pending', 'succeeded', 'dead', 'cancelled'] as const;

	async function copy(text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = true;
		} catch {
			copied = false;
		}
	}
</script>

<p class="muted mb-5 max-w-2xl text-xs">{t('wh.note')}</p>

{#if form?.created?.secret}
	<section class="card mb-5 max-w-2xl space-y-3 border-emas bg-surface p-6" role="status">
		<h2 class="text-sm font-bold">{t('wh.secretNow')}</h2>
		<p class="muted text-xs">{t('wh.secretWarn')}</p>
		<code class="block rounded-xl border border-ink/10 bg-page p-3 text-xs break-all select-all">{form.created.secret}</code>
		<button type="button" class="btn btn-light" onclick={() => copy(form.created.secret ?? '')}>{copied ? t('keys.copied') : t('keys.copy')}</button>
	</section>
{/if}
{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}

<form method="POST" action="?/create" class="card mb-6 max-w-xl space-y-4 p-6" use:enhance>
	<h2 class="text-sm font-bold">{t('wh.add')}</h2>
	<div>
		<label class="lbl mb-1.5" for="url">{t('wh.url')}</label>
		<input id="url" class="inp" name="url" type="url" inputmode="url" placeholder="https://toko.example/erapay/webhook" required />
	</div>
	<button type="submit" class="btn btn-red">{t('wh.add')}</button>
</form>

<div class="card mb-6 overflow-hidden">
	<div class="p-5"><h2 class="text-sm font-bold">{t('wh.endpoints')}</h2></div>
	{#if data.endpoints === null}
		<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else if data.endpoints.length === 0}
		<p class="muted px-5 pb-5 text-xs">{t('wh.none')}</p>
	{:else}
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>URL</th><th>{t('wh.hint')}</th><th>{t('c.status')}</th><th>{t('c.created')}</th><th></th></tr></thead>
				<tbody>
					{#each data.endpoints as e (e.id)}
						<tr>
							<td class="font-bold">{e.url}</td>
							<td><code>…{e.secret_hint}</code></td>
							<td><StateBadge kind="wh" value={e.status} /></td>
							<td class="muted">{fmt(e.created_at)}</td>
							<td>
								{#if e.status === 'active'}
									<div class="flex gap-2">
										<form
											method="POST"
											action="?/rotate"
											use:enhance={({ cancel }) => {
												if (!confirm(t('wh.rotateConfirm'))) cancel();
											}}
										>
											<input type="hidden" name="id" value={e.id} />
											<button type="submit" class="btn btn-light">{t('wh.rotate')}</button>
										</form>
										<form
											method="POST"
											action="?/disable"
											use:enhance={({ cancel }) => {
												if (!confirm(t('wh.disableConfirm'))) cancel();
											}}
										>
											<input type="hidden" name="id" value={e.id} />
											<button type="submit" class="btn btn-danger">{t('wh.disable')}</button>
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

<div class="card overflow-hidden">
	<div class="flex flex-wrap items-center justify-between gap-3 p-5">
		<h2 class="text-sm font-bold">{t('wh.deliveries')}</h2>
		<nav class="flex flex-wrap gap-1.5" aria-label={t('fin.filter')}>
			{#each filters as s (s)}
				<a
					href={s ? `?status=${s}` : '?'}
					aria-current={data.status === s ? 'true' : undefined}
					class="rounded-full border border-ink/10 px-3 py-1 text-[11px] font-bold hover:bg-ink/5 aria-[current=true]:bg-merah aria-[current=true]:text-white"
					>{s ? t(`wh.d.${s}` as const) : t('wh.all')}</a
				>
			{/each}
		</nav>
	</div>
	{#if data.deliveries === null}
		<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else if data.deliveries.length === 0}
		<p class="muted px-5 pb-5 text-xs">{t('wh.dnone')}</p>
	{:else}
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>{t('c.created')}</th><th>{t('wh.event')}</th><th>URL</th><th>{t('c.status')}</th><th>{t('wh.attempts')}</th><th>{t('wh.http')}</th><th>{t('wh.error')}</th><th></th></tr></thead>
				<tbody>
					{#each data.deliveries as d (d.id)}
						<tr>
							<td class="muted">{fmt(d.created_at)}</td>
							<td class="font-bold">{d.event_type}{#if d.redelivery_of}<span class="muted ml-1 font-normal">↻</span>{/if}</td>
							<td class="max-w-56 truncate" title={urlOf(d.endpoint_id)}>{urlOf(d.endpoint_id)}</td>
							<td>
								<StateBadge kind="wd" value={d.status} />
								{#if d.next_attempt_at}<div class="muted mt-1 text-[10px]">{t('wh.next')}: {fmt(d.next_attempt_at)}</div>{/if}
							</td>
							<td>{d.attempts}</td>
							<td>{d.last_http_status ?? '—'}</td>
							<td class="muted">{d.last_error ?? '—'}</td>
							<td>
								{#if d.status !== 'pending'}
									<form method="POST" action="?/redeliver" use:enhance>
										<input type="hidden" name="id" value={d.id} />
										<button type="submit" class="btn btn-light">{t('wh.redeliver')}</button>
									</form>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
