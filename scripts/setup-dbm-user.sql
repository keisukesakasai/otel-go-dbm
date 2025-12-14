-- Datadog Database Monitoring用ユーザー作成スクリプト
-- CloudSQLに接続して実行してください

-- Datadog用ユーザーを作成（既に存在する場合はスキップ）
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_user WHERE usename = 'datadog') THEN
        CREATE USER datadog WITH PASSWORD 'your-secure-password-here';
    END IF;
END
$$;

-- 必要な権限を付与
GRANT SELECT ON pg_stat_database TO datadog;
GRANT SELECT ON pg_stat_activity TO datadog;
GRANT SELECT ON pg_stat_statements TO datadog;
GRANT SELECT ON pg_stat_user_tables TO datadog;
GRANT SELECT ON pg_stat_user_indexes TO datadog;
GRANT SELECT ON pg_statio_user_tables TO datadog;
GRANT SELECT ON pg_statio_user_indexes TO datadog;

-- pg_monitorロールを付与（PostgreSQL 10+）
GRANT pg_monitor TO datadog;

-- postgresデータベースでpg_stat_statements拡張機能を有効化
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- testdbデータベースに接続して設定
\c testdb

-- datadogスキーマを作成
CREATE SCHEMA IF NOT EXISTS datadog;
GRANT USAGE ON SCHEMA datadog TO datadog;
GRANT USAGE ON SCHEMA public TO datadog;

-- pg_monitorロールを付与
GRANT pg_monitor TO datadog;

-- pg_stat_statements拡張機能を有効化（testdbデータベース）
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- testdbデータベースのテーブルへの権限を付与
GRANT SELECT ON ALL TABLES IN SCHEMA public TO datadog;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO datadog;

-- explain_statement関数を作成（実行計画取得用）
-- 参考: https://docs.datadoghq.com/ja/database_monitoring/setup_postgres/troubleshooting/
-- 注意: EXPLAINはVOLATILE関数でしか実行できないため、明示的にVOLATILEを指定
CREATE OR REPLACE FUNCTION datadog.explain_statement(query TEXT)
RETURNS TABLE(
    plan JSONB
)
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
AS $$
DECLARE
    result JSONB;
BEGIN
    -- EXPLAIN (FORMAT JSON) を実行して実行計画を取得
    EXECUTE format('EXPLAIN (FORMAT JSON) %s', query) INTO result;
    
    -- 結果を返す
    RETURN QUERY SELECT result;
END;
$$;

-- datadogユーザーに実行権限を付与
GRANT EXECUTE ON FUNCTION datadog.explain_statement(TEXT) TO datadog;

-- 確認
SELECT 'Datadog user setup completed successfully' AS status;
\du datadog
