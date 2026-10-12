<script lang="ts">
	import { getI18n } from '../../lib/i18n/i18n.svelte';
	const { t } = getI18n();
</script>

<div class="card p-6">
	<h2 class="text-sm font-bold">{t('overview.title')}</h2>
	<p class="muted mt-2 text-xs">{t('overview.empty')}</p>
</div>
