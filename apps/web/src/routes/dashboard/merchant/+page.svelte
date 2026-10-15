<script lang="ts">
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import StateBadge from '../../../lib/components/StateBadge.svelte';

	let { data } = $props();
	const { t } = getI18n();

	const initials = (n: string) =>
		n
			.split(/\s+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((w) => w[0]!.toUpperCase())
			.join('') || '?';
</script>

{#if data.profile === null}
	<p class="card p-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
{:else}
	<!-- hero -->
	<section class="hero relative mb-5 overflow-hidden rounded-3xl bg-linear-to-br from-merah-gelap via-merah-tua to-merah p-6 text-white shadow-lg sm:p-8">
		<div class="pointer-events-none absolute -right-16 -top-20 size-72 rounded-full bg-emas/25 blur-3xl"></div>
		<div class="pointer-events-none absolute -bottom-24 left-1/3 size-64 rounded-full bg-white/10 blur-3xl"></div>
		<svg class="pointer-events-none absolute inset-0 size-full opacity-[0.07]" aria-hidden="true"><defs><pattern id="mp-dots" width="22" height="22" patternUnits="userSpaceOnUse"><circle cx="2" cy="2" r="1.6" fill="#fff" /></pattern></defs><rect width="100%" height="100%" fill="url(#mp-dots)" /></svg>
		<div class="relative flex flex-wrap items-center gap-5">
			<span class="grid size-20 shrink-0 place-items-center rounded-3xl bg-white/15 font-display text-3xl font-extrabold shadow-inner backdrop-blur">{initials(data.profile.display_name)}</span>
			<div class="min-w-0 flex-1">
				<span class="inline-flex items-center gap-2 rounded-full bg-white/15 px-3 py-1 text-[11px] font-bold tracking-wide backdrop-blur">
					<span class="size-1.5 animate-denyut rounded-full bg-emas"></span> {t('nav.myMerchant')}
				</span>
				<h1 class="mt-3 truncate font-display text-3xl font-extrabold leading-tight sm:text-4xl">{data.profile.display_name}</h1>
				<p class="mt-1 truncate text-[13px] text-white/75">{data.profile.legal_name}</p>
			</div>
			<div class="rounded-2xl bg-white px-4 py-3 text-ink shadow-md">
				<div class="muted text-[10px] font-bold uppercase tracking-wider">{t('c.status')}</div>
				<div class="mt-1"><StateBadge kind="onb" value={data.profile.onboarding_status} /></div>
			</div>
		</div>
	</section>

	{#if data.profile.review_note}
		<p class="mb-5 rounded-2xl border-l-4 border-emas bg-emas/10 px-4 py-3 text-xs leading-relaxed">
			<b>{t('mp.reviewNote')}</b> {data.profile.review_note}
		</p>
	{/if}

	<!-- ringkasan -->
	<div class="mb-5 grid gap-3 sm:grid-cols-3">
		<div class="tile card animate-naik p-4">
			<span class="grid size-9 place-items-center rounded-xl bg-merah/10 text-merah">
				<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 9l1.5-5h15L21 9M4 9v11h16V9M3 9h18M9 20v-6h6v6" /></svg>
			</span>
			<div class="mt-3 font-display text-3xl font-extrabold leading-none">{data.outlets?.length ?? '—'}</div>
			<div class="muted mt-1 text-[11px] font-bold">{t('mm.outlets')}</div>
		</div>
		<div class="tile card animate-naik p-4" style="animation-delay:60ms">
			<span class="grid size-9 place-items-center rounded-xl bg-sky-500/10 text-sky-600">
				<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 4h16v16H4z M8 9h8 M8 13h8 M8 17h5" /></svg>
			</span>
			<div class="mt-3 truncate text-sm font-extrabold leading-tight">{data.profile.legal_name}</div>
			<div class="muted mt-1 text-[11px] font-bold">{t('mm.legalName')}</div>
		</div>
		<div class="tile card animate-naik p-4" style="animation-delay:120ms">
			<span class="grid size-9 place-items-center rounded-xl bg-emerald-500/10 text-emerald-600">
				<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3 4 6v6c0 4.5 3.2 8 8 9 4.8-1 8-4.5 8-9V6l-8-3Z M9 12l2.2 2.2L15.5 10" /></svg>
			</span>
			<div class="mt-3 truncate text-sm font-extrabold leading-tight">{t(`onb.st.${data.profile.onboarding_status}` as never)}</div>
			<div class="muted mt-1 text-[11px] font-bold">{t('mp.verifStatus')}</div>
		</div>
	</div>
{/if}

<!-- outlet -->
<section>
	<h2 class="mb-3 flex items-center gap-2 text-sm font-extrabold">
		<span class="h-4 w-1 rounded-full bg-merah"></span>{t('mm.outlets')}
	</h2>
	{#if data.outlets === null}
		<p class="card p-5 text-xs font-semibold text-red-700" role="alert">{t('c.loadFail')}</p>
	{:else if data.outlets.length === 0}
		<div class="card grid place-items-center gap-3 border-dashed px-6 py-12 text-center">
			<span class="empty-float grid size-16 place-items-center rounded-full bg-merah/10 text-merah">
				<svg viewBox="0 0 24 24" class="size-8" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 9l1.5-5h15L21 9M4 9v11h16V9M3 9h18M9 20v-6h6v6" /></svg>
			</span>
			<p class="text-sm font-bold">{t('mm.noOutlets')}</p>
		</div>
	{:else}
		<ul class="grid list-none gap-3 sm:grid-cols-2 xl:grid-cols-3">
			{#each data.outlets as o, i (o.id)}
				<li style="animation-delay:{Math.min(i, 8) * 50}ms" class="card flex animate-naik items-center gap-3 p-4 transition hover:-translate-y-0.5 hover:shadow-md">
					<span class="grid size-11 shrink-0 place-items-center rounded-2xl bg-linear-to-br from-merah to-merah-tua font-display text-base font-extrabold text-white shadow-sm">{initials(o.name)}</span>
					<span class="min-w-0">
						<span class="block truncate text-sm font-extrabold">{o.name}</span>
						<code class="muted text-[11px]">{o.id.slice(0, 8)}</code>
					</span>
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	.hero { animation: hero-in 0.6s cubic-bezier(0.2, 0.8, 0.2, 1) both; }
	.empty-float { animation: float 3s ease-in-out infinite; }
	@keyframes hero-in { from { opacity: 0; transform: translateY(-10px) scale(0.99); } to { opacity: 1; transform: none; } }
	@keyframes float { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-6px); } }
	@media (prefers-reduced-motion: reduce) {
		.hero, .empty-float, .tile { animation: none; }
	}
</style>
