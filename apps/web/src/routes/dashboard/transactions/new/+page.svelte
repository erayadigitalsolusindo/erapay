<script lang="ts">
	let { data, form } = $props();
</script>

<h1>Buat QRIS</h1>
<p class="note">Nominal dalam Rupiah penuh. Kode QR berlaku terbatas dan bisa dibayar sungguhan bila memakai kredensial production.</p>

<form method="POST" class="card">
	<input type="hidden" name="idempotencyKey" value={data.idempotencyKey} />
	<label>Nominal (Rp)
		<input name="amount" inputmode="numeric" pattern="[0-9]*" required value={form?.values?.amount ?? ''} />
	</label>
	<label>Referensi
		<input name="reference" maxlength="64" required value={form?.values?.reference ?? ''} />
	</label>
	<label>Deskripsi (opsional)
		<input name="description" maxlength="255" value={form?.values?.description ?? ''} />
	</label>
	{#if form?.message}<p class="err" role="alert">{form.message}</p>{/if}
	<button type="submit">Buat kode QRIS</button>
</form>

<style>
	.note { color: #6b7280; max-width: 36rem; }
	.card { display: flex; flex-direction: column; gap: 0.75rem; max-width: 24rem; padding: 1.25rem; background: #fff; border: 1px solid #e5e7eb; border-radius: 1rem; }
	label { display: flex; flex-direction: column; gap: 0.25rem; font-size: 0.9rem; }
	input { padding: 0.5rem 0.75rem; border: 1px solid #d1d5db; border-radius: 0.75rem; }
	input:focus { outline: 2px solid #e8b84a; border-color: #c8102e; }
	button { padding: 0.6rem 1rem; border: 0; border-radius: 0.75rem; background: #c8102e; color: #fff; font-weight: 700; cursor: pointer; }
	button:hover { background: #8a0a1f; }
	.err { color: #b91c1c; margin: 0; }
</style>
