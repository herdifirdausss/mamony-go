# Mamony Go — Engineering Principles & Design Handbook

**Status:** Draft guideline, 21 November 2026. **Relation:** Companion to Mamony Go Production Workflow v5, §12–§17. Ready to copy to `docs/standards/engineering-principles.md` after owner approval. Section numbering follows the full workflow for easy cross-references.

## 12. Prinsip rekayasa perangkat lunak — dari slogan menjadi keputusan

**Status:** Usulan kebijakan engineering untuk Mamony Go (belum merupakan ADR Accepted). **Lingkup:** seluruh perubahan. **Sumber:** rangkuman dan interpretasi untuk Mamony dari [Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), [Organizing Go code](https://go.dev/blog/organizing-go-code), [Martin Fowler — YAGNI](https://martinfowler.com/bliki/Yagni.html), serta [Refactoring.Guru](https://refactoring.guru/). Ini bukan aturan resmi dari sumber yang sama untuk semua bahasa; sebagian merupakan kebijakan yang **diusulkan** khusus Mamony.

### 12.1 Urutan prioritas bila prinsip saling bertentangan

1. **Correctness dan domain invariants.** Arti saldo, double-entry, idempotency, keamanan tenant, pembulatan, dan retry tetap sesuai persyaratan yang disetujui.
2. **Security, integrity, dan recoverability.** Jangan meniadakan pemeriksaan akses, audit, transaksi, validasi, atau rollback demi kode lebih singkat.
3. **Clarity dan simplicity.** Pilih desain paling sederhana yang memenuhi (1) dan (2); satu jalan yang mudah diikuti lebih baik daripada framework baru tanpa kebutuhan terbukti.
4. **Changeability.** Jaga coupling rendah dan cohesion tinggi pada boundary yang benar-benar ada; refactor sewaktu kebutuhan perubahan diketahui.
5. **Performance dan scalability.** Optimasi berdasarkan budget, profiling, kapasitas, dan beban yang dibuktikan, bukan dugaan.

Prinsip bersifat **heuristik pengambilan keputusan**, bukan kata kunci yang bisa otomatis membuktikan kualitas. Linter dapat memeriksa beberapa gejala, tetapi tidak bisa menyimpulkan SRP, YAGNI, atau design pattern yang benar secara semantik.

### 12.2 Katalog prinsip inti dan contoh di Mamony

| Prinsip | Arti praktis | Contoh diterapkan | Salah kaprah / pengecualian | Cara review |
|---|---|---|---|---|
| **KISS** (Keep It Simple) | Solusi seminimal mungkin yang tetap memenuhi kontrak dan failure path. | Mulai dari `net/http`, SQL jelas, constructor eksplisit, dan satu binary. | “Paling sedikit baris” bukan berarti paling sederhana; menghilangkan constraint DB bisa mengubah correctness. | Jelaskan mengapa setiap layer/dep ada; tes skenario sulit. |
| **YAGNI** (You Aren't Gonna Need It) | Jangan membangun kemampuan spekulatif hanya karena mungkin dibutuhkan. | Tidak menambahkan Kafka, plugin engine, multi-region, atau 5 provider abstraction sebelum ada pemicu. | Tidak berlaku untuk requirement yang sudah approved atau kontrol keselamatan yang memang dibutuhkan saat ini. | Hubungkan fitur/abstraksi dengan AC/ADR atau data nyata. |
| **DRY** (Don't Repeat Yourself) | Satu pengetahuan/aturan domain otoritatif, hindari logika yang dapat menyimpang. | Definisi currency exponent dan permission berasal dari sumber tunggal yang jelas. | Dua baris SQL mirip bukan otomatis duplikasi pengetahuan; shared helper lintas capability bisa menciptakan coupling buruk. | Cari perubahan aturan yang harus diedit di banyak tempat. |
| **SoC** (Separation of Concerns) | Parsing HTTP, aturan bisnis, dan persistence memiliki tanggung jawab berbeda. | `handler` parse DTO, `usecase` orkestrasi, `repository` SQL. | Jangan membuat satu package per fungsi jika boundary belum mandiri. | Lihat jalur perubahan dan import graph. |
| **High cohesion / low coupling** | Bagian yang berubah karena alasan sama ditempatkan bersama, dependency lintas boundary diperkecil. | Posting/reversal/invariant tetap C01; C03 memakai port, bukan query ke schema C01. | Paket kecil tidak selalu kohesif; banyak interface bisa menambah coupling konseptual. | Review dependency direction dan change history. |
| **Encapsulation / information hiding** | Detail yang dapat berubah disembunyikan di balik surface yang stabil. | SQL/pgx tidak bocor dari façade ledger atau kontrak payment. | Jangan menyembunyikan aturan domain sampai tidak bisa diuji/ditelusuri. | Periksa exported API dan penggunaan tipe eksternal. |
| **Explicitness over magic** | Keputusan dan dependency penting bisa dilacak dari kode dan konfigurasi. | Constructor injection; route/permission inventory; transaksi dinyatakan di use case. | Duplikasi konfigurasi manual tanpa sinkronisasi juga buruk. | Telusuri entry point → action → DB dan hasil check wiring. |
| **Least astonishment** | Perilaku fungsi/API sesuai nama, tipe, serta kontrak yang diharapkan. | `Post` tidak diam-diam mengirim callback provider; `GetBalance` bukan operasi mutasi. | Jangan mengasumsikan arti `amount` tanpa currency, unit, atau kontrak. | Contract tests serta nama/deskripsi API. |
| **Fail fast on invalid setup / fail closed on authorization** | Konfigurasi tidak valid menghentikan startup; scope/izin kosong ditolak. | Startup menolak secret/config salah; tenant context kosong tidak memberi read. | Jangan fail-fast dengan menggagalkan semua request karena kesalahan satu provider eksternal yang bisa diisolasi. | Invalid-config, RLS, permission tests. |
| **Defensive boundaries, trusted internals** | Validasi penuh di trust boundary, lalu gunakan domain types yang menjaga invariant. | Parse jumlah nominal di handler; `Money`/`Currency` valid di domain; constraint tetap di DB. | Validasi berulang tanpa alasan pada setiap fungsi internal bisa mengaburkan desain. | Input boundary dan invariant tests. |
| **Composition over inheritance** | Gabungkan fungsi, struct, dan interface kecil daripada hierarki subtype. | Middleware `http.Handler`, adapter provider, injected clock/port. | Meniru OOP inheritance dengan embedded struct berlapis tanpa kebutuhan. | Review API Go idiomatik. |
| **Make invalid states hard to represent** | Modelkan transisi, tipe dan constraint untuk membatasi keadaan tidak sah. | Status payment divalidasi; journal balancing di DB; amount bukan `float64`. | Struct tidak menjamin semua invariant tanpa constructor, DB constraints, dan tests. | Property test, invalid transition, DB check. |
| **Single source of truth** | Setiap requirement, schema, dan kontrak punya pemilik serta status. | PRD/ADR untuk kebijakan; OpenAPI untuk wire; migrations untuk schema. | Jangan memperbanyak “aturan final” di skill, prompt, README dan kode yang mudah berbeda. | Traceability, drift check. |
| **Principle of least privilege** | Akses minimum untuk role/runtime/tool dan least privilege sepanjang waktu. | C03 runtime DB role tidak dapat write ledger; CI PR tidak punya secret prod. | Service internal tidak otomatis dipercaya; authn bukan authz. | Grants tests dan permission inventory. |
| **Boy Scout Rule** | Saat mengubah suatu area, tinggalkan lebih jelas tanpa perluasan scope liar. | Rename helper ambigu, tambah tes reproduksi terkait bug. | Tidak mengizinkan refactor besar tak terkait pada PR settlement. | Review scope diff dan perilaku tetap. |
| **Rule of Three** (heuristik) | Pertimbangkan abstraction setelah ada beberapa kasus independen dan kemiripan semantik terbukti. | Setelah beberapa adapter provider benar-benar punya workflow sama, evaluasi shared contract. | Bukan hukum “harus tiga”; dua kasus dapat cukup bila constraint jelas, satu kasus dapat perlu abstraksi demi test/boundary. | Justifikasi abstraksi dan alternatif sederhana. |
| **Avoid premature optimization** | Tetapkan target dulu, ukur, baru optimasi. | Tuning index dari `EXPLAIN (ANALYZE, BUFFERS)` di fixture representatif. | Jangan menunda constraint safety atau timeout yang merupakan kebutuhan dasar. | Benchmark/profiling, baseline, regression. |
| **Keep changes reversible** | Perubahan fitur dan schema dipotong menjadi tahapan teruji dan dapat dipulihkan. | Expand–migrate–switch–contract; flag untuk routing baru. | Reversal ledger adalah *business operation* tersendiri, bukan mengedit jurnal yang immutable. | Migration upgrade, rollout, restore evidence. |
| **Observability as design** | Rencanakan bukti runtime saat mendesain behavior penting. | request correlation, ledger mismatch count, outbox age, status provider unknown. | Log detail keuangan/PII/secret berlebihan melanggar prinsip minimisasi data. | Check log schema, safe fields, smoke alerts. |

### 12.3 SOLID dalam Go: gunakan intensi, jangan meniru Java

[SOLID](https://refactoring.guru/) berasal dari tradisi object-oriented design. Untuk Mamony Go, terjemahkan ke **paket, fungsi, struct, interface, kontrak, dan dependency direction**, bukan memaksa inheritance atau membuat interface di semua layer.

| Huruf | Pertanyaan review | Implementasi Go yang disarankan | Contoh pelanggaran |
|---|---|---|---|
| **S — Single Responsibility** | Apakah bagian ini berubah karena beberapa alasan yang tidak berkaitan? | Handler transport, use case bisnis, repo persistence; satu capability tetap menguasai invariant-nya. | `ConfirmPayment` sekaligus parse HTTP, menulis SQL, menentukan FX, format JSON, mengirim email. |
| **O — Open/Closed** | Adakah variasi yang sudah nyata dan seharusnya bisa ditambah tanpa mengubah kebijakan inti? | Port kecil + adapter, *jika* variasinya terbukti (misalnya provider simulator dengan dua implementasi). | Satu switch besar di domain yang mengimpor semua SDK provider; **tetapi** switch kecil yang eksplisit untuk 2 status tetap boleh. |
| **L — Liskov Substitution** | Jika implementasi port diganti, apakah kontrak hasil dan error tetap berlaku? | Contract tests lintas adapter: timeout, status unknown, idempotency, cancellation, retry eligibility. | Fake provider selalu sukses sementara production adapter bisa memberi unknown; use case mengasumsikan semua error aman untuk retry. |
| **I — Interface Segregation** | Apakah consumer dipaksa mengetahui metode yang tidak dipakainya? | Interface kecil di sisi pemakai, atau parameter fungsi konkret bila belum perlu interface. | `AllFinancialServices` dengan puluhan metode di-import oleh setiap module. |
| **D — Dependency Inversion** | Apakah kebijakan bisnis bergantung pada detail SQL/HTTP? | Use case bergantung pada kontrak dengan tipe domain, module root/app menyuntikkan adapter. | `internal/modules/payment/internal/usecase` mengimpor `pgxpool`, HTTP framework, atau SDK vendor. |

**Contoh port minimal dari sisi consumer** (ilustrasi, bukan kontrak final C01/C03):

```go
// Package usecase pada capability payment mendefinisikan kebutuhan sempitnya.
type LedgerPoster interface {
    Post(ctx context.Context, cmd PostCommand) (PostResult, error)
}

type Service struct {
    ledger LedgerPoster
}

func New(ledger LedgerPoster) *Service {
    return &Service{ledger: ledger}
}
```

`PostCommand`/`PostResult` harus memakai tipe yang disetujui pada package kontrak yang dapat dijangkau app; cuplikan ini sengaja hanya menunjukkan **arah dependensi**. Implementasi `LedgerPoster` disediakan adapter di composition root yang menerjemahkan kontrak payment ↔ ledger. Interface tidak perlu dibuat bila consumer hanya membutuhkan fungsi konkret lokal dan tidak ada boundary yang dibedakan. Sumber Go: [Code Review Comments — Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces), [Package names](https://go.dev/blog/package-names).

### 12.4 Aturan menyelesaikan konflik prinsip

| Jika muncul konflik | Putusan awal |
|---|---|
| YAGNI vs security/ledger safeguards | Requirement integritas, audit, permission, dan recovery yang telah disetujui **menang**. YAGNI hanya menolak fitur spekulatif. |
| DRY vs module independence | Pertahankan duplikasi kecil yang belum berbagi invariant; jangan menghubungkan C01 dan C03 lewat `utils` global. |
| OCP vs KISS | Gunakan `switch` yang jelas untuk dua variasi ketika lebih sederhana; ekstrak Strategy bila variasi dan perilaku divergen nyata. |
| SRP vs terlalu banyak file/package | Gunakan file terpisah dalam package yang sama terlebih dahulu; package baru setelah batas owner/test jelas. |
| Performance vs correctness | Buktikan invariant dulu; optimasi hanya jika maintain semantics, atomicity, dan replay. |
| Testability vs dependency injection kompleks | Pilih seam paling kecil (interface consumer, function param, injected clock) tanpa framework IoC. |

**Keputusan AI:** sebelum menambah layer/interface/abstraction/pattern/dep, sebutkan (a) masalah terukur, (b) alternatif lebih sederhana, (c) acceptance criteria yang diuntungkan, dan (d) risiko tambahan. Jika tidak ada bukti, jangan tambah.

## 13. Refactoring, code smells, dan disiplin perubahan

**Basis:** [Refactoring.Guru — Refactoring](https://refactoring.guru/refactoring), [Code Smells](https://refactoring.guru/refactoring/smells), [How to refactor](https://refactoring.guru/refactoring/how-to), [Catalog](https://refactoring.guru/refactoring/catalog), [Martin Fowler — Opportunistic Refactoring](https://martinfowler.com/bliki/OpportunisticRefactoring.html). Sumber-sumber ini memaparkan indikasi dan teknik; **bukan** perintah melakukan setiap refactor secara otomatis.

### 13.1 Definisi dan guardrail Mamony

- **Refactoring** mengubah struktur internal **tanpa sengaja mengubah perilaku teramati**. Bug fix dan feature change adalah aktivitas berbeda; pisahkan commit atau diff jika mungkin agar review mudah.
- Identifikasi behavior yang dilindungi: HTTP status/body, authz, SQL side effects, event schema, monetary semantics, time/retry/idempotency behavior, migration compatibility. Untuk ledger, perilaku internal yang tampak tidak penting pun dapat mempengaruhi hasil finansial.
- Tulis characterization/regression tests pada area yang masih minim test; jalankan tests sebelum dan sesudah. Simpan snapshot hasil yang bermakna; jangan memakai snapshot besar yang tidak tervalidasi sebagai satu-satunya bukti.
- Lakukan transformasi kecil, `go test` pada package terkena, lalu cek risiko yang lebih luas. `gofmt`, `go vet`, lint dan integration menyusul sesuai kelas.
- Jangan mengubah behavior dan menyebutnya “refactor”; dokumentasikan sebagai keputusan/fitur dengan AC sendiri.

### 13.2 Smell → refactoring → batas penilaian

| Smell atau sinyal | Teknik kandidat | Contoh Mamony | Jangan otomatis bila… |
|---|---|---|---|
| Long function / deep nesting | Extract function; guard clauses; pecah fase bisnis | `Confirm` menyatu validasi, state transition, persistence, response mapping | Memecah satu transaksi ke fungsi yang menyembunyikan boundary commit/rollback. |
| Large package / multiple reasons to change | Move function; extract cohesive package | HTTP DTO dan SQL query bercampur di `payment` | File banyak tetapi masih satu model bisnis kohesif. |
| Primitive obsession | Introduce domain type; validate constructor | `int64` nominal tanpa currency, `string` sembarang untuk state atau ID | Hanya alias tanpa invariant dan malah mengaburkan wire format. |
| Duplicate *knowledge* | Extract shared rule di owner | Satu rumus FX rounding digunakan di quote, settlement dan test oracle | Kode mirip punya lifecycle/domain owner yang berbeda. |
| Long parameter list | Parameter object kecil, jika menjernihkan | `Post(ctx, account, amount, currency, reference, ...)` | Menyembunyikan semua dependency dalam map/generic bag. |
| Feature envy / domain logic di handler | Move logic ke domain/usecase | Handler menghitung debits dan credits sendiri | Mapping transport murni memang tugas handler. |
| Shotgun surgery | Tetapkan owner untuk policy tunggal | Satu permission berubah dan perlu edit 8 file tak terkait | Banyak kontrak memang harus berubah secara eksplisit karena compatibility. |
| Speculative generality / unused abstraction | Inline/delete dead code, deprecate aman | Generic provider plugin yang belum punya provider kedua | Ada approved requirement atau extension point keamanan yang nyata. |
| Excessive indirection / middleman | Inline layer atau kecilkan API | Handler → manager → facade → service → interactor → repo tanpa fungsi berbeda | Lapisan itu boundary otorisasi/transaksi atau contract yang nyata. |
| Mixed error handling | Introduce classification di satu boundary | SQLSTATE, not-found, retryable dan provider-unknown disamakan menjadi HTTP 500 | Pemetaan khusus per boundary memang dibutuhkan. |
| Hidden mutable global state | Inject dependency atau buat ownership explicit | `var pool *pgxpool.Pool`, mutable settings, clock global | Nilai statis immutable dan benar-benar tanpa lifecycle. |
| Concurrency without owner | Simplify to synchronous, add cancellation/owner | Goroutine dari handler memproses settlement setelah request selesai tanpa jaminan durable | Ada job runner/queue/outbox dengan shutdown dan retry yang jelas. |

### 13.3 Urutan refactoring yang aman

```text
1. Identify: sebutkan smell, dampak, boundary dan risiko.
2. Characterize: catat behavior yang harus tetap, test awal dan sumber kontrak.
3. Plan: pilih transformasi terkecil + rollback.
4. Change: satu perubahan struktural; hindari perubahan aturan domain.
5. Verify: go test terarah; integration/contract/concurrency jika tersentuh.
6. Review: diff behavior, import graph, DB side effects dan performa relevan.
7. Record: PR menyebut behavior unchanged + bukti; dokumentasi bila path berubah.
```

**Contoh keputusan:** jika `confirm.go` mempunyai 250 baris, **angka 250 bukan bukti pelanggaran**. Periksa apakah ada tanggung jawab yang berbeda, cognitive load, kebutuhan test, serta apakah extraction menyembunyikan transaksi. Sebaliknya, file 40 baris yang memasukkan `float64` ke ledger adalah masalah serius walaupun singkat.

### 13.4 Refactoring ketika ada perubahan database/API

- Mengubah nama atau memindahkan fungsi Go dalam satu package: biasanya kelas S jika behavior tetap dan kontrak eksternal tidak berubah; naik kelas jika menyentuh permission atau transaction path.
- Memindahkan business logic dari handler ke usecase: uji HTTP contract dan domain errors; pastikan tidak mengubah semantik response.
- Memindahkan repository atau SQL: uji SQLSTATE, isolation, row lock, `Rows.Close`, `rows.Err`, transaction ownership, RLS, connection reuse, dan runtime grants.
- Mengubah public API / events / DB column: **bukan refactoring murni** jika consumer dapat mengamati perubahan. Pakai compatibility contract dan rencana rollout/migration.
- Refactor kritis ledger: jalankan invariants, replay, concurrency, rollback, verifier dan outbox tests sebelum mengklaim behavior-preserving.

## 14. Design patterns — katalog selektif yang idiomatik untuk Go

**Sumber:** [Refactoring.Guru — design patterns](https://refactoring.guru/design-patterns), [Contoh Go](https://refactoring.guru/design-patterns/go), [Go package design](https://go.dev/blog/organizing-go-code). **Aturan:** pattern adalah nama solusi atas masalah yang *sudah ada*, bukan checklist yang harus diimplementasikan semua. Contoh berbasis object/class di referensi perlu diadaptasi ke komposisi Go. Bedakan *Gang of Four (GoF)* dari pola arsitektur sistem terdistribusi/enterprise.

### 14.1 Pola GoF yang kemungkinan berguna lebih awal

| Pattern | Kategori | Kebutuhan nyata yang menjadi pemicu | Penerapan Go / Mamony | Risiko penggunaan berlebihan |
|---|---|---|---|---|
| **Adapter** | Structural | Kontrak C03 butuh semantik berbeda dari API C01 atau SDK C05. | Struct/function adapter di `internal/app/adapters` menerjemahkan DTO, error, dan result. | Menganggap setiap function call perlu adapter. |
| **Facade** | Structural | Subsystem perlu surface kecil dan stabil. | `ledger.New(...); ledgerModule.Post(...)` tanpa membocorkan repo internal. | Menambah façade bertingkat tanpa aturan boundary. |
| **Strategy** | Behavioral | Ada beberapa kebijakan nyata dengan kontrak sama: mis. FX quote selection atau simulator failure mode. | Function type atau interface kecil di consumer; dites sebagai contract. | Membuat registry/plugin engine padahal satu implementasi. |
| **Decorator** | Structural | Perilaku lintas request: metrics, tracing, logging, auth gate. | Bungkus `http.Handler` atau implementasi port dengan wrapper eksplisit. | Rangkaian wrapper menyembunyikan transaction context atau mencatat data rahasia. |
| **State** | Behavioral | State payment dan transisi cukup kompleks. | Mulai dari enum + explicit transition function/table + exhaustive tests; object-State hanya bila memberi nilai. | Satu kelas/struct per status untuk workflow yang sederhana. |
| **Chain of Responsibility** | Behavioral | HTTP middleware berurutan dengan tanggung jawab jelas. | Authentication, body limit, request ID, authorization sesuai kebijakan route. | Urutan middleware salah sehingga authz tak dieksekusi. |
| **Factory function** / Factory Method sederhana | Creational | Construction butuh validasi config, dependency, atau inisialisasi resource. | `NewService(deps)` atau `NewAmount(...) (Money,error)`; bukan hierarki creator Java. | Factory untuk setiap struct tanpa invariant. |
| **Command** | Behavioral | Job/intent perlu diwakili secara durable dan replayable. | Struct command + handler; perintah durable hanya selesai jika tersimpan di DB/outbox sesuai AC. | Mengganti event, command dan transaksi dengan satu generic message bag. |

### 14.2 Pola lain: pelajari tetapi jangan otomatis dipasang

| Pattern | Sikap Mamony sekarang | Saat relevan |
|---|---|---|
| Builder | On demand, sering cukup struct config + `New` validation. | Pembuatan request/config benar-benar banyak kombinasi dan ada validasi berurutan. |
| Abstract Factory | Tunda. | Beberapa *families* adapter yang benar-benar berubah bersama (mis. satu provider menyediakan client, parser, verifier). |
| Prototype | Tunda. | Cloning object mahal/kompleks yang muncul pada profiling, bukan menyalin simple Go struct. |
| Singleton | Hindari global mutable singleton; pilih instance per app dalam composition root. | Sumber daya satu instance per runtime dapat tetap dikelola eksplisit via constructor. |
| Bridge | Umumnya tak perlu hierarki berganda. | Dua dimensi variasi independen telah nyata. |
| Composite | On demand. | Policy tree atau rule group rekursif. |
| Flyweight | Tunda sampai profiling. | Data object sangat banyak dengan shared immutable state. |
| Proxy | On demand, review security. | Auth/cache/remote access proxy dengan semantik jelas; jangan bypass resource authorization. |
| Observer | Hati-hati. | Event *non-durable* lokal dapat pakai callback; event bisnis durable memakai outbox dan dedupe, bukan callback in-memory. |
| Iterator | Biasanya gunakan `for`/slice/DB cursor dengan lifecycle eksplisit. | Traversal collection kompleks dan stabil. |
| Mediator | Jarang; `app` composition bukan mediator domain universal. | Banyak komponen perlu koordinasi terpusat dan ownership-nya nyata. |
| Memento | Jarang. | Checkpoint state yang punya ukuran, retensi, dan semantics jelas; bukan replacement untuk audit ledger. |
| Template Method | Go tidak memakai inheritance kelas ala GoF; komposisi function/callback sering lebih jelas. | Algoritma bersama dengan beberapa step yang memang bervariasi. |
| Visitor | Jarang. | Banyak operasi untuk struktur node/AST yang stabil; bukan default fintech domain model. |

Daftar Refactoring.Guru Go memuat pola creational, structural, dan behavioral, tetapi katalog ini **diprioritaskan**, bukan port semua pola secara literal. `Repository`, `Unit of Work`, `Transactional Outbox`, `CQRS`, `Saga`, `Circuit Breaker`, dan `Bulkhead` dibahas sebagai **enterprise/system patterns**, bukan GoF yang harus meniru OO classes.

### 14.3 Pola sistem dan keputusan awal

| Pattern / approach | Default Mamony | Kapan dipakai | Evidence utama |
|---|---|---|---|
| **Repository** | Ya, adapter akses persistence per owner capability. | SQL perlu dipisahkan dari kebijakan use case. | PostgreSQL integration, grants, query/explain saat perlu. |
| **Unit of Work / Transaction Boundary** | Ya untuk beberapa write yang harus atomic dalam capability. | Use case butuh menyatukan write dan outbox. | Commit/rollback, cancellation, failure tests. |
| **Transactional Outbox** | Ya untuk event yang harus durable sesuai ADR. | DB write harus menghasilkan event tanpa dual-write tidak atomik. | Event hadir hanya sesudah commit; retry/dedupe/ordering. |
| **Idempotent Consumer / Inbox** | Ya saat ada at-least-once delivery. | Event dapat diproses ulang. | Duplicate delivery tidak menimbulkan efek ganda. |
| **CQRS** | **Lightweight separation** read/write jika kompleksitas menuntut; jangan membangun dua platform. | Model statement/read sangat berbeda dari post immutable. | Konsistensi dan lag read model terukur. |
| **Saga / Process Manager** | Kandidat bila multi-step external side effects tak bisa satu transaksi. | Payment orchestrates provider unknown, compensation yang bermakna domain. | Failure matrix, reconciliation, timeout and restart. |
| **Circuit Breaker** | Bukan baseline untuk simulator lokal. | Downstream remote berulang gagal dan ada fallback/fail fast policy yang benar. | Load test, failure simulation, retry budget. |
| **Bulkhead / Worker pool** | On demand dengan concurrency limit eksplisit. | Provider/job overload mengganggu request lain. | Queue depth, isolation, goroutine lifecycle. |
| **Cache-Aside** | Tunda untuk data saldo authoritative. | Hot read yang stale tolerance-nya eksplisit, key scope benar, invalidation dapat diuji. | Consistency/staleness contract, benchmark. |
| **Event Sourcing** | Jangan secara otomatis menyamakan immutable ledger dengan seluruh app event-sourced. | Ada kebutuhan replay event sebagai primary state semua aggregates. | Cost model dan migration policy. |
| **Hexagonal/Clean Architecture** | Gunakan *dependency boundary*, bukan template direktori rigid. | Perlu memisahkan core bisnis dari adapter, sudah terpenuhi oleh modular monolith. | Import checks, tests, owner graph. |

### 14.4 Template keputusan penggunaan pattern (maksimum satu paragraf di PR kecil)

```text
Problem: [perubahan nyata yang sulit ditangani tanpa pattern]
Simplest alternative: [switch/function/struct biasa]
Chosen pattern: [nama, bagaimana mengurangi coupling/risiko]
Cost: [jenis baru, indirection, tests, ownership]
Proof: [test yang menunjukkan behavior dan batas]
Revisit trigger: [kondisi yang membuat pilihan berubah]
```

Jika masalah sederhana selesai dengan fungsi Go kecil, jawab “no pattern needed” dan lanjutkan. **Menamai pattern tidak pernah menggantikan pembuktian invariant.**

## 15. System design untuk modular monolith finansial

**Referensi pembelajaran:** [roadmap.sh System Design](https://roadmap.sh/system-design), [roadmap.sh Backend](https://roadmap.sh/backend), [PostgreSQL isolation](https://www.postgresql.org/docs/current/transaction-iso.html), [Go concurrency cancellation](https://go.dev/blog/pipelines), [Google SRE Workbook](https://sre.google/workbook/). Roadmap bersifat **cakupan kompetensi**, bukan bukti bahwa Mamony memerlukan setiap komponen berskala besar.

### 15.1 System design dimulai dari requirement dan failure, bukan diagram teknologi

Untuk setiap boundary/fitur kelas M/L, jawab secara singkat:

1. **Functional:** siapa actor, outcome, command/query, state transition, sumber dana simulasi, owner capability?
2. **Non-functional:** correctness, latency, throughput, concurrency, availability, security, cost, operability, recovery. Angka yang belum diukur tulis **TBD** dan jangan dipresentasikan sebagai SLO aktual.
3. **Data:** source of truth, schema, constraints, indeks, migrasi, retensi, tenant scope, lifecycle.
4. **Consistency:** kapan state dianggap committed, siapa boleh membaca stale data, invariant apa yang tak boleh sementara dilanggar?
5. **Failure:** timeout, duplicate, out-of-order, partial commit, unknown outcome, retry, operator action, restore.
6. **Interfaces:** HTTP/event/query contracts, ownership, authz, versioning, error mapping.
7. **Operations:** metrics/log/trace, alert, diagnostics, rollback, manual recovery.
8. **Trade-off:** minimal 1 alternatif lebih sederhana; alasan menolak; cost dan revisiting trigger.

Gambar diagram konteks (actor → app → DB/simulator), container/proses (dua listener + jobs/relay), capability graph, lalu sequence untuk **happy path dan failure path**. Jangan membangun diagram deployment yang tidak sesuai runtime nyata.

### 15.2 Konsep system design yang *harus* dipahami, dengan kebijakan Mamony

| Konsep | Pertanyaan inti | Penerapan dan guardrail |
|---|---|---|
| **Latency vs throughput vs concurrency** | Apakah masalahnya respons lambat, jumlah kerja per detik, atau kontensi? | Tetapkan budget per dependency; ukur p50/p95/p99 dan antrean pada beban representatif; jangan hanya menilai average. |
| **Availability vs consistency vs durability** | Apa yang terjadi jika node mati, pesan terlambat, atau koneksi terputus? | Ledger financial state tidak boleh mengandalkan eventual cache sebagai truth. Recovery harus memulihkan jurnal dan verifier. |
| **ACID / isolation / locks** | Write mana harus atomic dan bisa race? | Transaction C01, unique constraint idempotency, lock ordering, deadlock/serialization retry sesuai DB invariant. |
| **CAP/PACELC** | Apa asumsi partition/replication? | Pelajari trade-off untuk sistem terdistribusi; **tidak** berarti memilih label CP/AP untuk satu PostgreSQL dan mengabaikan desain transaksi. |
| **Scaling vertical/horizontal** | Di mana bottleneck yang terukur? | Mulai satu proses + PostgreSQL; multi-instance hanya jika session, relay ownership, migrasi, pool budget, dan idempotency aman. |
| **Sync vs async** | Perlukah caller menunggu side effect? | Local DB atomic synchronous; event durable via outbox; provider unknown perlu state/rekonsiliasi, bukan goroutine fire-and-forget. |
| **Queues / delivery semantics** | Apakah terjadi duplikasi, order berubah, poison messages? | Assume at-least-once saat relay, dedupe consumer, retry bounded, dead-letter/replay policy jika kebutuhan nyata. |
| **Retry, timeout, cancellation** | Apakah retry aman? Apa bila hasil remote unknown? | `context.Context`; retry hanya error aman + idempotency, backoff/jitter dan keseluruhan deadline; jangan retry provider debit ambigu tanpa inquiry/rekonsiliasi. |
| **Backpressure / rate limit** | Bagaimana saat traffic melampaui kemampuan DB/provider? | Connection pool budget, bounded workers, overload response; prioritaskan queue yang jelas daripada spawn goroutine tak terbatas. |
| **Caching** | Bolehkah data stale dan bagaimana invalidation? | Jangan cache saldo authoritative untuk keputusan posting; cache konfigurasi statis/FX read bila freshness contract disetujui. |
| **Partitioning/sharding/replication** | Apakah data benar-benar melampaui satu DB? | Tunda; menyulitkan transaksi finansial dan reconciliation. Ukur dahulu. |
| **Observability / SLO** | Bagaimana tahu sistem sehat secara teknis dan bisnis? | Monitor request error/latency, queue/outbox lag, provider unknown, imbalance/reconciliation, retries, restore. |
| **RPO/RTO and restore** | Data apa boleh hilang? Berapa lama memulihkan? | Tentukan target lewat kebutuhan; uji restore ke instance terisolasi dengan verifier, jangan hanya cek backup file ada. |
| **Blast radius / graceful degradation** | Jika simulator FX/provider gagal, apa yang tetap aman? | Fail closed pada izin/ledger; endpoint status/read boleh tetap tersedia jika independen; hindari membuat aplikasi ‘sukses’ palsu. |

### 15.3 Failure-mode matrix wajib untuk alur uang (contoh awal)

| Titik gagal | Risiko | Respon desain yang diharapkan | Bukti |
|---|---|---|---|
| Request ganda bersamaan, key sama | Double charge/post | Unique constraint + satu hasil replay sesuai scope key | DB integration 2+ koneksi + same-key. |
| Key sama, payload berbeda | Reuse menyesatkan | Reject conflict tanpa perubahan journal | Fingerprint/unique test. |
| Proses mati sebelum commit | Sebagian data tersimpan | DB atomic rollback, outbox tidak muncul | Fault/transaction test. |
| Respons hilang setelah commit | Caller mengulang | Replay mengembalikan hasil committed tanpa memposting lagi | Simulasikan lost response. |
| Outbox relay crash sesudah publish | Event diterima dua kali | At-least-once + consumer dedupe | Redelivery test. |
| Provider timeout/unknown | Kesalahan repeat transaksi eksternal | Status `unknown/pending`, inquiry lalu reconcile | Simulator failure injection. |
| Dua posting berebut balance floor | Saldo negatif | Lock/constraint/serialization DB, transaction retried safely bila benar | Concurrency DB test terpisah dari Go race. |
| Tenant context hilang/reuse pool | Cross-tenant leak | Transaction-local RLS dan runtime role deny-by-default | Tests tenant A/B/empty + pool reuse. |
| Migrasi berhenti sebagian | Schema inconsistent | Versioned migration, transactional DDL saat mungkin, staged rollout | Fresh+upgrade test, recovery runbook. |
| Restore dari backup | Journal/account mismatch | Restore terisolasi dan recompute verifier | Restore drill dengan posted journal. |

Semua detail final mengikuti PRD/ADR/TD Mamony; tabel adalah *design review scenarios*, bukan klaim fitur Go sudah berjalan.

### 15.4 Backend fundamentals yang sering terlewat

| Area fundamental | Minimum yang AI harus memahami | Check atau exercise praktis |
|---|---|---|
| **HTTP/REST semantics** | Method safety/idempotence, status/error, headers, body limits, pagination, content type, caching rules. | `httptest`, OpenAPI contract tests, invalid JSON and duplicate requests. |
| **Networking** | DNS, TLS, timeouts, connection reuse/pooling, request cancellation, failure TCP vs application errors. | Simulated slow/hung provider, TLS errors dan shutdown. |
| **Authentication vs authorization** | Identitas caller ≠ izin pada resource; explicit principal dan tenant isolation. | Unauthorized access to resource milik tenant lain. |
| **SQL & relational modeling** | PK/FK/unique/check, transactions, isolation, MVCC, row locks, indexes, query plans. | `EXPLAIN`, concurrency test real Postgres, migrations. |
| **Data representation** | integer minor units, overflow, currency exponent, timezone UTC, encoding/normalization, IDs, stable ordering. | Edge-case parser, currency mismatch, timestamps near boundaries. |
| **API and event evolution** | Compatibility, schema version, deprecation, consumer-driven risk, backward/forward handling. | oasdiff/contract fixture, event replay with supported schema. |
| **OS/runtime** | Process signals, file descriptors, memory, CPU, goroutines, garbage collection, resource limits. | Graceful shutdown, leak/race tests, load and profiling when needed. |
| **Build & source control** | `go.mod`/`go.sum`, semver, reproducible build, Git discipline, bisectable commits. | Clone-clean `make check`, pinned toolchain, `go mod verify`. |
| **Logging and diagnostics** | Structured logs, correlation, metrics, traces, no PII/secret leakage. | Failure drill produces actionable logs without sensitive fields. |
| **Testing strategy** | Unit vs integration vs contract vs race/fuzz/load/recovery, deterministic fixtures. | Risk-specific evidence matrix; no blanket 100% coverage target. |

### 15.5 Template design ringkas (M/L)

```markdown
# [Feature / risk] Design Brief
Owner capability: Cxx | Risk: M/L | Status: Proposed
AC / source links: [...] | Scope / excluded: [...]
Actor → entry point → use case → module owner → DB/outbox/provider: ...
Invariant: ... | Consistency boundary: ...
Data schema/grants/migration: ...
API/event and compatibility: ...
Timeout/retry/idempotency/unknown outcome: ...
Failure matrix + recovery: ...
Observability and rollout: ...
Alternatives and reasons: ...
Verification commands/tests: ...
Open decisions requiring owner approval: ...
```

**YAGNI untuk system design:** load balancer, CDN, Redis, message broker, Kubernetes, multi-region, sharding, distributed transactions, custom circuit breakers, dan service mesh **bukan syarat foundation** Mamony. Mereka menjadi kandidat hanya setelah requirement dan pengukuran menunjukkan manfaat yang melebihi kompleksitas.

## 16. DevSecOps, security engineering, dan praktik operasional dasar

**Referensi:** [roadmap.sh DevSecOps](https://roadmap.sh/devsecops) sebagai peta kompetensi, [NIST SSDF SP 800-218 v1.1 (final)](https://csrc.nist.gov/pubs/sp/800/218/final) sebagai praktik secure SDLC, [OWASP ASVS 5.0.0](https://asvs.owasp.org/) sebagai kumpulan requirement verifikasi aplikasi, [GitHub supply-chain security](https://docs.github.com/en/code-security/concepts/supply-chain-security), dan [Go vulnerability management](https://go.dev/doc/security/vuln/). Ini memperluas SEC/OPS §3 dan CI §10; jangan buat pipeline kedua yang menduplikasi gate sama.

### 16.1 Kebijakan secure-by-design dan shift-left

| Tahap SDLC | Praktik minimum | Evidence | Kelas dampak |
|---|---|---|---|
| **Requirements** | Definisikan asset, actor, trust boundary, data klasifikasi, abuse case dan acceptance keamanan. | PRD/TD + security AC. | Semua M/L; L wajib mendalam. |
| **Design** | Threat modeling ringan: spoofing, tampering, repudiation, information disclosure, DoS, privilege escalation; bedakan kontrol prevent/detect/recover. | Data-flow + threat-to-test mapping. | L auth/ledger/external wajib. |
| **Code** | Parameterized SQL, output encoding sesuai media, typed config, validations, context deadlines, secure secrets, no hardcoded credentials. | Lint/source review, unit/contract tests. | Semua. |
| **Dependencies** | Update/inventory direct dan transitive deps, `go mod verify`, vulnerability scan dengan triage exploitability. | SBOM atau dependency list, govulncheck, exceptions beralasan. | Semua perubahan dependency. |
| **CI** | Least-privilege workflows, SHA pin tindakan, secret scanning sesuai availability, artifact integrity. | CI logs, permission review, action diff. | Semua. |
| **Build/deploy** | Reproducible-ish pinned toolchain, non-root image, minimal runtime, env isolation, immutable image digest. | Image scan/signature/provenance jika tahapnya siap, deploy record. | Tahap staging/prod simulator. |
| **Runtime** | TLS, access control, observability safe fields, rate limiting, resource/time limits, backup/restore. | Security integration, smoke, runbook. | Sesuai exposure. |
| **Respond** | Patch/rollback, revoke credentials, incident timeline, postmortem, dependency re-scan terjadwal. | Drill/log incident, restore verification. | Release/operation. |

**Kebijakan Penting:** scanner **tidak** bisa membuktikan otorisasi object-level, kesesuaian amount, atau ketiadaan double posting. Itu perlu policy analysis dan domain-specific tests. Jangan mengklaim “ASVS compliant” hanya karena memakai checklist; pilih control IDs versi yang dipin dan buktikan cakupannya.

### 16.2 Threat review untuk Mamony

| Asset/boundary | Contoh ancaman | Pencegahan utama | Uji negatif |
|---|---|---|---|
| Public API → payment | Caller memanipulasi amount/reference; replay key | Authn/Authz, validation, scoped idempotency, rate budget | Invalid amount, key reuse with different body, unauthorized resource. |
| Private listener → operator | Privilege escalation atau maker-checker bypass | Listener network policy + independent authorization + approval bound to exact payload | Approver sama dengan maker; payload changed after approval. |
| Application → PostgreSQL | SQL injection, wrong tenant, excessive grants | Parameterized SQL, runtime role, transaction-local RLS, least privilege | RLS empty context, tenant B read, forbidden schema writes. |
| Ledger posting | Rounding/overflow, double post, race | Integer minor units, checked arithmetic, DB uniqueness/atomic transaction | Max amounts, concurrency, cancellation, rollback. |
| Outbox/consumer | Replay, reordering, poison message | Versioned envelope, dedupe, ordering policy, bounded retries | Duplicate/out-of-order event, retry exhaustion. |
| Provider simulator | Unknown outcome, forged webhook, unbounded retry | Authenticate/scope callback if endpoint exposed; idempotent updates, inquiry/reconcile | Forged callback, timeout after remote effect, duplicate callback. |
| Build agent/CI → repository | Malicious skill/plugin, poisoned dependency, secret exfiltration | Pin tools, review third-party scripts, restricted tokens, safe secret handling | Validate permissions and untrusted-input handling. |

Sistem ini tetap simulator sesuai ADR-0008. Jangan membuat koneksi ke provider nyata atau menyimpan data bank/rekening riil untuk menguji threat scenario.

### 16.3 Minimal quality gates: jalankan *lapisan berbeda*, bukan semuanya sekaligus

| Gate | Otomatis / manual | Scope | Aturan |
|---|---|---|---|
| `gofmt`, `go vet`, `golangci-lint` | Otomatis | Style, bug patterns, sebagian security | Required PR baseline, false positive ditriage dengan alasan. |
| `go test`, race, DB integration | Otomatis | Invariant semantik, goroutine race, SQL concurrency | Race dan DB concurrency **bukti berbeda**. |
| OpenAPI/contract + dependency graph | Otomatis | API drift, layer/owner import | Wajib jika boundary ada; tidak menyatakan contract valid hanya dari build. |
| `govulncheck`, dependency review | Otomatis + triage manusia | Advisory & reachable code | Jadwal scan, prosedur exception dan upgrade. |
| Secret scan, CI permissions, image security | Otomatis + review | Repository/CI/runtime packaging | Aktifkan saat kemampuan dan tahap relevan; jangan klaim tool native tersedia pada private plan tanpa verifikasi. |
| Domain + threat model review | Manusia dibantu AI | Authz, ledger, recovery, error ambiguity | Wajib M/L sesuai risiko; AC→test evidence. |
| Restore/DR, load, chaos/fault injection | Test operasional | Recovery dan behavior saat dependency gagal | Bertahap sesuai blast radius, bukan tiap edit typo. |

### 16.4 Basics yang sering terlewat tetapi mahal saat gagal

- **Time semantics:** pakai UTC untuk storage, timezone hanya pada display; inject clock pada test; definisikan timestamp dari event vs processing time, dan retention berdasarkan timezone yang jelas.
- **Numeric semantics:** minor units bertipe; jangan pakai binary floating-point untuk posting; `int64` tetap perlu overflow checks dan jelas currency exponent; FX rounding mengikuti ADR bernama.
- **Error taxonomy:** klasifikasikan validation, authorization, not found, conflict, retryable, and outcome unknown; jangan menampilkan internal stack/SQL dalam response publik.
- **Resource hygiene:** tutup body/rows, periksa `rows.Err()`, defer rollback dengan lifecycle yang jelas, tetapkan HTTP/server/client timeout, pool budget, shutdown goroutine/worker.
- **Determinism:** tests tidak bergantung pada wall-clock, locale, random unseeded, provider internet, atau race scheduling yang kebetulan.
- **Data ownership:** tidak ada cross-schema SQL shortcut; query lintas capability melalui façade/port/read model yang disetujui.
- **Schema evolution:** migrasi yang sudah dirilis immutable, test fresh/upgrade, staged backfill, index builds dan lock budget dijelaskan.
- **Dependency hygiene:** minimalkan library baru dan risiko transitive; review lisensi, maintenance, ukuran binary/build, dan supply chain bila relevan.
- **Operational documentation:** quickstart dari clone bersih, env example aman, make targets, runbooks yang bisa diuji dan owner keputusan jelas.
- **Developer ergonomics:** error messages actionable, log ringkas, test fixtures kecil, `gopls`/symbol search sebelum membaca seluruh repository.

## 17. Implementasi untuk AI coding: prinsip yang benar-benar terpakai

### 17.1 Apa yang *otomatis* dan apa yang memerlukan engineering judgment

| Rule | Bisa CI? | Cara menegakkan |
|---|---|---|
| Formatting, lint, vulnerable dependency | **Ya** | §10 toolchain yang dipin. |
| Import boundary dan forbidden package | **Ya** | Import graph checker + tests untuk positive/negative fixtures. |
| HTTP/event contract & schema compatibility | **Sebagian besar** | OpenAPI/event validation, contract tests. |
| Ledger invariants/idempotency/RLS | **Dengan executable tests** | DB integrations, concurrent fault tests. |
| Refactoring behavior-preserving | **Sebagian** | Regression/contract tests + diff review. |
| SRP, KISS, DRY, YAGNI, correct pattern | **Tidak sepenuhnya** | Review perubahan nyata, alasan pembentukan abstraksi, alternatif minimal. |
| Proper trade-off / business semantics | **Keputusan manusia** | Requirement/ADR owner, failure matrix, risk evidence. |

### 17.2 Sisipan singkat yang disarankan untuk `AGENTS.md` (bukan menyalin seluruh bab)

```markdown
## Engineering design rules
- Correctness, financial invariants, authorization, and recovery outrank brevity.
- Follow KISS/YAGNI: implement only approved behavior; avoid speculative frameworks,
  repositories, interfaces, factories, caches, brokers, and generic abstractions.
- Apply SOLID as Go dependency boundaries, not class hierarchies. Prefer small
  consumer-owned interfaces when a genuine seam is needed.
- Preserve module ownership; domain/usecase may not import concrete HTTP/DB adapters.
- Before adding a design pattern or dependency, state the problem, simplest alternative,
  benefit, cost, and how to verify the decision.
- Refactors must preserve observable behavior; run targeted regression/contract checks.
- For finance/auth/data changes, enumerate failure, duplicate, concurrency, and recovery cases.
- Report actual checks as PASS / FAIL / NOT RUN. Do not infer proof from code inspection.
- Read relevant headings in docs/standards/engineering-principles.md on demand;
  never paste the full handbook into every task context.
```

### 17.3 Skill routing: satu reviewer terarah, bukan instruksi global panjang

Tambahkan **`mamony-engineering-review`** sebagai skill *on demand* yang aktif ketika ada architecture/refactoring/pattern decision, termasuk saat AI berencana menambah dependency/package. Skill ini memeriksa **problem → alternative → chosen design → evidence → risk**, lalu memanggil `mamony-boundary-review` atau `mamony-ledger-review` hanya jika modul/risiko benar-benar menyentuhnya. Jangan muat seluruh §12–§16 dalam konteks setiap request.

### 17.4 Checklist review singkat per kelas perubahan

| Kelas | Basic design review | Tambahan evidence |
|---|---|---|
| **S** | Scope kecil, naming jelas, tidak menambah abstractions tanpa kebutuhan, targeted tests, lint. | Jika behavior unaffected: tunjukkan test yang masih relevan. |
| **M** | AC, module owner, SOLID/SoC, alternatives, HTTP/DB contract dan failure path. | Integration untuk boundary yang berubah; migration/API drift jika ada. |
| **L** | Semua M + source of truth, trust boundaries, recovery, idempotency, transaction/lock, data retention, roll-forward. | Real DB concurrency, fault injection, security review, restore/runbook/reconciliation bila relevan. |

### 17.5 Decision record mini untuk mencegah over-engineering

Sebelum membuat dependency, package, pattern, interface, cache, broker atau worker baru, AI harus mengisi (maksimum 5 baris kecuali L):

```text
Need/AC: ...
Simplest valid design: ...
Why it fails / where it creates risk: ...
Chosen change + what it costs: ...
Proof and revisit trigger: ...
```

Jika **simplest valid design sudah memenuhi AC dan kontrol risiko**, gunakan itu. Jangan memperluas desain hanya supaya seluruh roadmap/GoF catalog “terpakai”.

### 17.6 Peta sumber dan batas interpretasi

| Sumber | Apa yang dipakai | Batas penerapan |
|---|---|---|
| [Refactoring.Guru — Design Patterns](https://refactoring.guru/design-patterns), [Go examples](https://refactoring.guru/design-patterns/go) | Vocabulary, intent dan trade-offs pola. | Pola GoF bukan keharusan. Contoh OO diadaptasi, bukan dicopy mentah. |
| [Refactoring.Guru — Refactoring](https://refactoring.guru/refactoring), [Code Smells](https://refactoring.guru/refactoring/smells) | Prosedur perubahan kecil, tes, smell heuristics. | Smell tidak otomatis bug; perubahan behavior butuh spec. |
| [roadmap.sh — System Design](https://roadmap.sh/system-design) | Peta topik skala, keandalan, konsistensi, infrastruktur. | Bukan requirement produk atau rekomendasi memasang seluruh komponen. |
| [roadmap.sh — Backend](https://roadmap.sh/backend) | Kurikulum HTTP, API, server, DB, security, testing. | Prioritas dipilih dari scope Mamony. |
| [roadmap.sh — DevSecOps](https://roadmap.sh/devsecops) | Lensa SDLC + keamanan dari kode sampai operasi. | Untuk kontrol konkret, gunakan NIST/OWASP dan bukti CI. |
| [Go project — Effective Go](https://go.dev/doc/effective_go), [Review Comments](https://go.dev/wiki/CodeReviewComments) | Idiom, naming, error, interface dan packaging. | Tidak semua saran lama mencakup semua fitur Go terbaru; review pada versi toolchain yang dipin. |
| [Martin Fowler — YAGNI](https://martinfowler.com/bliki/Yagni.html) | Menunda capability spekulatif sambil mempermudah perubahan. | Bukan izin untuk menghilangkan requirement saat ini atau kontrol integritas. |
| [OWASP ASVS 5.0.0](https://asvs.owasp.org/), [NIST SSDF SP 800-218 v1.1 final](https://csrc.nist.gov/pubs/sp/800/218/final) | Requirement security verification dan secure SDLC. | Pilih control IDs/versi spesifik; jangan membuat klaim compliance tanpa audit. |

**Ringkasan penerapan:** prinsip core §12 berlaku secara konseptual untuk semua tugas, tetapi berkas panjang §12–§16 dibaca **selektif** menurut isu. CI mengunci hal yang bisa diuji; reviewer manusia memutus trade-off. Untuk Mamony Go, mempertahankan *correctness under failure* lebih bernilai daripada memperlihatkan sebanyak mungkin design pattern.
