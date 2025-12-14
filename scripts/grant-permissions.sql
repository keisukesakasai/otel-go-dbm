-- advent-userに必要な権限を付与するスクリプト
-- 実行方法: psql -U postgres -d testdb -f scripts/grant-permissions.sql

-- 1. スキーマの使用権限を付与
GRANT USAGE ON SCHEMA public TO "advent-user";

-- 2. 既存テーブルへの権限を付与
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO "advent-user";

-- 3. シーケンス（AUTO_INCREMENT相当）への権限を付与
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO "advent-user";

-- 4. 将来作成されるテーブルへの権限を自動付与（デフォルト権限の設定）
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "advent-user";
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO "advent-user";

-- 5. テーブル作成権限を付与（マイグレーション用）
GRANT CREATE ON SCHEMA public TO "advent-user";

-- 6. 確認: 現在の権限を表示
SELECT 
    grantee,
    table_schema,
    table_name,
    privilege_type
FROM information_schema.table_privileges
WHERE grantee = 'advent-user'
ORDER BY table_schema, table_name, privilege_type;

-- 7. 確認: スキーマ権限を表示
SELECT 
    nspname as schema_name,
    nspacl as privileges
FROM pg_namespace
WHERE nspname = 'public';

