-- works テーブルの主要クエリ用インデックスを追加する。
--
-- /api/works /api/tags /api/circles /api/history は全て hidden を WHERE に持つため、
-- (hidden, ソート/フィルタ列) の複合インデックスにしておけば、
-- 「可視作品の中でソート/絞り込み」が単一インデックスで処理される。

CREATE INDEX IF NOT EXISTS idx_works_hidden_purchase ON works(hidden, purchase_date);
CREATE INDEX IF NOT EXISTS idx_works_hidden_circle   ON works(hidden, circle);
CREATE INDEX IF NOT EXISTS idx_works_hidden_title    ON works(hidden, title);
CREATE INDEX IF NOT EXISTS idx_works_hidden_created  ON works(hidden, created_at);
CREATE INDEX IF NOT EXISTS idx_works_hidden_series   ON works(hidden, series_name);
