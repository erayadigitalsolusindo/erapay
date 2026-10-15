<script lang="ts">
	import '../../app.css';
	import { createI18n, setI18n } from '../../lib/i18n/i18n.svelte';
	import Header from './_components/Header.svelte';
	import Sidebar from './_components/Sidebar.svelte';
	import TabBar from './_components/TabBar.svelte';

	let { data, children } = $props();

	// Nilai awal dari cookie (server); perubahan berikutnya ditulis balik ke cookie oleh Header.
	// svelte-ignore state_referenced_locally
	const i18n = createI18n(data.lang);
	setI18n(i18n);
	// svelte-ignore state_referenced_locally
	let theme = $state<'light' | 'dark' | 'system'>(data.theme);

	$effect(() => {
		document.documentElement.lang = i18n.lang;
	});
</script>

<svelte:head>
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
	<link
		href="https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,600;9..144,800&family=Plus+Jakarta+Sans:wght@400;500;600;700;800&display=swap"
		rel="stylesheet"
	/>
</svelte:head>

<div
	data-theme={theme}
	class="adm font-sans min-h-screen bg-surface text-ink antialiased lg:grid lg:grid-cols-[248px_minmax(0,1fr)]"
>
	<Sidebar user={data.user} menus={data.menus} />
	<div class="min-w-0">
		<Header user={data.user} bind:theme />
		<TabBar user={data.user} menus={data.menus} />
		<main class="border-t border-ink/10 bg-page p-5 lg:p-8">
			{#if data.user.merchantId && !data.user.emailVerified}
				<p class="mb-3 rounded-xl border border-amber-300 bg-amber-50 px-4 py-3 text-xs font-semibold text-amber-900" role="status">
					{i18n.t('onb.banner.unverified')}
					<a href="/dashboard/onboarding" class="ml-1 underline">{i18n.t('onb.banner.link')}</a>
				</p>
			{/if}
			{#if data.onboarding}
				<p class="mb-5 rounded-xl border border-amber-300 bg-amber-50 px-4 py-3 text-xs font-semibold text-amber-900" role="status">
					{#if data.onboarding.status === 'rejected'}
						{i18n.t('onb.banner.rejected', { note: data.onboarding.note ?? '—' })}
					{:else if data.onboarding.status === 'action_required'}
						{i18n.t('onb.banner.action', { note: data.onboarding.note ?? '—' })}
						<a href="/dashboard/onboarding" class="ml-1 underline">{i18n.t('onb.banner.link')}</a>
					{:else if ['submitted', 'under_review'].includes(data.onboarding.status)}
						{i18n.t('onb.banner.pending')}
					{:else}
						{i18n.t('onb.banner.other', { status: data.onboarding.status })}
					{/if}
				</p>
			{/if}
			{@render children()}
			<footer class="muted mt-8 text-[10px]">{i18n.t('footer.tagline')}</footer>
		</main>
	</div>
</div>
