<script lang="ts">
	import { enhance } from '$app/forms';
	import Field from '../../../../lib/components/auth/Field.svelte';

	let { data, form } = $props();
	let busy = $state(false);
	let token = $state('');
	let email = $state('');
	let password = $state('');
	const status = $derived(data.status);
	const outcome = $derived(form?.status);
</script>

<svelte:head><title>Setup Panel Platform · EraPay</title></svelte:head>

{#if outcome === 'created'}
	<div class="text-center">
		<div class="mx-auto grid size-14 place-items-center rounded-full bg-emerald-100 text-emerald-700">✓</div>
		<h2 class="mt-4 text-lg font-bold text-neutral-800">DEWA dibuat</h2>
		<p class="mt-2 text-sm text-neutral-600">
			Login di <a href="/platform/login" class="font-bold text-merah underline">/platform/login</a> lalu selesaikan pengaturan 2FA.
		</p>
	</div>
{:else if status === null}
	<p class="text-sm text-red-700">Data setup tidak dapat dimuat.</p>
{:else if status.done}
	<div class="text-center">
		<div class="mx-auto grid size-14 place-items-center rounded-full bg-emerald-100 text-emerald-700">✓</div>
		<h2 class="mt-4 text-lg font-bold text-neutral-800">Panel sudah aktif</h2>
		<p class="mt-2 text-sm text-neutral-600">
			DEWA platform sudah ada. <a href="/platform/login" class="font-bold text-merah underline">Masuk ke panel</a>.
		</p>
	</div>
{:else if !status.enabled}
	<p class="text-sm text-neutral-600">
		Setup belum dikonfigurasi (env <code class="rounded bg-neutral-100 px-1">ERAPAY_PLATFORM_SETUP_TOKEN</code> tidak terpasang / sudah ditarik).
		<a href="/platform/login" class="font-bold text-merah underline">Masuk ke panel</a> bila DEWA sudah ada.
	</p>
{:else}
	<div>
		{#if form?.message}
			<div class="mb-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">{form.message}</div>
		{/if}
		<p class="mb-4 text-sm text-neutral-600">
			Bootstrap DEWA pertama (sekali saja). Pengguna ini langsung terverifikasi dan hanya bisa login lewat panel.
		</p>
		<form method="POST" novalidate class="space-y-4" use:enhance={({ cancel }) => {
			busy = true;
			return async () => { busy = false; };
		}}>
			<Field id="token" name="token" label="Token setup" icon="lock" autocomplete="off" placeholder="ERAPAY_PLATFORM_SETUP_TOKEN" bind:value={token} />
			<Field id="email" name="email" label="Email DEWA" icon="mail" type="email" autocomplete="off" placeholder="dewa@erapay.test" bind:value={email} />
			<Field id="password" name="password" label="Kata sandi" icon="lock" type="password" autocomplete="new-password" placeholder="min. 12 karakter" bind:value={password} />
			<button
				type="submit"
				disabled={busy}
				class="w-full rounded-xl bg-gradient-to-b from-merah to-merah-tua px-5 py-3.5 font-bold text-white shadow-lg shadow-merah/40 transition hover:-translate-y-0.5 disabled:opacity-90"
			>
				Buat DEWA & aktifkan panel
			</button>
		</form>
	</div>
{/if}
