<script lang="ts">
	import { enhance } from '$app/forms';
	import { untrack } from 'svelte';
	import Field from '../../../../lib/components/auth/Field.svelte';

	let { form } = $props();
	let email = $state(untrack(() => form?.email ?? ''));
	let password = $state('');
	let mfaCode = $state('');
	let challengeId = $state(untrack(() => form?.challenge_id ?? ''));
	let mfa = $state(untrack(() => form?.mfa ?? false));
	let showPw = $state(false);
	let busy = $state(false);

	$effect(() => {
		if (form?.mfa) {
			mfa = true;
			challengeId = form.challenge_id ?? challengeId;
			email = form.email ?? email;
		}
	});
</script>

<svelte:head><title>Login Panel Platform · EraPay</title></svelte:head>

{#if form?.error}
	<div class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">{form.error}</div>
{/if}

<form method="POST" novalidate class="space-y-4" use:enhance>
	{#if mfa}
		<p class="text-sm font-medium text-neutral-600">
			Login 2FA untuk <b class="text-merah">{email}</b>
		</p>
		<input type="hidden" name="email" value={email} />
		<input type="hidden" name="password" value={password} />
		<input type="hidden" name="challenge_id" value={challengeId} />
		<Field id="mfa_code" name="mfa_code" label="Kode OTP / recovery code" icon="lock" autocomplete="one-time-code" placeholder="123456" bind:value={mfaCode} />
	{:else}
		<Field id="email" name="email" label="Email" icon="mail" type="email" autocomplete="username" placeholder="dewa@erapay.test" bind:value={email} />
		<Field id="password" name="password" label="Kata sandi" icon="lock" type={showPw ? 'text' : 'password'} autocomplete="current-password" placeholder="••••••••" bind:value={password}>
			{#snippet trailing()}
				<button
					type="button"
					onclick={() => (showPw = !showPw)}
					aria-label={showPw ? 'Sembunyikan kata sandi' : 'Tampilkan kata sandi'}
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
	{/if}

	<button
		type="submit"
		disabled={busy}
		class="w-full rounded-xl bg-gradient-to-b from-merah to-merah-tua px-5 py-3.5 font-bold text-white shadow-lg shadow-merah/40 transition hover:-translate-y-0.5 disabled:opacity-90"
	>
		{mfa ? 'Verifikasi 2FA' : 'Masuk ke panel'}
	</button>
</form>

<p class="mt-5 text-center text-xs text-neutral-500">
	Hanya akun <b>DEWA</b> (Platform Admin) yang dapat masuk ke panel ini.
</p>
