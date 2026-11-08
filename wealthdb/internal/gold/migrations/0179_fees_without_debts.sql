-- ============================================================
-- gold schema, migration 0179 — a portfolio's fee base leaves out its
-- debts (docs/SPENDING.md §7).
--
--   * web_fees: the 'value' line of a portfolio no longer holds its
--     mortgages and cards. Its fee rate is fees over its assets' mean
--     value, the base its return reads, as `wealthdb spending fees`
--     reads it.
--
-- OR REPLACE keeps this replayable (see gold.Migrate's REPLAY note).
-- ============================================================

-- web_fees: what the fee tiles read, at day grain, so a picked window
-- cuts it where it cuts the other Spending tiles.
--   * line 'fee': a spending line of primary BANK_FEES, its value turned
--     to a cost. A refund is negative. fee_group is its detailed group.
--   * line 'value': an account's value on a day. A portfolio's mortgages
--     and cards are left out, as its return leaves them out; an account
--     that is a card or a mortgage keeps its own balance.
--   * line 'return': the entity's return for the month that ends on the
--     day, in `currency`, repeated on each of its accounts so an account
--     picker keeps it.
-- The entity is the account's portfolio, else the account.
CREATE OR REPLACE VIEW web_fees AS
    WITH acct AS (
        SELECT a.silver_source_id, a.account_external_id, a.account_kind,
               account_label(a.display_name, a.account_external_id,
                             a.silver_source_id, a.account_kind) AS account_label,
               account_display_name(a.display_name, a.account_external_id) AS display_name,
               NULLIF(a.portfolio_external_id, '') AS portfolio
          FROM accounts a),
    entity AS (
        SELECT c.*,
               to_json([c.silver_source_id, COALESCE(c.portfolio, c.account_external_id)])::VARCHAR AS entity_key,
               CASE WHEN c.portfolio IS NOT NULL THEN 'portfolio' ELSE 'account' END AS entity_kind,
               COALESCE(p.display_name, c.portfolio, c.display_name) AS entity
          FROM acct c
          LEFT JOIN portfolios p ON p.silver_source_id = c.silver_source_id
               AND p.portfolio_external_id = c.portfolio)
    SELECT epoch_ms(t.occurred_at * 1000) AS day, 'fee' AS line,
           e.silver_source_id, e.account_label, e.entity_key, e.entity_kind, e.entity,
           fee_group(t.spend_detailed) AS fee_group, CAST(NULL AS TEXT) AS currency,
           -t.value_usd AS value_usd, -t.value_chf AS value_chf,
           -t.value_eur AS value_eur, -t.value_gbp AS value_gbp,
           CAST(NULL AS DOUBLE) AS twr
      FROM serving_spending_transactions t
      JOIN entity e ON e.silver_source_id = t.silver_source_id
       AND e.account_external_id = t.account_external_id
     WHERE t.spend_primary = 'BANK_FEES'
    UNION ALL
    SELECT epoch_ms(h.as_of_day * 1000), 'value',
           e.silver_source_id, e.account_label, e.entity_key, e.entity_kind, e.entity,
           NULL, NULL,
           h.total_value_usd, h.total_value_chf, h.total_value_eur, h.total_value_gbp,
           NULL
      FROM serving_accounts_history h
      JOIN entity e ON e.silver_source_id = h.silver_source_id
       AND e.account_external_id = h.account_external_id
     WHERE e.entity_kind = 'account' OR e.account_kind NOT IN ('mortgage', 'card')
    UNION ALL
    SELECT epoch_ms(r.end_day * 1000), 'return',
           e.silver_source_id, e.account_label, e.entity_key, e.entity_kind, e.entity,
           NULL, r.currency, NULL, NULL, NULL, NULL, r.twr
      FROM report_returns r
      JOIN entity e ON e.silver_source_id = r.silver_source_id
       AND ((r.grain = 'portfolios' AND r.entity_id = e.portfolio)
         OR (r.grain = 'accounts' AND e.portfolio IS NULL AND r.entity_id = e.account_external_id))
     WHERE r.granularity = 'monthly' AND r.window_from_year = 0 AND NOT r.is_summary;

INSERT INTO schema_meta (gold_schema_version, applied_at)
    VALUES (179, CAST(epoch(now()) AS BIGINT));
