-- ============================================================
-- gold schema, migration 0178 — mark age sums in any order
-- (docs/DESIGN.md §4.7).
--
--   * report_marks: each line's value is a decimal before the sums, in
--     USD and in the output currency. A float sum rounds in the order
--     it adds, and DuckDB adds in parallel in no fixed order, so a total
--     moved in its last digit between runs. A decimal sum is exact, so
--     the order no longer matters. Each line rounds once, to 1e-10.
--
-- OR REPLACE keeps this replayable (see gold.Migrate's REPLAY note).
-- report_marks_multi and serving_marks read the macro by name and keep
-- their columns.
-- ============================================================

-- report_marks: per month end, the value of the positions on the daily
-- history that day, and the part marked within five days. A line's age
-- is the month end less coalesce(valued_at, snapshot_at), in days.
-- Liabilities and cash are out, and so is a line of no value. A line is
-- valued as the daily history values it, at its snapshot's rate.
-- marked_pct weighs by USD value whatever p_ccy is, so the figure has
-- one definition. A month end with no line has a NULL marked_pct and
-- says so in quality, and a line with no rate to USD or p_ccy leaves the
-- sums and is counted there.
CREATE OR REPLACE MACRO report_marks(p_ccy) AS TABLE (
    WITH lines AS (
        SELECT p.instant // 86400 AS day, p.snapshot_at, p.valued_at, p.currency, p.market_value
          FROM positions_at((SELECT list(day * 86400 + 86399) FROM mark_month_ends())) p
          JOIN accounts a ON a.silver_source_id = p.silver_source_id
           AND a.account_external_id = p.account_external_id
         WHERE p.market_value > 0 AND p.asset_class <> 'cash'
           AND a.account_kind NOT IN ('mortgage', 'card')),
    valued AS (
        SELECT l.day,
               l.day - COALESCE(l.valued_at, l.snapshot_at) // 86400 <= 5 AS marked,
               CAST(fx_reporting_value(l.market_value, l.currency, 'USD', fxl, fxb) AS DECIMAL(38,10)) AS usd,
               CAST(COALESCE(CASE WHEN l.currency = p_ccy THEN l.market_value::DOUBLE END,
                             l.market_value::DOUBLE * o0.rate, l.market_value::DOUBLE * o1.rate * o2.rate,
                             l.market_value::DOUBLE * o3.rate * o4.rate) AS DECIMAL(38,10)) AS out_val
          FROM lines l
          ASOF LEFT JOIN fx_reporting_legs fxl    ON fxl.from_ccy = l.currency AND fxl.day <= (l.snapshot_at // 86400)
          ASOF LEFT JOIN fx_reporting_bridges fxb ON fxb.day <= (l.snapshot_at // 86400)
          ASOF LEFT JOIN fx_daily o0 ON o0.from_ccy = l.currency AND o0.to_ccy = p_ccy AND o0.day <= (l.snapshot_at // 86400)
          ASOF LEFT JOIN fx_daily o1 ON o1.from_ccy = l.currency AND o1.to_ccy = 'CHF' AND o1.day <= (l.snapshot_at // 86400)
          ASOF LEFT JOIN fx_daily o2 ON o2.from_ccy = 'CHF' AND o2.to_ccy = p_ccy AND o2.day <= (l.snapshot_at // 86400)
          ASOF LEFT JOIN fx_daily o3 ON o3.from_ccy = l.currency AND o3.to_ccy = 'USD' AND o3.day <= (l.snapshot_at // 86400)
          ASOF LEFT JOIN fx_daily o4 ON o4.from_ccy = 'USD' AND o4.to_ccy = p_ccy AND o4.day <= (l.snapshot_at // 86400))
    SELECT e.day * 86400 AS month_end,
           CAST(100 * COALESCE(SUM(v.usd) FILTER (WHERE v.marked), 0) / NULLIF(SUM(v.usd), 0) AS DOUBLE) AS marked_pct,
           CAST(COALESCE(SUM(v.out_val) FILTER (WHERE v.marked), 0) AS DECIMAL(28,4)) AS value_marked,
           CAST(COALESCE(SUM(v.out_val), 0) AS DECIMAL(28,4)) AS value_total,
           CASE WHEN COUNT(v.day) = 0 THEN 'no_positions'
                WHEN COUNT(*) FILTER (WHERE v.usd IS NULL OR v.out_val IS NULL) > 0
                    THEN 'unpriced_lines=' || COUNT(*) FILTER (WHERE v.usd IS NULL OR v.out_val IS NULL)
                ELSE '' END AS quality
      FROM mark_month_ends() e
      LEFT JOIN valued v ON v.day = e.day
     GROUP BY 1
     ORDER BY 1
);

INSERT INTO schema_meta (gold_schema_version, applied_at)
    VALUES (178, CAST(epoch(now()) AS BIGINT));
