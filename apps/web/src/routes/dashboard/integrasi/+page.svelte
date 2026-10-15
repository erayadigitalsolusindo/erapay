<script lang="ts">
	import { withConfirm } from '../../../lib/ui/confirm';
	import Combobox from '../../../lib/components/Combobox.svelte';
	import StateBadge from '../../../lib/components/StateBadge.svelte';
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';

	const i18n = getI18n();
	const { t } = i18n;
	let { data, form } = $props();
	let tab = $state<'keys' | 'webhooks'>(data.tab as 'keys' | 'webhooks');
	let copied = $state('');

	const fmt = (iso: string | null) => (iso ? new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' }) : '—');
	const keyState = (k: { revoked_at: string | null; expires_at: string | null }) =>
		k.revoked_at ? 'revoked' : k.expires_at && new Date(k.expires_at) < new Date() ? 'expired' : 'active';
	const urlOf = (id: string) => data.endpoints?.find((e) => e.id === id)?.url ?? id.slice(0, 8);
	const filters = ['', 'pending', 'succeeded', 'dead', 'cancelled'] as const;
	const activeKeys = $derived((data.keys ?? []).filter((k) => keyState(k) === 'active').length);
	const activeHooks = $derived((data.endpoints ?? []).filter((e) => e.status === 'active').length);
	const failed = $derived((data.deliveries ?? []).filter((d) => d.status === 'dead').length);

	async function copy(id: string, text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = id;
			setTimeout(() => (copied = ''), 1800);
		} catch {
			copied = '';
		}
	}

	const curl = `curl -X POST https://<host-api-erapay>/v1/payments \\
  -H "Authorization: Bearer epk_sbx_…" \\
  -H "Idempotency-Key: order-1001-v1" \\
  -H "Content-Type: application/json" \\
  -d '{"merchant_reference":"INV-1001","amount":25000,"currency":"IDR"}'`;
	const sig = `X-EraPay-Signature: t=<unix>,v1=<hex>
v1 = HMAC_SHA256(secret, "<t>." + raw_body)`;
</script>

<!-- header terminal -->
<section class="hero relative mb-5 overflow-hidden rounded-2xl border border-white/10 bg-[#0b1020] p-5 font-mono text-slate-200 shadow-lg sm:p-6">
	<div class="pointer-events-none absolute inset-0 opacity-[0.08]" style="background-image:linear-gradient(#fff 1px,transparent 1px),linear-gradient(90deg,#fff 1px,transparent 1px);background-size:28px 28px"></div>
	<div class="pointer-events-none absolute -right-20 -top-24 size-64 rounded-full bg-emerald-400/15 blur-3xl"></div>
	<div class="relative">
		<div class="flex items-center gap-1.5" aria-hidden="true"><span class="size-2.5 rounded-full bg-red-400/80"></span><span class="size-2.5 rounded-full bg-amber-300/80"></span><span class="size-2.5 rounded-full bg-emerald-400/80"></span><span class="ml-3 text-[11px] text-slate-500">erapay — integrasi</span></div>
		<h1 class="mt-4 text-2xl font-bold tracking-tight text-white sm:text-3xl"><span class="text-emerald-400">$</span> integrasi<span class="cursor">▍</span></h1>
		<p class="mt-2 max-w-2xl text-[12px] leading-relaxed text-slate-400">{t('ig.intro')}</p>
		<dl class="mt-5 grid grid-cols-3 gap-2 text-[11px] sm:max-w-lg">
			<div class="rounded-lg border border-white/10 bg-white/5 px-3 py-2"><dt class="text-slate-500">api_keys.active</dt><dd class="mt-0.5 text-xl font-bold text-emerald-300">{data.canKeys ? activeKeys : '—'}</dd></div>
			<div class="rounded-lg border border-white/10 bg-white/5 px-3 py-2"><dt class="text-slate-500">endpoints.active</dt><dd class="mt-0.5 text-xl font-bold text-sky-300">{data.canHooks ? activeHooks : '—'}</dd></div>
			<div class="rounded-lg border border-white/10 bg-white/5 px-3 py-2"><dt class="text-slate-500">deliveries.dead</dt><dd class="mt-0.5 text-xl font-bold {failed ? 'text-rose-300' : 'text-slate-300'}">{data.canHooks ? failed : '—'}</dd></div>
		</dl>
	</div>
</section>

