<script lang="ts">
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	let { data } = $props();
	const { t } = getI18n();
</script>

<svelte:head><title>{t('pf.ov.pageTitle')}</title></svelte:head>

<div class="space-y-6">
	<div class="card p-6">
		<h1 class="text-xl font-bold">{t('pf.ov.welcome', { name: data.user.email.split('@')[0] })}</h1>
		<p class="muted mt-1 text-sm">{t('pf.ov.intro')}</p>
	</div>

	<div class="grid gap-5 md:grid-cols-2">
		<a href="/platform/access" class="card group p-5 transition hover:-translate-y-0.5 hover:shadow-lg">
			<div class="flex items-center gap-3">
				<span class="grid size-10 place-items-center rounded-xl bg-merah text-white">
					<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
						<rect x="4" y="11" width="16" height="9" rx="2" /><path d="M8 11V8a4 4 0 0 1 8 0v3" />
					</svg>
				</span>
				<div>
					<div class="text-sm font-bold group-hover:text-merah">{t('pf.nav.access')}</div>
					<div class="muted text-xs">{t('pf.ov.access.desc')}</div>
				</div>
			</div>
		</a>

		<div class="card p-5">
			<div class="flex items-center gap-3">
				<span class="grid size-10 place-items-center rounded-xl bg-emas text-black">
					<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
						<path d="M5 7h14l-2 12H7L5 7Z" /><path d="M8 7V6a4 4 0 0 1 8 0v1" />
					</svg>
				</span>
				<div>
					<div class="text-sm font-bold">{t('pf.ov.mode')}</div>
					<div class="muted text-xs">{t('pf.ov.mode.desc')}</div>
				</div>
			</div>
		</div>

		<div class="card p-5 md:col-span-2">
			<div class="muted text-xs leading-relaxed">{t('pf.ov.note')}</div>
		</div>
	</div>
</div>
