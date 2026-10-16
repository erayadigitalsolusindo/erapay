<script lang="ts">
	import Combobox from '../../../../lib/components/Combobox.svelte';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	let { data } = $props();
	const i18n = getI18n();
	const { t, tx } = i18n;

	type Row = (typeof data.rows)[number];
	let grep = $state('');
	let wrap = $state(false);

	const p2 = (n: number) => String(n).padStart(2, '0');
	// Stempel waktu ala log server: 2026-10-16 13:04:59 (zona waktu lokal peramban).
	const ts = (s: string) => {
		const d = new Date(s);
		return `${d.getFullYear()}-${p2(d.getMonth() + 1)}-${p2(d.getDate())} ${p2(d.getHours())}:${p2(d.getMinutes())}:${p2(d.getSeconds())}`;
	};

	type Level = 'ERROR' | 'WARN' | 'INFO';
	const level = (a: string): Level => {
		if (/fail|denied|forbidden|locked|invalid|reject|blocked/i.test(a)) return 'ERROR';
		if (/impersonation|revoke|delete|remove|password|suspend|disable|reset|owner/i.test(a)) return 'WARN';
		return 'INFO';
	};
	const LV: Record<Level, string> = {
		ERROR: 'bg-red-500/20 text-red-300',
		WARN: 'bg-amber-400/20 text-amber-300',
		INFO: 'bg-sky-400/15 text-sky-300'
	};
	const fields = (md: Record<string, unknown> | null) =>
		Object.entries(md ?? {})
			.filter(([k]) => k !== 'session_id')
			.map(([k, v]) => [k, typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v)] as const);

	const line = (r: Row) =>
		[ts(r.created_at), level(r.action), r.actor_email ?? '', r.action, r.entity_id ?? '', r.request_id ?? '', ...fields(r.metadata).map(([k, v]) => `${k}=${v}`)]
			.join(' ')
			.toLowerCase();
	const rows = $derived(grep.trim() ? data.rows.filter((r) => line(r).includes(grep.trim().toLowerCase())) : data.rows);
	const tally = $derived(
		rows.reduce<Record<Level, number>>((a, r) => ((a[level(r.action)] += 1), a), { ERROR: 0, WARN: 0, INFO: 0 })
	);

	function exportLog() {
		const text = rows
			.map((r) => `${ts(r.created_at)} ${level(r.action).padEnd(5)} actor=${r.actor_email ?? '-'} action=${r.action} tenant=${r.entity_type === 'merchant' ? r.entity_id : '-'} ${fields(r.metadata).map(([k, v]) => `${k}=${v}`).join(' ')}${r.request_id ? ` req=${r.request_id}` : ''}`.trimEnd())
			.join('\n');
		const a = document.createElement('a');
		a.href = URL.createObjectURL(new Blob([text + '\n'], { type: 'text/plain' }));
		a.download = `erapay-audit-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.log`;
		a.click();
		URL.revokeObjectURL(a.href);
	}
</script>

<svelte:head><title>{t('pf.au.pageTitle')}</title></svelte:head>