<!-- tab -->
<div class="mb-5 flex gap-1 border-b border-ink/10 font-mono text-xs font-bold" role="tablist">
	{#if data.canKeys}
		<button type="button" role="tab" aria-selected={tab === 'keys'} class="tab" class:on={tab === 'keys'} onclick={() => (tab = 'keys')}>{'{ }'} {t('ig.tabKeys')}</button>
	{/if}
	{#if data.canHooks}
		<button type="button" role="tab" aria-selected={tab === 'webhooks'} class="tab" class:on={tab === 'webhooks'} onclick={() => (tab = 'webhooks')}>↯ Webhook</button>
	{/if}
</div>

{#if form?.message}<p class="mb-4 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}

{#snippet secretBox(id: string, title: string, warn: string, secret: string)}
	<section class="mb-5 overflow-hidden rounded-xl border border-amber-400/60 bg-[#0b1020] font-mono text-slate-200 shadow-md" role="status">
		<div class="flex flex-wrap items-center justify-between gap-2 border-b border-white/10 bg-amber-400/10 px-4 py-2.5">
			<h2 class="text-xs font-bold text-amber-300">⚠ {title}</h2>
			<button type="button" class="rounded-md border border-white/20 px-2.5 py-1 text-[11px] font-bold hover:bg-white/10" onclick={() => copy(id, secret)}>{copied === id ? t('keys.copied') : t('keys.copy')}</button>
		</div>
		<code class="block px-4 py-3 text-[12px] break-all text-emerald-300 select-all">{secret}</code>
		<p class="border-t border-white/10 px-4 py-2 text-[11px] text-slate-400">{warn}</p>
	</section>
{/snippet}

{#if tab === 'keys' && data.canKeys}
	<p class="muted mb-4 max-w-2xl text-xs">{t('keys.note')}</p>
	{#if form?.createdKey}
		{@render secretBox('key', t('keys.copyNow'), `${t('keys.copyWarn')} · ${t('keys.meta', { env: i18n.tx(`env.${form.createdKey.environment}`, form.createdKey.environment), scopes: form.createdKey.scopes.join(', ') })}`, form.createdKey.secret)}
	{/if}

	<div class="mb-5 grid gap-4 lg:grid-cols-[minmax(0,26rem)_1fr]">
		<form method="POST" action="?/keyCreate" class="card space-y-4 p-5" use:enhance>
			<h2 class="font-mono text-sm font-bold">+ {t('keys.create')}</h2>
			<div>
				<label class="lbl mb-1.5" for="name">{t('keys.name')}</label>
				<input id="name" class="inp font-mono" name="name" maxlength="80" required placeholder={t('keys.namePh')} value={form?.name ?? ''} />
			</div>
			<fieldset class="rounded-xl border border-ink/10 px-3 py-2">
				<legend class="px-1 text-xs font-bold">{t('keys.scopes')}</legend>
				{#each data.scopes as s (s.id)}
					<label class="flex items-center gap-2 py-1 text-xs"><input type="checkbox" class="accent-merah" name="scopes" value={s.id} checked={s.id !== 'report:read'} /> {i18n.tx(`scope.${s.id}`, s.label)} <code class="muted text-[11px]">{s.id}</code></label>
				{/each}
			</fieldset>
			<button type="submit" class="btn btn-red">{t('keys.createBtn')}</button>
		</form>

		<div class="overflow-hidden rounded-xl border border-white/10 bg-[#0b1020] font-mono text-[11px] text-slate-300 shadow-sm">
			<div class="flex items-center justify-between border-b border-white/10 px-4 py-2 text-slate-500"><span>quickstart.sh</span>
				<button type="button" class="rounded px-2 py-0.5 font-bold text-slate-300 hover:bg-white/10" onclick={() => copy('curl', curl)}>{copied === 'curl' ? t('keys.copied') : t('keys.copy')}</button></div>
			<pre class="overflow-x-auto p-4 leading-relaxed"><code>{curl}</code></pre>
			<p class="border-t border-white/10 px-4 py-2 text-slate-500">{t('ig.idemNote')}</p>
		</div>
	</div>

	<div class="card overflow-hidden">
		<div class="p-5"><h2 class="font-mono text-sm font-bold">{t('keys.list')}</h2></div>
		{#if data.keys === null}
			<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('keys.loadFail')}</p>
		{:else if data.keys.length === 0}
			<p class="muted px-5 pb-5 font-mono text-xs">{t('keys.none')}</p>
		{:else}
			<div class="overflow-x-auto">
				<table>
					<thead><tr><th>{t('keys.col.name')}</th><th>{t('keys.col.key')}</th><th>{t('keys.col.scopes')}</th><th>{t('keys.col.status')}</th><th>{t('keys.col.lastUsed')}</th><th></th></tr></thead>
					<tbody>
						{#each data.keys as k (k.id)}
							<tr>
								<td class="font-bold">{k.name}</td>
								<td><code class="rounded bg-ink/5 px-1.5 py-0.5">epk_{k.environment === 'production' ? 'prd' : 'sbx'}_{k.prefix}_…</code></td>
								<td>{#each k.scopes as sc (sc)}<code class="mr-1 rounded bg-sky-500/10 px-1.5 py-0.5 text-[10px] text-sky-700">{sc}</code>{/each}</td>
								<td><span class="pill {keyState(k) === 'active' ? 'bg-emerald-100 text-emerald-800' : keyState(k) === 'revoked' ? 'bg-rose-100 text-rose-800' : 'bg-ink/10 text-ink'}">{t(`keys.st.${keyState(k)}` as const)}</span></td>
								<td class="muted">{fmt(k.last_used_at)}</td>
								<td>
									{#if keyState(k) === 'active'}
										<div class="flex flex-wrap items-center gap-2">
											<form method="POST" action="?/keyRotate" use:enhance class="flex items-center gap-2">
												<input type="hidden" name="id" value={k.id} />
												<div class="muted flex items-center gap-1 text-[11px]"><span>{t('keys.grace')}</span>
													<Combobox id={`grace-${k.id}`} name="grace" class="w-28" value="3600" options={[{ value: '0', label: '0' }, { value: '3600', label: t('keys.grace.1h') }, { value: '86400', label: t('keys.grace.24h') }]} />
												</div>
												<button type="submit" class="btn btn-light">{t('keys.rotate')}</button>
											</form>
											<form method="POST" action="?/keyRevoke" use:enhance={withConfirm({ variant: 'danger', title: 'Cabut key', successMessage: 'Key dicabut.', confirmText: 'Ya, cabut', message: t('keys.revokeConfirm', { name: k.name }) })}>
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
{/if}

{#if tab === 'webhooks' && data.canHooks}
	<p class="muted mb-4 max-w-2xl text-xs">{t('wh.note')}</p>
	{#if form?.createdHook?.secret}
		{@render secretBox('hook', t('wh.secretNow'), t('wh.secretWarn'), form.createdHook.secret)}
	{/if}

	<div class="mb-5 grid gap-4 lg:grid-cols-[minmax(0,26rem)_1fr]">
		<form method="POST" action="?/hookCreate" class="card space-y-4 p-5" use:enhance>
			<h2 class="font-mono text-sm font-bold">+ {t('wh.add')}</h2>
			<div>
				<label class="lbl mb-1.5" for="url">{t('wh.url')}</label>
				<input id="url" class="inp font-mono" name="url" type="url" inputmode="url" placeholder="https://toko.example/erapay/webhook" required />
			</div>
			<button type="submit" class="btn btn-red">{t('wh.add')}</button>
		</form>

		<div class="overflow-hidden rounded-xl border border-white/10 bg-[#0b1020] font-mono text-[11px] text-slate-300 shadow-sm">
			<div class="flex items-center justify-between border-b border-white/10 px-4 py-2 text-slate-500"><span>verifikasi-tanda-tangan</span>
				<button type="button" class="rounded px-2 py-0.5 font-bold text-slate-300 hover:bg-white/10" onclick={() => copy('sig', sig)}>{copied === 'sig' ? t('keys.copied') : t('keys.copy')}</button></div>
			<pre class="overflow-x-auto p-4 leading-relaxed"><code>{sig}</code></pre>
			<p class="border-t border-white/10 px-4 py-2 text-slate-500">{t('ig.sigNote')}</p>
		</div>
	</div>

	<div class="card mb-5 overflow-hidden">
		<div class="p-5"><h2 class="font-mono text-sm font-bold">{t('wh.endpoints')}</h2></div>
		{#if data.endpoints === null}
			<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
		{:else if data.endpoints.length === 0}
			<p class="muted px-5 pb-5 font-mono text-xs">{t('wh.none')}</p>
		{:else}
			<div class="overflow-x-auto">
				<table>
					<thead><tr><th>URL</th><th>{t('wh.hint')}</th><th>{t('c.status')}</th><th>{t('c.created')}</th><th></th></tr></thead>
					<tbody>
						{#each data.endpoints as e (e.id)}
							<tr>
								<td class="font-mono font-bold">{e.url}</td>
								<td><code>…{e.secret_hint}</code></td>
								<td><StateBadge kind="wh" value={e.status} /></td>
								<td class="muted">{fmt(e.created_at)}</td>
								<td>
									{#if e.status === 'active'}
										<div class="flex gap-2">
											<form method="POST" action="?/hookRotate" use:enhance={withConfirm({ variant: 'warning', title: 'Rotasi secret', successMessage: 'Secret berhasil dirotasi.', confirmText: 'Ya, rotasi', message: t('wh.rotateConfirm') })}>
												<input type="hidden" name="id" value={e.id} />
												<button type="submit" class="btn btn-light">{t('wh.rotate')}</button>
											</form>
											<form method="POST" action="?/hookDisable" use:enhance={withConfirm({ variant: 'danger', title: 'Nonaktifkan endpoint', successMessage: 'Endpoint dinonaktifkan.', confirmText: 'Ya, nonaktifkan', message: t('wh.disableConfirm') })}>
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
			<h2 class="font-mono text-sm font-bold">{t('wh.deliveries')}</h2>
			<nav class="flex flex-wrap gap-1.5" aria-label={t('fin.filter')}>
				{#each filters as s (s)}
					<a href={s ? `?tab=webhooks&status=${s}` : '?tab=webhooks'} aria-current={data.status === s ? 'true' : undefined} class="rounded-full border border-ink/10 px-3 py-1 text-[11px] font-bold hover:bg-ink/5 aria-[current=true]:bg-merah aria-[current=true]:text-white">{s ? t(`wh.d.${s}` as const) : t('wh.all')}</a>
				{/each}
			</nav>
		</div>
		{#if data.deliveries === null}
			<p class="px-5 pb-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
		{:else if data.deliveries.length === 0}
			<p class="muted px-5 pb-5 font-mono text-xs">{t('wh.dnone')}</p>
		{:else}
			<div class="overflow-x-auto">
				<table>
					<thead><tr><th>{t('c.created')}</th><th>{t('wh.event')}</th><th>URL</th><th>{t('c.status')}</th><th>{t('wh.attempts')}</th><th>{t('wh.http')}</th><th>{t('wh.error')}</th><th></th></tr></thead>
					<tbody>
						{#each data.deliveries as d (d.id)}
							<tr>
								<td class="muted">{fmt(d.created_at)}</td>
								<td class="font-mono font-bold">{d.event_type}{#if d.redelivery_of}<span class="muted ml-1 font-normal">↻</span>{/if}</td>
								<td class="max-w-56 truncate font-mono" title={urlOf(d.endpoint_id)}>{urlOf(d.endpoint_id)}</td>
								<td>
									<StateBadge kind="wd" value={d.status} />
									{#if d.next_attempt_at}<div class="muted mt-1 text-[10px]">{t('wh.next')}: {fmt(d.next_attempt_at)}</div>{/if}
								</td>
								<td>{d.attempts}</td>
								<td><code>{d.last_http_status ?? '—'}</code></td>
								<td class="muted">{d.last_error ?? '—'}</td>
								<td>
									{#if d.status !== 'pending'}
										<form method="POST" action="?/hookRedeliver" use:enhance>
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
{/if}

<style>
	.hero { animation: hero-in 0.5s cubic-bezier(0.2, 0.8, 0.2, 1) both; }
	.cursor { color: #34d399; animation: blink 1s steps(2, start) infinite; }
	.tab { padding: 0.6rem 1rem; border-bottom: 2px solid transparent; margin-bottom: -1px; opacity: 0.6; }
	.tab:hover { opacity: 1; }
	.tab.on { opacity: 1; border-bottom-color: var(--color-merah); color: var(--color-merah); }
	@keyframes hero-in { from { opacity: 0; transform: translateY(-8px); } to { opacity: 1; transform: none; } }
	@keyframes blink { to { visibility: hidden; } }
	@media (prefers-reduced-motion: reduce) {
		.hero, .cursor { animation: none; }
	}
</style>
