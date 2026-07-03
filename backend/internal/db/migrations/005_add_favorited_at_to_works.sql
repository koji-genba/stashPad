-- お気に入り機能(issue #72)。NULL = 非お気に入り、日時が入っていればお気に入り。
-- 日時にしておくことで「お気に入り登録順」ソートがそのまま手に入る。
ALTER TABLE works ADD COLUMN favorited_at TEXT;
