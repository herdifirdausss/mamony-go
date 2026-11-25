# Curated reference release

FND-02 partial; bounded synthetic experiment accepted in [#92](https://github.com/herdifirdausss/mamony-go/issues/92). No runtime wiring, capability enablement, Money abstraction, database or provider configuration.

| Field | Value |
|---|---|
| Mamony release | fnd02d-2026-11-25.1 |
| Prior release | fnd02a-2026-11-25.1 |
| Currency source | [SIX ISO 4217 List One XML](https://www.six-group.com/dam/download/financial-information/data-center/iso-currrency/lists/list-one.xml) |
| Source acquisition | Owner-supplied file inspected 2026-11-25; upstream retrieval unavailable |
| Publication attribute | Pblshd=2026-09-17 (not effective date or latest-version assertion) |
| Exact uploaded bytes | 47491 |
| Uploaded XML SHA-256 | 33139b438657d1cee116ba737807ea71d19d6de4b90f799a09c56f0cc6a1b0ff |
| Country source | [UN M49 overview](https://unstats.un.org/unsd/methodology/m49/overview/) ISO-alpha2 column, inspected 2026-11-25; web rows only, no raw download digest |
| Prior FND-02a curated data SHA-256 | 7cd0ffb8aa27b279931d861ca6dea52256272c512a47b5e0013ca1049e306e3c |
| Canonical FND-02d data SHA-256 | 9c335b6f3362735bde0523de78153e8525c56d0d8ad04e4133155ca6c01cbcf1 |

[ISO code usage](https://www.iso.org/iso-3166-country-codes.html) allows free use of country and currency codes. [SIX maintenance page](https://www.six-group.com/en/products-services/financial-information/market-reference-data/data-standards.html) identifies List One as the current list; full file redistribution permission remains unconfirmed. Commit only selected factual codes/exponents in a new minimal representation, not XML/XLS, names or a copy of the complete database. The raw file digest identifies supplied bytes, not publisher authentication or independent confirmation of currency recency.

## Reproduce selection

Parse XML without external entities. Read ISO_4217 publication attribute and CcyTbl/CcyNtry rows. Select IDR, JPY, KWD, SGD, USD by Ccy. For every selected code require at least one row, no IsFund=true and all CcyMnrUnts and CcyNbr values consistent. Observed 280 total rows, one row for each selected code except USD (19 consistent rows). Retain only Ccy and numeric CcyMnrUnts; no numeric code or country-to-currency mapping in catalog. Verify the separate alpha-2 values ID, JP, KW, SG, US against UN M49, not currency prefixes. Original FND-02a bytes used C,<code>,<exponent> and N,<code>. For FND-02d, keep the same factual selections and explicitly allocate local definition and monetary-unit IDs per [#100](https://github.com/herdifirdausss/mamony-go/issues/100). Canonical bytes are ASCII CSV-like lines C,<code>,<exponent>,<definition-ID>,<unit-ID> and N,<code>, sorted bytewise, each terminated by LF, including last. Compute SHA-256 of exactly the Go constant bytes. Do not confuse the source and curated hashes. Recheck official source/terms and compare exact definitions when updating release.

## Identity and limitations

Load validates the checksum and canonical shape before returning a private read-only catalog. Lookups fail closed; unsupported means absent from this curated release, not invalid globally. Version now returns the stored local definition ID (mamony-currency-v2:<code>:<allocation>); UnitID returns the stored monetary-unit ID (mamony-unit-v1:<code>:<allocation>). Allocation 1 per curated code is a local owner decision, not ISO identity, exponent, release, effective date or historical inception. Current and exact pinned lookup read the same immutable dataset. The legacy v1 identity has no alias. Definition ID binds the immutable code/unit/exponent tuple; changes to exponent or monetary unit cannot reuse a definition ID. Independent golden tuple tests freeze this initial baseline; parser validation alone cannot authenticate past history. Cross-release comparisons are test evidence, not durable append-only enforcement. No real history is imported; source publication and local release are not effective dates. FND-02c remains an isolated fixture model. Actual lifecycle, corrections, retirement and real effective-date provenance require separate review. Checksums detect drift, not a writer changing data and checksum together. Country membership is not eligibility, nationality, residency or compliance evidence. No provider exponent, enabled currencies or public wire format in this package. The actual source file is not redistributed. Keep FND-02 integration and broader coverage outstanding.
