<script lang="ts">
	let { class: kelas = 'h-9' }: { class?: string } = $props();
</script>

<!-- Logo resmi EraPay dengan outline putih mengikuti bentuk objek agar tetap terbaca di latar merah. -->
<img src="/erapay-logo-outline.svg" alt="EraPay" class="w-auto {kelas}" draggable="false" />
