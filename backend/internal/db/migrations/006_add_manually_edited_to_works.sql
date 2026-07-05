-- 手動編集(title/circle)を CSV 再インポートより優先させるためのフラグ(issue #64 案 A)。
-- PATCH で title/circle を変更した作品はこのフラグを立て、CSV インポータは
-- フラグが立っている作品の title/circle を上書きしない。
ALTER TABLE works ADD COLUMN manually_edited INTEGER NOT NULL DEFAULT 0;
