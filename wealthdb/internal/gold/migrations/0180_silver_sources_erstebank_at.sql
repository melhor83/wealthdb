-- Add 'erstebank_at' to the silver_kind whitelist so the erstebank_at
-- adapter can register a silver source. erstebank_at is the George (Erste
-- Bank und Sparkassen, Austria) collector: it emits a `cash` account per
-- checking account and a `card` account per credit card, their roster and
-- daily closing balances (a card's as negative cash), and both ledgers
-- (collectors/erstebank_at, internal/silver/erstebank_at).
--
-- DuckDB can't widen a CHECK constraint in place — the rename-recreate
-- workaround of 0125, carrying every column.

ALTER TABLE silver_sources RENAME TO silver_sources_pre_0180;

CREATE TABLE silver_sources (
    silver_source_id    TEXT    PRIMARY KEY,
    silver_kind         TEXT    NOT NULL CHECK (silver_kind IN (
        'schwab', 'ubs', 'swissquote', 'fidelity',
        'relevate', 'viac', 'cointracking', 'carta', 'angellist',
        'equityzen', 'manual', 'fred', 'chase', 'firstcitizens',
        'raiffeisen_at', 'amex', 'synthetic', 'plaid', 'refdata',
        'curves', 'erstebank_at'
    )),
    silver_path         TEXT    NOT NULL,
    high_watermark      BIGINT  NOT NULL,
    first_loaded_at     BIGINT  NOT NULL,
    last_loaded_at      BIGINT  NOT NULL,
    fx_priority         INTEGER,
    observed_snapshots_through    BIGINT DEFAULT -1,
    observed_transactions_through BIGINT DEFAULT -1
);

INSERT INTO silver_sources
SELECT silver_source_id, silver_kind, silver_path,
       high_watermark, first_loaded_at, last_loaded_at, fx_priority,
       observed_snapshots_through, observed_transactions_through
  FROM silver_sources_pre_0180;

DROP TABLE silver_sources_pre_0180;

INSERT INTO schema_meta (gold_schema_version, applied_at)
    VALUES (173, CAST(epoch(now()) AS BIGINT));
