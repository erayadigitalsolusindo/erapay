<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';

	let { data } = $props();
	const i18n = getI18n();
	const { t, tx } = i18n;

	const s = $derived(data.stats);
	let busy = $state(false);

	// Segarkan otomatis tiap 30 dtk selama tab terlihat; berhenti saat komponen dilepas.
	$effect(() => {
		const id = setInterval(() => {
			if (document.visibilityState === 'visible') void invalidateAll();
		}, 30000);
		return () => clearInterval(id);
	});
	async function refresh() {
		busy = true;
		await invalidateAll();
		busy = false;
	}

	const nf = $derived(new Intl.NumberFormat(i18n.locale));
	const n = (v: number) => nf.format(v);
	const rp = (v: number) => `Rp${new Intl.NumberFormat('id-ID').format(v)}`;
	const pct = (a: number, b: number) => (b > 0 ? Math.round((a / b) * 100) : 0);
	const clock = (iso: string) => new Date(iso).toLocaleTimeString(i18n.locale, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
	const hh = (iso: string) => new Date(iso).toLocaleTimeString(i18n.locale, { hour: '2-digit', minute: '2-digit' });

	const series = $derived(s?.audit.series_24h ?? []);
	const peak = $derived(Math.max(1, ...series.map((p) => p.count)));
	const recent = $derived(s?.audit.recent ?? []);

	const MS_ORDER = ['draft', 'submitted', 'under_review', 'action_required', 'approved', 'rejected', 'suspended'];
	const MS_COLOR: Record<string, string> = {
		draft: 'bg-neutral-400',
		submitted: 'bg-sky-500',
		under_review: 'bg-amber-500',
		action_required: 'bg-orange-500',
		approved: 'bg-emerald-500',
		rejected: 'bg-red-500',
		suspended: 'bg-merah-gelap'
	};
	const merchants = $derived(MS_ORDER.map((k) => [k, s?.merchants_by_status?.[k] ?? 0] as const));
	const merchantTotal = $derived(merchants.reduce((a, [, v]) => a + v, 0));

	const mfaPct = $derived(s ? pct(s.users.mfa_enabled, s.users.total) : 0);
	const payRate = $derived(s ? pct(s.payments.succeeded_24h, s.payments.total_24h) : 0);

	// Postur keamanan: hanya aturan yang bisa dihitung dari angka agregat.
	const checks = $derived(
		s
			? [
					{ label: t('pf2.chk.mfa'), ok: s.users.total > 0 && s.users.mfa_enabled === s.users.total, detail: `${s.users.mfa_enabled}/${s.users.total}` },
					{ label: t('pf2.chk.failed'), ok: s.sessions.failed_logins_1h < 20, detail: String(s.sessions.failed_logins_1h) },
					{ label: t('pf2.chk.imp'), ok: s.sessions.impersonation === 0, detail: String(s.sessions.impersonation) },
					{ label: t('pf2.chk.wh'), ok: s.webhooks.dead_24h === 0, detail: String(s.webhooks.dead_24h) },
					{ label: t('pf2.chk.pending'), ok: s.payments.pending < 50, detail: String(s.payments.pending) }
				]
			: []
	);
	const allOk = $derived(checks.every((c) => c.ok));

	const actionTone = (a: string) =>
		a.startsWith('auth.') ? 'text-sky-400' : a.startsWith('impersonation.') ? 'text-amber-400' : a.startsWith('platform.') || a.startsWith('rbac.') ? 'text-rose-400' : 'text-emerald-400';

	const quick = [
		{ href: '/platform/access', k: 'pf2.quick.access', d: 'pf2.quick.accessDesc', icon: 'M4 11h16v9H4zM8 11V8a4 4 0 0 1 8 0v3' },
		{ href: '/platform/tenants', k: 'pf2.quick.tenants', d: 'pf2.quick.tenantsDesc', icon: 'M3 21h18M5 21V7l7-4 7 4v14M9 9h1M14 9h1M9 13h1M14 13h1' },
		{ href: '/platform/audit', k: 'pf2.quick.audit', d: 'pf2.quick.auditDesc', icon: 'M7 3h8l4 4v14H7zM15 3v4h4M10 12h6M10 16h6' }
	] as const;
</script>

<svelte:head><title>{t('pf.ov.pageTitle')}</title></svelte:head>

<div class="space-y-5">
	<!-- Header konsol -->
	<header class="card relative overflow-hidden p-5">
		<div class="pointer-events-none absolute inset-y-0 right-0 w-1/3 bg-gradient-to-l from-merah/5 to-transparent"></div>
		<div class="flex flex-wrap items-start justify-between gap-4">
			<div>
				<div class="font-mono text-[10px] tracking-[.18em] text-merah uppercase">dewa@erapay:~$ platform status</div>
				<h1 class="font-display mt-1 text-xl font-extrabold">{t('pf2.dash.title')}</h1>
				<p class="muted mt-1 max-w-2xl text-xs leading-relaxed">{t('pf2.dash.sub')}</p>
			</div>
			<div class="flex flex-wrap items-center gap-2 font-mono text-[11px]">
				<span class="flex items-center gap-1.5 rounded-md border border-ink/10 px-2.5 py-1.5">
					<span class="size-2 rounded-full {data.apiUp ? 'animate-pulse bg-emerald-500' : 'bg-red-500'}"></span>
					{t('pf2.dash.api')} <b>{data.apiUp ? t('pf2.dash.up') : t('pf2.dash.down')}</b>
				</span>
				<span class="flex items-center gap-1.5 rounded-md border border-ink/10 px-2.5 py-1.5">
					<span class="size-2 rounded-full {s ? 'bg-emerald-500' : 'bg-red-500'}"></span>
					{t('pf2.dash.db')} <b>{s ? `${s.db_latency_ms} ms` : t('pf2.dash.down')}</b>
				</span>
				{#if s}<span class="text-ink/60">{t('pf2.dash.updated', { time: clock(s.generated_at) })}</span>{/if}
				<button type="button" class="btn btn-light py-1.5" onclick={refresh} disabled={busy}>
					<svg viewBox="0 0 24 24" class="size-3.5 {busy ? 'animate-spin' : ''}" fill="none" stroke="currentColor" stroke-width="2.2" aria-hidden="true"><path d="M21 12a9 9 0 1 1-3-6.7M21 3v6h-6" /></svg>
					{t('pf2.dash.refresh')}
				</button>
			</div>
		</div>
	</header>

	{#if !s}
		<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{t('pf2.dash.loadFail')}</p>
	{:else}
		<!-- KPI -->
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
			{#snippet kpi(label: string, value: string, sub: string, tone: 'ok' | 'warn' | 'bad' | 'plain' = 'plain', bar?: number)}
				<div class="card relative overflow-hidden p-4">
					<span class="absolute inset-y-0 left-0 w-1 {tone === 'ok' ? 'bg-emerald-500' : tone === 'warn' ? 'bg-amber-500' : tone === 'bad' ? 'bg-red-500' : 'bg-ink/20'}"></span>
					<div class="font-mono text-[10px] tracking-[.14em] text-ink/60 uppercase">{label}</div>
					<div class="mt-1.5 font-mono text-2xl font-bold tracking-tight tabular-nums">{value}</div>
					<div class="muted mt-0.5 text-[11px]">{sub}</div>
					{#if bar !== undefined}
						<div class="mt-2 h-1 overflow-hidden rounded-full bg-ink/10"><div class="h-full rounded-full {tone === 'bad' ? 'bg-red-500' : tone === 'warn' ? 'bg-amber-500' : 'bg-emerald-500'}" style:width={`${bar}%`}></div></div>
					{/if}
				</div>
			{/snippet}
			{@render kpi(t('pf2.kpi.users'), n(s.users.total), t('pf2.kpi.usersSub', { a: s.users.active, d: s.users.disabled }))}
			{@render kpi(t('pf2.kpi.mfa'), `${mfaPct}%`, t('pf2.kpi.mfaSub', { n: s.users.mfa_enabled, t: s.users.total }), mfaPct === 100 ? 'ok' : mfaPct >= 50 ? 'warn' : 'bad', mfaPct)}
			{@render kpi(t('pf2.kpi.sessions'), n(s.sessions.active), t('pf2.kpi.sessionsSub', { p: s.sessions.platform, i: s.sessions.impersonation }), s.sessions.impersonation > 0 ? 'warn' : 'plain')}
			{@render kpi(t('pf2.kpi.failed'), n(s.sessions.failed_logins_1h), t('pf2.kpi.failedSub', { n: s.sessions.failed_logins_24h }), s.sessions.failed_logins_1h >= 20 ? 'bad' : s.sessions.failed_logins_1h >= 5 ? 'warn' : 'ok')}
			{@render kpi(t('pf2.kpi.pay'), n(s.payments.total_24h), t('pf2.kpi.paySub', { ok: s.payments.succeeded_24h, rate: payRate }), s.payments.total_24h === 0 ? 'plain' : payRate >= 90 ? 'ok' : payRate >= 70 ? 'warn' : 'bad', s.payments.total_24h ? payRate : undefined)}
			{@render kpi(t('pf2.kpi.vol'), rp(s.payments.volume_24h), t('pf2.kpi.volSub'))}
			{@render kpi(t('pf2.kpi.pending'), n(s.payments.pending), t('pf2.kpi.pendingSub'), s.payments.pending >= 50 ? 'warn' : 'plain')}
			{@render kpi(t('pf2.kpi.wh'), n(s.webhooks.pending), t('pf2.kpi.whSub', { n: s.webhooks.dead_24h }), s.webhooks.dead_24h > 0 ? 'bad' : 'ok')}
		</div>

		<div class="grid gap-5 lg:grid-cols-3">
			<!-- Aktivitas audit -->
			<section class="card p-5 lg:col-span-2">
				<div class="flex items-baseline justify-between gap-2">
					<h2 class="text-sm font-bold">{t('pf2.sec.activity')}</h2>
					<span class="font-mono text-[11px] text-ink/60">{t('pf2.sec.activitySub', { n: n(s.audit.events_24h) })}</span>
				</div>
				{#if series.length}
					<svg viewBox="0 0 480 140" class="mt-4 h-40 w-full" role="img" aria-label={t('pf2.sec.activity')} preserveAspectRatio="none">
						{#each [0, 1, 2, 3] as g (g)}
							<line x1="0" x2="480" y1={10 + g * 35} y2={10 + g * 35} stroke="currentColor" stroke-opacity="0.08" stroke-dasharray="3 4" />
						{/each}
						{#each series as p, i (p.hour)}
							{@const h = p.count === 0 ? 1.5 : Math.max(3, (p.count / peak) * 110)}
							<rect x={i * 20 + 3} y={120 - h} width="14" height={h} rx="2" class={p.count === peak && peak > 1 ? 'fill-merah' : 'fill-merah/55'}>
								<title>{hh(p.hour)} · {p.count}</title>
							</rect>
						{/each}
					</svg>
					<div class="mt-1 flex justify-between font-mono text-[10px] text-ink/50">
						<span>{hh(series[0].hour)}</span><span>{hh(series[Math.floor(series.length / 2)].hour)}</span><span>{hh(series[series.length - 1].hour)}</span>
					</div>
				{:else}
					<p class="muted mt-6 text-xs">{t('pf2.sec.none')}</p>
				{/if}
			</section>

			<!-- Postur keamanan -->
			<section class="card p-5">
				<div class="flex items-center justify-between gap-2">
					<h2 class="text-sm font-bold">{t('pf2.sec.posture')}</h2>
					<span class="pill font-mono text-[10px] {allOk ? 'bg-emerald-500/15 text-emerald-700' : 'bg-amber-500/20 text-amber-700'}">{allOk ? t('pf2.chk.ok') : t('pf2.chk.warn')}</span>
				</div>
				<ul class="mt-3 divide-y divide-ink/10">
					{#each checks as c (c.label)}
						<li class="flex items-start gap-3 py-2.5">
							<span class="mt-0.5 grid size-4 shrink-0 place-items-center rounded-full text-[10px] font-bold text-white {c.ok ? 'bg-emerald-500' : 'bg-amber-500'}" aria-hidden="true">{c.ok ? '✓' : '!'}</span>
							<span class="min-w-0 flex-1 text-xs leading-snug">{c.label}</span>
							<span class="font-mono text-[11px] text-ink/60">{c.detail}</span>
							<span class="sr-only">{c.ok ? t('pf2.chk.ok') : t('pf2.chk.warn')}</span>
						</li>
					{/each}
				</ul>
			</section>
		</div>

		<div class="grid gap-5 lg:grid-cols-3">
			<!-- Tenant per status -->
			<section class="card p-5">
				<div class="flex items-baseline justify-between gap-2">
					<h2 class="text-sm font-bold">{t('pf2.sec.merchants')}</h2>
					<span class="font-mono text-[11px] text-ink/60">{t('pf2.kpi.tenantsSub', { n: s.merchants_by_status?.approved ?? 0 })}</span>
				</div>
				<div class="mt-4 flex h-3 overflow-hidden rounded-full bg-ink/10" role="img" aria-label={t('pf2.sec.merchants')}>
					{#each merchants as [k, v] (k)}
						{#if v > 0}<div class="{MS_COLOR[k]} h-full" style:width={`${(v / merchantTotal) * 100}%`} title={`${tx(`pf2.ms.${k}`, k)}: ${v}`}></div>{/if}
					{/each}
				</div>
				<ul class="mt-4 space-y-2">
					{#each merchants as [k, v] (k)}
						<li class="flex items-center gap-2.5 text-xs">
							<span class="size-2.5 rounded-sm {MS_COLOR[k]}"></span>
							<span class="flex-1">{tx(`pf2.ms.${k}`, k)}</span>
							<span class="font-mono font-bold tabular-nums">{n(v)}</span>
							<span class="w-9 text-right font-mono text-[10px] text-ink/50">{pct(v, merchantTotal)}%</span>
						</li>
					{/each}
				</ul>
			</section>

			<!-- Log terminal -->
			<section class="overflow-hidden rounded-2xl border border-ink/10 bg-[#0d0709] shadow-sm lg:col-span-2">
				<div class="flex items-center justify-between border-b border-white/10 px-4 py-2.5">
					<div class="flex items-center gap-2">
						<span class="size-2.5 rounded-full bg-red-500/80"></span><span class="size-2.5 rounded-full bg-amber-400/80"></span><span class="size-2.5 rounded-full bg-emerald-500/80"></span>
						<h2 class="ml-2 font-mono text-[11px] font-bold text-white/80">{t('pf2.sec.recent')}</h2>
					</div>
					<a href="/platform/audit" class="font-mono text-[11px] text-emas hover:underline">{t('pf2.sec.recentAll')}</a>
				</div>
				<div class="overflow-x-auto p-4 font-mono text-[11px] leading-relaxed">
					{#each recent as r (r.at + r.action)}
						<div class="flex gap-3 whitespace-nowrap">
							<span class="text-white/40">{clock(r.at)}</span>
							<span class="{actionTone(r.action)} font-bold">{r.action}</span>
							<span class="text-white/50">{r.entity_type}</span>
						</div>
					{:else}
						<div class="text-white/50">{t('pf2.sec.none')}</div>
					{/each}
					<div class="mt-1 text-emerald-400">$ <span class="animate-pulse">▍</span></div>
				</div>
			</section>
		</div>

		<!-- Pintasan -->
		<section>
			<h2 class="font-mono text-[10px] tracking-[.18em] text-ink/60 uppercase">{t('pf2.sec.quick')}</h2>
			<div class="mt-2 grid gap-3 md:grid-cols-3">
				{#each quick as q (q.href)}
					<a href={q.href} class="card group flex items-center gap-3 p-4 transition hover:-translate-y-0.5 hover:border-merah/40 hover:shadow-md">
						<span class="grid size-10 place-items-center rounded-xl bg-merah/10 text-merah transition group-hover:bg-merah group-hover:text-white">
							<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d={q.icon} /></svg>
						</span>
						<span class="min-w-0">
							<span class="block text-sm font-bold group-hover:text-merah">{t(q.k)}</span>
							<span class="muted block text-xs">{t(q.d)}</span>
						</span>
					</a>
				{/each}
			</div>
		</section>
	{/if}
</div>
