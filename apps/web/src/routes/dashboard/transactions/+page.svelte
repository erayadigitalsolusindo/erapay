<script lang="ts">
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	const { t } = getI18n();
</script>

<div class="card p-6">
	<div class="flex flex-wrap items-center justify-between gap-3">
		<div>
			<h2 class="font-bold">{t('trx.list')}</h2>
			<p class="muted mt-1 text-xs">{t('trx.listEmpty')}</p>
		</div>
		<a href="/dashboard/transactions/new" class="btn btn-red">＋ {t('trx.new')}</a>
	</div>
</div>
