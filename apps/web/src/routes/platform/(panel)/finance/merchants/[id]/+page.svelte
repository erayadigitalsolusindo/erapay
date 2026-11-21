<script lang="ts">
	import Page from '../../../../../dashboard/finance/merchants/[id]/+page.svelte';
	let { data } = $props();
</script>

<Page data={data as any} />
