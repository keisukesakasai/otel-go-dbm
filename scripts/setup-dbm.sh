#!/bin/bash

# Datadog Database Monitoring用ユーザー作成スクリプト

set -e

# 環境変数から設定を読み込む
DB_HOST=${DB_HOST}
DB_PORT=${DB_PORT:-5432}
DB_USER=${DB_USER:-postgres}
DB_NAME=${DB_NAME:-testdb}

echo "=========================================="
echo "Datadog DBM用ユーザー作成スクリプト"
echo "=========================================="
echo "ホスト: $DB_HOST"
echo "ポート: $DB_PORT"
echo "ユーザー: $DB_USER"
echo "データベース: $DB_NAME"
echo "=========================================="
echo ""

# パスワードの確認
if [ -z "$DB_PASSWORD" ]; then
    echo "⚠️  DB_PASSWORD環境変数が設定されていません"
    echo "以下のコマンドでパスワードを設定してください:"
    echo "  export DB_PASSWORD=<your-password>"
    echo ""
    read -sp "パスワードを入力してください: " DB_PASSWORD
    echo ""
fi

# Datadog用ユーザーのパスワード
if [ -z "$DATADOG_USER_PASSWORD" ]; then
    echo ""
    read -sp "Datadog用ユーザーのパスワードを入力してください（新規作成の場合）: " DATADOG_USER_PASSWORD
    echo ""
    if [ -z "$DATADOG_USER_PASSWORD" ]; then
        echo "⚠️  Datadog用ユーザーのパスワードが設定されていません"
        echo "既存のユーザーを使用する場合は、DATADOG_USER_PASSWORDを空のままにしてください"
        read -p "既存のユーザーを使用しますか？ (y/n): " USE_EXISTING
        if [ "$USE_EXISTING" != "y" ]; then
            exit 1
        fi
    fi
fi

echo ""
echo "CloudSQLに接続中..."

# SQLスクリプトを一時ファイルに作成
TEMP_SQL=$(mktemp)
cat > "$TEMP_SQL" <<EOF
-- Datadog用ユーザーを作成（既に存在する場合はスキップ）
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_user WHERE usename = 'datadog') THEN
        CREATE USER datadog WITH PASSWORD '${DATADOG_USER_PASSWORD}';
        RAISE NOTICE 'Datadog user created successfully';
    ELSE
        RAISE NOTICE 'Datadog user already exists';
    END IF;
END
\$\$;

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

-- pg_stat_statements拡張機能を有効化（postgresデータベース）
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- testdbデータベースに接続して設定
\c ${DB_NAME}

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
-- 注意: EXPLAINはVOLATILE関数でしか実行できないため、明示的にVOLATILEを指定
CREATE OR REPLACE FUNCTION datadog.explain_statement(query TEXT)
RETURNS TABLE(
    plan JSONB
)
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
AS \$\$
DECLARE
    result JSONB;
BEGIN
    EXECUTE format('EXPLAIN (FORMAT JSON) %s', query) INTO result;
    RETURN QUERY SELECT result;
END;
\$\$;

-- datadogユーザーに実行権限を付与
GRANT EXECUTE ON FUNCTION datadog.explain_statement(TEXT) TO datadog;

-- 確認
SELECT 'Datadog user setup completed successfully' AS status;
EOF

# SQLを実行
docker run --rm -i \
  -e PGPASSWORD="$DB_PASSWORD" \
  postgres:15-alpine \
  psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres < "$TEMP_SQL"

# 一時ファイルを削除
rm "$TEMP_SQL"

echo ""
echo "✅ Datadog用ユーザーのセットアップが完了しました"
echo ""
echo "次のステップ:"
echo "1. datadog/conf.d/postgres.d/conf.yaml の username を 'datadog' に変更"
echo "2. datadog/conf.d/postgres.d/conf.yaml の password を設定"
echo "3. docker-compose restart datadog-agent でDatadog Agentを再起動"

