<script lang="ts">
	import Page from '../../../../dashboard/finance/merchants/+page.svelte';
	let { data, form } = $props();
</script>

<Page data={data as any} form={form as any} />
