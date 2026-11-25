-- Bounded synthetic Ledger storage. No account seeds or runtime grants.
CREATE TABLE mamony_ledger.migration_fingerprints (
    version bigint PRIMARY KEY CHECK (version > 0),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE mamony_ledger.currency_definitions (
    definition_id text COLLATE "C" PRIMARY KEY,
    code text COLLATE "C" NOT NULL,
    exponent smallint NOT NULL,
    monetary_unit_id text COLLATE "C" NOT NULL,
    source_release_id text COLLATE "C" NOT NULL,
    source_catalog_sha256 text COLLATE "C" NOT NULL,
    CONSTRAINT currency_frozen_usd CHECK (
        definition_id = 'mamony-currency-v2:USD:1' AND code = 'USD' AND exponent = 2 AND
        monetary_unit_id = 'mamony-unit-v1:USD:1' AND source_release_id = 'fnd02d-2026-11-25.1' AND
        source_catalog_sha256 = '9c335b6f3362735bde0523de78153e8525c56d0d8ad04e4133155ca6c01cbcf1'
    )
);
CREATE TABLE mamony_ledger.books (
    book_id text COLLATE "C" PRIMARY KEY,
    definition_id text COLLATE "C" NOT NULL UNIQUE REFERENCES mamony_ledger.currency_definitions(definition_id),
    CONSTRAINT book_id_bytes CHECK (octet_length(book_id) BETWEEN 1 AND 128)
);
CREATE TABLE mamony_ledger.account_types (
    type_code text COLLATE "C" PRIMARY KEY,
    accounting_class text COLLATE "C" NOT NULL,
    normal_side text COLLATE "C" NOT NULL,
    floor_minor bigint NOT NULL,
    CONSTRAINT account_type_mapping CHECK (floor_minor = 0 AND (
        (type_code = 'COLLECTION_CASH' AND accounting_class = 'ASSET' AND normal_side = 'D') OR
        (type_code = 'CONTRIBUTED_CAPITAL' AND accounting_class = 'EQUITY' AND normal_side = 'C') OR
        (type_code = 'CUSTOMER_AVAILABLE' AND accounting_class = 'LIABILITY' AND normal_side = 'C')
    ))
);
CREATE TABLE mamony_ledger.accounts (
    account_id text COLLATE "C" PRIMARY KEY,
    book_id text COLLATE "C" NOT NULL REFERENCES mamony_ledger.books(book_id),
    type_code text COLLATE "C" NOT NULL REFERENCES mamony_ledger.account_types(type_code),
    owner_ref text COLLATE "C" NOT NULL,
    status text COLLATE "C" NOT NULL,
    balance_minor bigint NOT NULL,
    account_seq bigint NOT NULL,
    CONSTRAINT account_id_bytes CHECK (octet_length(account_id) BETWEEN 1 AND 128),
    CONSTRAINT account_owner_bytes CHECK (octet_length(owner_ref) BETWEEN 1 AND 128),
    CONSTRAINT account_k2a_status CHECK (status = 'ACTIVE'),
    CONSTRAINT account_k2a_balance CHECK (balance_minor = 0),
    CONSTRAINT account_k2a_sequence CHECK (account_seq = 0)
);
REVOKE ALL ON ALL TABLES IN SCHEMA mamony_ledger FROM PUBLIC;

-- mamony:integrity:v1
INSERT INTO mamony_ledger.migration_fingerprints (version, sha256) VALUES (1, '7c9e80d05f2fafca532ff1087e96278e4401cc3d45bfd3d5f1e82693cb79734b');
