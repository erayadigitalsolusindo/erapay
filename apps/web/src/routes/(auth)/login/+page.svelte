<script lang="ts">
	import { enhance } from '$app/forms';
	import { getContext, untrack } from 'svelte';
	import Field from '../../../lib/components/auth/Field.svelte';
	import Logo from '../../../lib/components/auth/Logo.svelte';

	let { form } = $props();
	const auth = getContext<{ goyang(): void; pindah(href: string): void }>('auth');

	let email = $state(untrack(() => form?.email ?? ''));
	let password = $state('');
	let merchantId = $state('');
	let showPw = $state(false);
	let busy = $state(false);
	let errEmail = $state('');
	let errPw = $state('');

	$effect(() => {
		if (form?.error) auth.goyang();
	});

	function validate() {
		errEmail = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim()) ? '' : 'Masukkan alamat email yang valid.';
		errPw = password ? '' : 'Kata sandi wajib diisi.';
		return !errEmail && !errPw;
	}
</script>

<svelte:head><title>Masuk · EraPay</title></svelte:head>

<div class="flex items-center gap-2 lg:hidden">
	<span class="grid size-9 place-items-center rounded-xl bg-merah-gelap"><Logo /></span>
	<span class="font-display text-xl font-extrabold tracking-[0.18em] text-merah-tua">ERAPAY</span>
</div>

<p class="mt-5 text-sm font-semibold tracking-widest text-merah uppercase lg:mt-0">Selamat datang kembali</p>
<h2 class="font-display mt-1 text-3xl font-extrabold text-merah-gelap">Masuk ke akun Anda</h2>
<p class="mt-2 text-sm text-neutral-500">Kelola pembayaran bisnis Anda dengan aman.</p>

<form
	method="POST"
	novalidate
	class="mt-7 space-y-5"
	use:enhance={({ cancel }) => {
		if (!validate()) {
			auth.goyang();
			cancel();
			return;
		}
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

	<Field id="email" name="email" label="Email" icon="mail" type="email" autocomplete="username" placeholder="nama@usaha.co.id" bind:value={email} error={errEmail} />

	<Field
		id="password" name="password" label="Kata sandi" icon="lock"
		type={showPw ? 'text' : 'password'} autocomplete="current-password" placeholder="••••••••"
		bind:value={password} error={errPw}
	>
		{#snippet trailing()}
			<button
				type="button"
				onclick={() => (showPw = !showPw)}
				aria-label={showPw ? 'Sembunyikan kata sandi' : 'Tampilkan kata sandi'}
				aria-pressed={showPw}
				class="absolute inset-y-0 right-3 grid place-items-center rounded-lg px-1.5 text-neutral-400 transition hover:text-merah"
			>
				<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
					{#if showPw}
						<path d="M3 3l18 18M10.6 5.1A9.9 9.9 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3.2 4M6.5 6.6C3.7 8.3 2 12 2 12s3.5 7 10 7c1.7 0 3.2-.4 4.5-1M9.9 9.9a3 3 0 0 0 4.2 4.2" />
					{:else}
						<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" />
					{/if}
				</svg>
			</button>
		{/snippet}
	</Field>

	{#if form?.needMerchant}
		<Field id="merchant_id" name="merchant_id" label="ID merchant" icon="store" autocomplete="off" placeholder="Tempel ID merchant" bind:value={merchantId} />
	{/if}

	<button
		type="submit"
		disabled={busy}
		class="tombol-kilau relative flex w-full items-center justify-center gap-2 overflow-hidden rounded-xl bg-gradient-to-b from-merah to-merah-tua px-5 py-3.5 font-bold text-white shadow-lg shadow-merah/40 transition hover:-translate-y-0.5 hover:shadow-xl hover:shadow-merah/50 active:translate-y-0 disabled:opacity-90"
	>
		{#if busy}
			<svg class="size-5 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden="true"><circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity=".3" stroke-width="3" /><path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" stroke-width="3" stroke-linecap="round" /></svg>
		{/if}
		<span class="relative">{busy ? 'Memverifikasi…' : 'Masuk dengan aman'}</span>
	</button>
</form>

<p class="mt-6 flex items-center justify-center gap-2 text-xs text-neutral-500">
	<svg viewBox="0 0 24 24" class="size-4 text-emerald-600" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><rect x="5" y="11" width="14" height="9" rx="2" /><path d="M8 11V8a4 4 0 0 1 8 0v3" /></svg>
	Koneksi terenkripsi · Sesi dapat dicabut kapan saja
</p>
<p class="mt-3 text-center text-sm text-neutral-600">
	Belum punya akun?
	<a
		href="/register"
		onclick={(e) => { e.preventDefault(); auth.pindah('/register'); }}
		class="font-bold text-merah hover:underline"
	>Daftar sekarang</a>
</p>