<div class="space-y-4">
	<div class="flex flex-wrap items-end justify-between gap-3">
		<div>
			<h1 class="font-display text-2xl font-extrabold">{t('pf.nav.audit')}</h1>
			<p class="muted mt-1 max-w-2xl text-[13px]">{t('pf.au.sub')}</p>
		</div>
		<form method="GET" class="flex flex-wrap items-center gap-2">
			<Combobox id="audit-prefix" name="prefix" class="w-48" value={data.prefix} options={[
				{ value: 'impersonation.', label: t('pf.au.f.imp') },
				{ value: '', label: t('pf.au.f.all') },
				{ value: 'auth.', label: t('pf.au.f.auth') }
			]} />
			<input name="merchant_id" value={data.merchant} placeholder={t('pf.au.merchantPh')} class="inp w-56 py-2" />
			<button class="rounded-full bg-merah px-4 py-2 text-xs font-bold text-white">{t('pf.au.filter')}</button>
		</form>
	</div>

	{#if data.error}<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{data.error}</p>{/if}

	<!-- jendela terminal (selalu gelap, apa pun tema) -->
	<section class="overflow-hidden rounded-2xl border border-white/10 bg-[#0b0f14] font-mono text-[#c9d1d9] shadow-xl">
		<div class="flex flex-wrap items-center gap-3 border-b border-white/10 bg-[#11171e] px-4 py-2.5">
			<span class="flex gap-1.5" aria-hidden="true">
				<span class="size-3 rounded-full bg-[#ff5f56]"></span><span class="size-3 rounded-full bg-[#ffbd2e]"></span><span class="size-3 rounded-full bg-[#27c93f]"></span>
			</span>
			<span class="truncate text-[11px] text-white/50">erapay@platform:~$ tail -n {data.rows.length} audit.log{data.prefix ? ` | grep '^${data.prefix}'` : ''}{data.merchant ? ` | grep ${data.merchant}` : ''}</span>
			<span class="ml-auto flex items-center gap-2 text-[10px]">
				<span class="rounded bg-red-500/20 px-1.5 py-0.5 text-red-300">ERR {tally.ERROR}</span>
				<span class="rounded bg-amber-400/20 px-1.5 py-0.5 text-amber-300">WRN {tally.WARN}</span>
				<span class="rounded bg-sky-400/15 px-1.5 py-0.5 text-sky-300">INF {tally.INFO}</span>
			</span>
		</div>

		<div class="flex flex-wrap items-center gap-2 border-b border-white/10 px-4 py-2 text-xs">
			<span class="text-emerald-400">$ grep -i</span>
			<input
				bind:value={grep}
				placeholder={tx('pf.au.grepPh', 'filter baris… (aktor, aksi, req id)')}
				class="min-w-48 flex-1 bg-transparent text-[#e6edf3] outline-none placeholder:text-white/25"
				spellcheck="false"
				autocomplete="off"
			/>
			<button type="button" onclick={() => (wrap = !wrap)} aria-pressed={wrap} class="rounded border border-white/15 px-2 py-0.5 text-[10px] text-white/60 transition hover:bg-white/10 aria-pressed:bg-white/15 aria-pressed:text-white">wrap</button>
			<button type="button" onclick={exportLog} disabled={!rows.length} class="rounded border border-white/15 px-2 py-0.5 text-[10px] text-white/60 transition hover:bg-white/10 disabled:opacity-30">.log ↓</button>
		</div>

		<div class="max-h-[70vh] overflow-auto py-1 text-[11.5px] leading-relaxed">
			{#each rows as r (r.id)}
				{@const lv = level(r.action)}
				<details class="group border-l-2 {lv === 'ERROR' ? 'border-red-500' : lv === 'WARN' ? 'border-amber-400/70' : 'border-transparent'} hover:bg-white/[.04] open:bg-white/[.05]">
					<summary class="flex cursor-pointer list-none items-start gap-x-3 px-3 py-1 {wrap ? 'flex-wrap' : 'whitespace-nowrap'}">
						<span class="select-none text-white/30 transition group-open:rotate-90">▸</span>
						<time class="shrink-0 text-white/45" datetime={r.created_at}>{ts(r.created_at)}</time>
						<span class="w-12 shrink-0 rounded px-1 text-center text-[10px] font-bold {LV[lv]}">{lv}</span>
						<span class="shrink-0 text-violet-300">{r.actor_email ?? 'system'}</span>
						<span class="shrink-0 font-bold text-emerald-300">{r.action}</span>
						{#if r.entity_type === 'merchant' && r.entity_id}<span class="shrink-0 text-cyan-300">tenant={r.entity_id}</span>{/if}
						<span class="{wrap ? 'break-all' : 'truncate'} min-w-0 text-white/60">
							{#each fields(r.metadata) as [k, v] (k)}<span class="mr-2"><span class="text-amber-200/80">{k}</span><span class="text-white/30">=</span>{v}</span>{/each}
							{#if r.request_id}<span class="text-white/30">req={r.request_id}</span>{/if}
						</span>
					</summary>
					<pre class="mx-3 mb-2 ml-9 overflow-x-auto rounded bg-black/40 p-3 text-[11px] text-[#9fb3c8]">{JSON.stringify({ id: r.id, time: r.created_at, actor: r.actor_email, action: r.action, entity: `${r.entity_type}:${r.entity_id ?? '-'}`, request_id: r.request_id, metadata: r.metadata }, null, 2)}</pre>
				</details>
			{:else}
				<p class="px-4 py-6 text-white/40">-- {t('pf.au.none')} --</p>
			{/each}
			<p class="px-4 py-2 text-white/25"><span class="text-emerald-400">$</span> <span class="animate-pulse">▍</span></p>
		</div>

		<div class="flex items-center justify-between border-t border-white/10 bg-[#11171e] px-4 py-2 text-[10px] text-white/40">
			<span>{rows.length}{grep.trim() ? ` / ${data.rows.length}` : ''} {tx('pf.au.entries', 'entri')} · append-only</span>
			{#if data.next}
				<a href={`?prefix=${encodeURIComponent(data.prefix)}&merchant_id=${encodeURIComponent(data.merchant)}&before=${data.next}`} class="rounded border border-white/15 px-2.5 py-1 font-bold text-emerald-300 transition hover:bg-white/10">{t('pf.au.older')}</a>
			{/if}
		</div>
	</section>
</div>
