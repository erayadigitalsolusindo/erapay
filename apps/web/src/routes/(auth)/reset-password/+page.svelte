<script lang="ts">
	import { enhance } from '$app/forms';
	import { getContext } from 'svelte';
	import Field from '../../../lib/components/auth/Field.svelte';

	let { data, form } = $props();
	const auth = getContext<{ goyang(): void; pindah(href: string): void }>('auth');
	let sandi = $state('');
	let sandi2 = $state('');
	let err = $state('');
	let busy = $state(false);

	$effect(() => {
		if (form?.error) auth.goyang();
	});
</script>

<svelte:head><title>Atur ulang kata sandi · EraPay</title></svelte:head>

<p class="text-sm font-semibold tracking-widest text-merah uppercase">Pemulihan akun</p>
<h2 class="font-display mt-1 text-3xl font-extrabold text-merah-gelap">Buat kata sandi baru</h2>

{#if form?.done}
	<div class="mt-6 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-4 text-sm/6 text-emerald-900" role="status">
		Kata sandi berhasil diubah. Semua sesi login lama sudah ditutup.
	</div>
	<a href="/login" class="mt-5 block w-full rounded-xl bg-gradient-to-b from-merah to-merah-tua px-5 py-3.5 text-center font-bold text-white shadow-lg shadow-merah/40">Masuk</a>
{:else if !data.token || form?.expired}
	<div class="mt-6 rounded-xl border border-red-200 bg-red-50 px-4 py-4 text-sm/6 text-red-800" role="alert">
		Tautan tidak valid, sudah kedaluwarsa, atau sudah dipakai.
	</div>
	<a href="/forgot-password" class="mt-5 block text-center text-sm font-bold text-merah hover:underline">Minta tautan baru</a>
{:else}
	<p class="mt-2 text-sm text-neutral-500">Minimal 12 karakter. Semua sesi login yang sedang aktif akan ditutup.</p>
	<form
		method="POST"
		novalidate
		class="mt-7 space-y-5"
		use:enhance={({ cancel }) => {
			err = sandi.length < 12 || sandi.length > 128 ? 'Kata sandi 12 sampai 128 karakter.' : sandi !== sandi2 ? 'Kata sandi tidak sama.' : '';
			if (err) {
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
		<input type="hidden" name="token" value={data.token} />
		<Field id="password" name="password" label="Kata sandi baru" icon="lock" type="password" autocomplete="new-password" placeholder="Minimal 12 karakter" bind:value={sandi} error={err} />
		<Field id="password_confirm" name="password_confirm" label="Ulangi kata sandi" icon="lock" type="password" autocomplete="new-password" placeholder="Ketik ulang kata sandi" bind:value={sandi2} />
		<button
			type="submit"
			disabled={busy}
			class="relative flex w-full items-center justify-center gap-2 rounded-xl bg-gradient-to-b from-merah to-merah-tua px-5 py-3.5 font-bold text-white shadow-lg shadow-merah/40 transition hover:-translate-y-0.5 disabled:opacity-90"
		>{busy ? 'Menyimpan…' : 'Simpan kata sandi'}</button>
	</form>
{/if}
