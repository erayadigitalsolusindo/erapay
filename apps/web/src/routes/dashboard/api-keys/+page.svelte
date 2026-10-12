<script lang="ts">
	import { enhance } from '$app/forms';
	let { data, form } = $props();
	let copied = $state(false);

	const fmt = (iso: string | null) => (iso ? new Date(iso).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' }) : '—');
	const status = (k: { revoked_at: string | null; expires_at: string | null }) =>
		k.revoked_at ? 'Dicabut' : k.expires_at && new Date(k.expires_at) < new Date() ? 'Kedaluwarsa' : 'Aktif';

	async function copy(text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = true;
		} catch {
			copied = false;
		}
	}
</script>

<h1>API Keys</h1>
<p class="note">Key dipakai aplikasi Anda (mis. kasir) untuk membuat pembayaran lewat API EraPay. Secret hanya tampil <strong>sekali</strong> saat dibuat; simpan di tempat aman dan jangan dibagikan.</p>

{#if form?.created}
	<section class="card secret" role="status">
		<h2>Salin key sekarang</h2>
		<p>Setelah halaman ini ditutup, secret tidak bisa dilihat lagi.</p>
		<code>{form.created.secret}</code>
		<button type="button" onclick={() => copy(form.created.secret)}>{copied ? 'Tersalin' : 'Salin'}</button>
		<small>Lingkungan: {form.created.environment} · Izin: {form.created.scopes.join(', ')}</small>
	</section>
{/if}
{#if form?.message}<p class="err" role="alert">{form.message}</p>{/if}

<form method="POST" action="?/create" class="card" use:enhance>
	<h2>Buat key baru</h2>
	<label>Nama
		<input name="name" maxlength="80" required placeholder="mis. Kasir ARUS" value={form?.name ?? ''} />
	</label>
	<fieldset>
		<legend>Izin</legend>
		{#each data.scopes as s (s.id)}
			<label class="chk"><input type="checkbox" name="scopes" value={s.id} checked={s.id !== 'report:read'} /> {s.label} <small>({s.id})</small></label>
		{/each}
	</fieldset>
	<button type="submit">Buat key</button>
</form>

<h2>Daftar key</h2>
{#if data.keys === null}
	<p class="err" role="alert">Daftar key tidak dapat dimuat.</p>
{:else if data.keys.length === 0}
	<p class="note">Belum ada key.</p>
{:else}
	<div class="scroll">
		<table>
			<thead><tr><th>Nama</th><th>Key</th><th>Izin</th><th>Status</th><th>Terakhir dipakai</th><th></th></tr></thead>
			<tbody>
				{#each data.keys as k (k.id)}
					<tr>
						<td>{k.name}</td>
						<td><code>epk_{k.environment === 'production' ? 'prd' : 'sbx'}_{k.prefix}_…</code></td>
						<td>{k.scopes.join(', ')}</td>
						<td>{status(k)}</td>
						<td>{fmt(k.last_used_at)}</td>
						<td class="act">
							{#if status(k) === 'Aktif'}
								<form method="POST" action="?/rotate" use:enhance class="inline">
									<input type="hidden" name="id" value={k.id} />
									<label class="grace">Tenggang
										<select name="grace"><option value="0">0</option><option value="3600" selected>1 jam</option><option value="86400">24 jam</option></select>
									</label>
									<button type="submit">Rotasi</button>
								</form>
								<form
									method="POST"
									action="?/revoke"
									class="inline"
									use:enhance={({ cancel }) => {
										if (!confirm(`Cabut key "${k.name}"? Aplikasi yang memakainya langsung berhenti bekerja.`)) cancel();
									}}
								>
									<input type="hidden" name="id" value={k.id} />
									<button type="submit" class="danger">Cabut</button>
								</form>
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<style>
	.note { color: #6b7280; max-width: 40rem; }
	.card { display: flex; flex-direction: column; gap: 0.75rem; max-width: 28rem; padding: 1.25rem; margin: 1rem 0; background: #fff; border: 1px solid #e5e7eb; border-radius: 1rem; }
	.secret { border-color: #e8b84a; background: #fff8f0; max-width: 40rem; }
	.secret code { overflow-wrap: anywhere; padding: 0.6rem; background: #fff; border: 1px solid #e5e7eb; border-radius: 0.5rem; user-select: all; }
	label { display: flex; flex-direction: column; gap: 0.25rem; font-size: 0.9rem; }
	.chk { flex-direction: row; align-items: center; gap: 0.5rem; }
	fieldset { border: 1px solid #e5e7eb; border-radius: 0.75rem; padding: 0.5rem 0.75rem; }
	input:not([type]) { padding: 0.5rem 0.75rem; border: 1px solid #d1d5db; border-radius: 0.75rem; }
	input:focus, select:focus, button:focus-visible { outline: 2px solid #e8b84a; }
	button { padding: 0.5rem 1rem; border: 0; border-radius: 0.75rem; background: #c8102e; color: #fff; font-weight: 700; cursor: pointer; }
	button:hover { background: #8a0a1f; }
	button.danger { background: #fff; color: #b91c1c; border: 1px solid #b91c1c; }
	.err { color: #b91c1c; }
	.scroll { overflow-x: auto; }
	table { border-collapse: collapse; width: 100%; font-size: 0.9rem; background: #fff; }
	th, td { text-align: left; padding: 0.5rem 0.75rem; border-bottom: 1px solid #e5e7eb; vertical-align: middle; }
	th { background: #fff8f0; font-size: 0.8rem; }
	.act { display: flex; gap: 0.5rem; flex-wrap: wrap; }
	.inline { display: flex; gap: 0.4rem; align-items: center; }
	.grace { flex-direction: row; align-items: center; font-size: 0.8rem; }
</style>
