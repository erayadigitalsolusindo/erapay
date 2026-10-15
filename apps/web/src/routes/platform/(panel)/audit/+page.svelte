<script lang="ts">
	import Combobox from '../../../../lib/components/Combobox.svelte';
	let { data } = $props();
	const fmt = (s: string) => new Date(s).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'medium' });
	const detail = (md: Record<string, unknown>) =>
		Object.entries(md)
			.filter(([k]) => k !== 'session_id')
			.map(([k, v]) => `${k}=${typeof v === 'object' ? JSON.stringify(v) : v}`)
			.join(' · ');
</script>

<svelte:head><title>Audit Log · Panel Platform</title></svelte:head>

<div class="space-y-6">
	<div class="card p-6">
		<h1 class="text-xl font-bold">Audit log</h1>
		<p class="muted mt-1 text-sm">Append-only. Default menampilkan jejak impersonasi DEWA (mulai, setiap request, selesai).</p>
		<form method="GET" class="mt-4 flex flex-wrap gap-2">
			<Combobox id="audit-prefix" name="prefix" class="w-52" value={data.prefix} options={[
				{ value: 'impersonation.', label: 'Impersonasi DEWA' },
				{ value: '', label: 'Semua aksi' },
				{ value: 'auth.', label: 'Autentikasi' }
			]} />
			<input name="merchant_id" value={data.merchant} placeholder="ID merchant (opsional)" class="w-72 rounded-xl border border-ink/15 bg-surface px-3 py-2 text-sm" />
			<button class="rounded-full bg-merah px-4 py-2 text-xs font-bold text-white">Filter</button>
		</form>
	</div>

	{#if data.error}<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{data.error}</p>{/if}

	<div class="card overflow-x-auto">
		<table class="w-full text-left text-xs">
			<thead class="muted border-b border-ink/10 uppercase">
				<tr><th class="p-3">Waktu</th><th class="p-3">Aktor</th><th class="p-3">Aksi</th><th class="p-3">Tenant</th><th class="p-3">Detail</th></tr>
			</thead>
			<tbody class="divide-y divide-ink/10">
				{#each data.rows as r (r.id)}
					<tr class="align-top">
						<td class="p-3 whitespace-nowrap">{fmt(r.created_at)}</td>
						<td class="p-3">{r.actor_email ?? '—'}</td>
						<td class="p-3 font-mono font-bold">{r.action}</td>
						<td class="p-3 font-mono text-[11px]">{r.entity_type === 'merchant' ? r.entity_id : '—'}</td>
						<td class="p-3 break-all">{detail(r.metadata)}{r.request_id ? ` · req=${r.request_id}` : ''}</td>
					</tr>
				{:else}
					<tr><td colspan="5" class="muted p-6">Belum ada catatan.</td></tr>
				{/each}
			</tbody>
		</table>
	</div>
	{#if data.next}
		<a href={`?prefix=${encodeURIComponent(data.prefix)}&merchant_id=${encodeURIComponent(data.merchant)}&before=${data.next}`} class="inline-block rounded-full border border-ink/15 px-4 py-2 text-xs font-bold">Lebih lama →</a>
	{/if}
</div>
