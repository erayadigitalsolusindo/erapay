# NFR [nfr] (PRD §8)

Baca juga: `../PRD.md`.

Target engineering pilot (bukan SLA ke merchant): availability API 99,5%/bulan (di luar gangguan upstream terdefinisi); p95 API non-gateway <300 ms; create payment bergantung gateway, timeout ditangani aman; list transaksi paginasi+filter server-side.

**Reliability:** tak ada pembayaran ganda dari retry internal; event penting durable sebelum ack; worker lanjut setelah restart; transaksi ambigu masuk antrean inquiry; RPO/RTO ditetapkan sebelum pilot.

**Observability:** log JSON terstruktur; metrik (rate, latency, error, gateway latency/error per gateway, webhook success/retry, queue age, exception rekonsiliasi); tracing bila layak; alert (error spike, webhook tertunda, queue menumpuk, selisih rekonsiliasi, backup gagal); dashboard kesehatan dependency.

**Skala:** satu deployment Go (worker bisa dipisah); connection pool PG berbatas; tanpa sharding/microservices sebelum ada bukti metrik.
