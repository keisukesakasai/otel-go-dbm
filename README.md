# OpenTelemetry Go DBM Sample Application

アドベントカレンダー用のサンプルアプリケーションです。Go + GORM + PostgreSQL + OpenTelemetry + Datadog DBM を使用したWebアプリケーションです。

## 構成

- **Goアプリケーション**: GORMを使用したDB操作を含むWebサーバー（参考サンプルアプリと同じ構造）
- **PostgreSQL**: CloudSQL互換のPostgreSQLデータベース
- **k6**: ロードテストツール
- **Datadog Agent**: OpenTelemetryトレースとDBMの収集

## 機能

### API エンドポイント

現在有効なエンドポイント（参考サンプルアプリと同じ構造）：

- `GET /health`: ヘルスチェックエンドポイント（DB接続確認含む）
- `GET /api/v1/analytics/user-orders`: ユーザー別の注文統計（複雑なJOIN、集約クエリ）
- `GET /api/v1/analytics/product-sales`: 商品別の売上統計（複雑なJOIN、集約クエリ）
- `GET /api/v1/analytics/category`: カテゴリ別の売上分析（GROUP BY、HAVING句）
- `GET /api/v1/orders/details?order_id=<id>`: 注文詳細取得（3テーブルJOIN）

### 主な機能

- OpenTelemetryによるトレーシング
- GORMプラグインによる自動DB計装
- Datadog Database Monitoring (DBM) との相関
- 複雑なクエリによる実行計画の可視化
- 参考サンプルアプリと同じ構造（handler構造体、メソッドレシーバー）

## セットアップ

### 前提条件

- Docker & Docker Compose
- Go 1.22以上（ローカル開発時）
- CloudSQL PostgreSQLインスタンス（またはローカルPostgreSQL）

### 起動方法

```bash
# 環境変数を設定（.envファイルを作成するか、以下のようにexport）
# 必須の環境変数:
export DD_API_KEY=<your-datadog-api-key>
export DB_HOST=<your-database-host>
export DB_PASSWORD=<your-database-password>
export DD_DBM_HOST=<your-database-host>
export DD_DBM_PASSWORD=<your-datadog-db-password>

# アプリケーションとDatadog Agentを起動
docker-compose up -d

# ログ確認
docker-compose logs -f app

# ヘルスチェック
curl http://localhost:8081/health
```

**注意**: `.env`ファイルを作成して環境変数を設定することもできます。`.env.example`を参考にしてください。

### ロードテスト実行

```bash
# 正常系シナリオ（約100リクエストを短時間で生成）
docker-compose --profile loadtest run --rm k6-normal
```

シナリオの説明：
- **scenario-normal.js**: 現在有効なエンドポイントを呼び出す正常系シナリオ（約100リクエスト）

### CloudSQL接続設定

CloudSQLに接続する場合は、環境変数で接続情報を設定してください：

```bash
export DB_HOST=<your-cloudsql-host>
export DB_PASSWORD=<your-cloudsql-password>
export DD_DBM_HOST=<your-cloudsql-host>
export DD_DBM_PASSWORD=<your-datadog-db-password>
docker-compose up -d app
```

#### 重要な注意事項

1. **データベースの作成**: CloudSQLに`testdb`データベースが存在することを確認してください。
2. **ファイアウォール設定**: CloudSQLインスタンスの「承認済みネットワーク」に接続元のIPアドレスを追加してください。
3. **SSL接続**: CloudSQLはSSL接続が必須のため、`DB_SSLMODE=require`が設定されています。

### Datadog Database Monitoring (DBM) セットアップ

DBMを有効にするには、以下の手順を実行してください：

1. **Datadogユーザーの作成**: `scripts/setup-dbm-user.sql`を実行して`datadog`ユーザーを作成
2. **実行計画関数の作成**: `scripts/setup-dbm-explain-function.sql`を実行（または`setup-dbm-user.sql`に含まれています）
3. **Datadog Agent設定**: `datadog/conf.d/postgres.d/conf.yaml`の`host`と`password`を実際の値に置き換えてください
4. **Datadog Agentの起動**: `docker-compose up -d datadog-agent`

**重要**: `datadog/conf.d/postgres.d/conf.yaml`には秘匿情報が含まれる可能性があるため、GitHubにプッシュする前に確認してください。

詳細は `DBM_SETUP.md` を参照してください。

## API使用例

```bash
# ヘルスチェック
curl "http://localhost:8081/health"

# ユーザー別の注文統計
curl "http://localhost:8081/api/v1/analytics/user-orders"

# 商品別の売上統計
curl "http://localhost:8081/api/v1/analytics/product-sales"

# カテゴリ別の売上分析
curl "http://localhost:8081/api/v1/analytics/category"

# 注文詳細取得
curl "http://localhost:8081/api/v1/orders/details?order_id=1"
```

## 開発

### ローカル開発

```bash
# 依存関係のインストール
go mod download

# アプリケーションをローカルで実行
go run main.go
```

### ビルド

```bash
# Dockerイメージのビルド
docker-compose build app
```

## 停止

```bash
# すべてのコンテナを停止・削除
docker-compose down
```
