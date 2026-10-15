<script lang="ts">
	import { enhance } from '$app/forms';

	let { data, form } = $props();
	let open = $state<string | null>(null);
	const fmt = (s: string) => new Date(s).toLocaleDateString('id-ID', { day: '2-digit', month: 'short', year: 'numeric' });
</script>

<svelte:head><title>Masuk sebagai Tenant · Panel Platform</title></svelte:head>

<div class="space-y-6">
	<div class="card p-6">
		<h1 class="text-xl font-bold">Masuk sebagai tenant</h1>
		<p class="muted mt-1 text-sm">
			Operasikan dashboard merchant sebagai pemiliknya. Alasan <b>wajib</b>, sesi berlaku 60 menit, dan setiap aksi
			tercatat di <a href="/platform/audit" class="underline">Audit log</a> atas nama akun DEWA Anda.
		</p>
		<form method="GET" class="mt-4 flex gap-2">
			<input name="q" value={data.q} placeholder="Cari nama / ID merchant" class="w-full max-w-sm rounded-xl border border-ink/15 bg-surface px-3 py-2 text-sm" />
			<button class="rounded-full bg-merah px-4 py-2 text-xs font-bold text-white">Cari</button>
		</form>
	</div>

	{#if data.error}<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{data.error}</p>{/if}

	<div class="card divide-y divide-ink/10">
		{#each data.merchants as m (m.id)}
			<div class="p-4">
				<div class="flex flex-wrap items-center justify-between gap-3">
					<div class="min-w-0">
						<div class="truncate text-sm font-bold">{m.display_name}</div>
						<div class="muted text-xs">{m.legal_name} · {m.onboarding_status} · {fmt(m.created_at)} · <code class="font-mono text-[11px]">{m.id}</code></div>
					</div>
					<button
						type="button"
						onclick={() => (open = open === m.id ? null : m.id)}
						class="rounded-full border border-merah px-3.5 py-1.5 text-xs font-bold text-merah transition hover:bg-merah hover:text-white"
					>
						Masuk sebagai…
					</button>
				</div>
				{#if open === m.id || form?.merchantId === m.id}
					<form method="POST" action="?/impersonate" use:enhance class="mt-3 flex flex-wrap items-start gap-2">
						<input type="hidden" name="merchant_id" value={m.id} />
						<textarea
							name="reason"
							required
							minlength="10"
							maxlength="500"
							rows="2"
							placeholder="Alasan (mis. tiket support #123: merchant tidak bisa membuat API key)"
							class="min-w-64 flex-1 rounded-xl border border-ink/15 bg-surface px-3 py-2 text-sm">{form?.merchantId === m.id ? form.reason : ''}</textarea>
						<button class="rounded-full bg-merah px-4 py-2 text-xs font-bold text-white">Masuk &amp; catat</button>
						{#if form?.merchantId === m.id && form.message}
							<p class="w-full text-xs font-semibold text-red-700" role="alert">{form.message}</p>
						{/if}
					</form>
				{/if}
			</div>
		{:else}
			<p class="muted p-6 text-sm">Tidak ada merchant yang cocok.</p>
		{/each}
	</div>
</div>
