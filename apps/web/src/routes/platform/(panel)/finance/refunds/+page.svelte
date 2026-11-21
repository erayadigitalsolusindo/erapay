<script lang="ts">
	import Page from '../../../../dashboard/finance/refunds/+page.svelte';
	let { data, form } = $props();
</script>

<Page data={data as any} form={form as any} />
