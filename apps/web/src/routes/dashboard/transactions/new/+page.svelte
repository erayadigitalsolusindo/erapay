<script lang="ts">
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	let { data, form } = $props();
	const { t } = getI18n();
</script>

<p class="muted mb-4 max-w-xl text-xs">{t('trx.newNote')}</p>

<form method="POST" class="card max-w-sm space-y-4 p-6">
	<input type="hidden" name="idempotencyKey" value={data.idempotencyKey} />
	<div>
		<label class="lbl mb-1.5" for="amount">{t('trx.amount')}</label>
		<input id="amount" class="inp" name="amount" inputmode="numeric" pattern="[0-9]*" required value={form?.values?.amount ?? ''} />
	</div>
	<div>
		<label class="lbl mb-1.5" for="reference">{t('trx.reference')}</label>
		<input id="reference" class="inp" name="reference" maxlength="64" required value={form?.values?.reference ?? ''} />
	</div>
	<div>
		<label class="lbl mb-1.5" for="description">{t('trx.description')}</label>
		<input id="description" class="inp" name="description" maxlength="255" value={form?.values?.description ?? ''} />
	</div>
	{#if form?.message}<p class="text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}
	<button type="submit" class="btn btn-red w-full">{t('trx.createQr')}</button>
</form>
