package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"

	otellog "otel-go-dbm/log"
)

// APIResponse 統一APIレスポンス
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}

// ErrorInfo エラー情報
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var tracer = otel.GetTracerProvider().Tracer("main")
var appLogger *slog.Logger

// initLogger はJSON形式でstdoutに出力するslog loggerを初期化します
func initLogger() {
	// JSON形式でstdoutに出力するハンドラーを作成
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	})

	// TraceHandlerでラップしてtrace_idとspan_idを追加
	traceHandler := otellog.NewTraceHandler(handler, nil)

	appLogger = slog.New(traceHandler)
	slog.SetDefault(appLogger)
}

type handler struct {
	db *sql.DB
}

func initTracer() func() {
	ctx := context.Background()

	// OTLPエクスポーターの設定
	otlpEndpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "datadog-agent:4318")
	otlpHeaders := getEnv("OTEL_EXPORTER_OTLP_HEADERS", "")

	// エンドポイントからプロトコルを除去（WithEndpointはホスト:ポートのみを受け取る）
	endpoint := strings.TrimPrefix(otlpEndpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),            // Datadog AgentはHTTPを使用
		otlptracehttp.WithURLPath("/v1/traces"), // OTLP HTTPエンドポイントのパス
	}

	// ヘッダーが設定されている場合は追加
	if otlpHeaders != "" {
		opts = append(opts, otlptracehttp.WithHeaders(parseHeaders(otlpHeaders)))
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		slog.Error("Failed to create OTLP exporter", "error", err)
		os.Exit(1)
	}

	// リソースの設定
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("otel-go-dbm"),
			semconv.ServiceVersion("1.0.0"),
			semconv.DeploymentEnvironment("advent"),
			attribute.String("telemetry.sdk.language", "go"),
		),
	)
	if err != nil {
		slog.Error("Failed to create resource", "error", err)
		os.Exit(1)
	}

	// トレーサープロバイダーの設定
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("OpenTelemetry tracer initialized")

	// クリーンアップ関数を返す
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			slog.Error("Error shutting down tracer provider", "error", err)
		}
	}
}

func parseHeaders(headers string) map[string]string {
	result := make(map[string]string)
	pairs := strings.Split(headers, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result
}

func initDB() (*sql.DB, error) {
	// 環境変数からDB接続情報を取得
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "advent-user")
	password := getEnv("DB_PASSWORD", "postgres")
	dbname := getEnv("DB_NAME", "testdb")
	sslmode := getEnv("DB_SSLMODE", "disable")

	// PostgreSQL接続文字列を作成
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	// OpenTelemetry計装付きSQLドライバーでデータベース接続を開く
	db, err := otelsql.Open("postgres", dsn,
		otelsql.WithAttributes(
			semconv.DBSystemPostgreSQL,
			semconv.DBName(dbname),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 接続をテスト
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 接続ユーザーを確認
	var currentUser string
	err = db.QueryRow("SELECT current_user").Scan(&currentUser)
	if err != nil {
		return nil, fmt.Errorf("failed to query current_user: %w", err)
	}
	slog.Info("Database connection established", "user", currentUser, "host", host, "database", dbname)

	slog.Info("Database connection established with OpenTelemetry instrumentation")
	return db, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// JSONレスポンスを送信
func sendJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// エラーレスポンスを送信
func sendError(w http.ResponseWriter, statusCode int, code, message string) {
	span := trace.SpanFromContext(context.Background())
	if span.IsRecording() {
		span.RecordError(errors.New(message))
		span.SetAttributes(
			semconv.HTTPStatusCode(statusCode),
		)
	}
	sendJSON(w, statusCode, APIResponse{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	})
}

// 成功レスポンスを送信
func sendSuccess(w http.ResponseWriter, statusCode int, data interface{}) {
	sendJSON(w, statusCode, APIResponse{
		Success: true,
		Data:    data,
	})
}

// ヘルスチェックエンドポイント（handler構造体のメソッドとして実装）
func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "health")
	defer span.End()

	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	// DB Ping
	ctx, dbPingSpan := tracer.Start(ctx, "health.db_ping")
	if err := h.db.PingContext(ctx); err != nil {
		dbPingSpan.RecordError(err)
		dbPingSpan.End()
		span.RecordError(err)
		sendError(w, http.StatusServiceUnavailable, "DB_ERROR", "Database ping failed")
		return
	}
	dbPingSpan.End()

	sendSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

// 複雑なクエリエンドポイント: ユーザー別の注文統計
func (h *handler) getUserOrderAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "getUserOrderAnalytics")
	defer span.End()

	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	type UserOrderStats struct {
		UserID      uint    `json:"user_id"`
		UserName    string  `json:"user_name"`
		UserEmail   string  `json:"user_email"`
		OrderCount  int64   `json:"order_count"`
		TotalAmount float64 `json:"total_amount"`
		AvgAmount   float64 `json:"avg_amount"`
		ItemCount   int64   `json:"item_count"`
	}

	var stats []UserOrderStats

	// クエリ実行
	ctx, querySpan := tracer.Start(ctx, "getUserOrderAnalytics.query")
	querySpan.SetAttributes(
		semconv.DBOperation("SELECT"),
		semconv.DBName(getEnv("DB_NAME", "testdb")),
	)
	defer querySpan.End()

	query := `
		SELECT 
			users.id as user_id,
			users.name as user_name,
			users.email as user_email,
			COUNT(DISTINCT orders.id) as order_count,
			COALESCE(SUM(orders.total_amount), 0) as total_amount,
			COALESCE(AVG(orders.total_amount), 0) as avg_amount,
			COALESCE(SUM(order_items.quantity), 0) as item_count
		FROM users
		LEFT JOIN orders ON orders.user_id = users.id
		LEFT JOIN order_items ON order_items.order_id = orders.id
		GROUP BY users.id, users.name, users.email
		ORDER BY total_amount DESC
		LIMIT 50
	`

	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		querySpan.RecordError(err)
		span.RecordError(err)
		slog.ErrorContext(ctx, "Failed to compute analytics", "error", err)
		sendError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get statistics")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var stat UserOrderStats
		if err := rows.Scan(
			&stat.UserID,
			&stat.UserName,
			&stat.UserEmail,
			&stat.OrderCount,
			&stat.TotalAmount,
			&stat.AvgAmount,
			&stat.ItemCount,
		); err != nil {
			querySpan.RecordError(err)
			span.RecordError(err)
			slog.ErrorContext(ctx, "Failed to scan row", "error", err)
			sendError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to scan results")
			return
		}
		stats = append(stats, stat)
	}

	if err := rows.Err(); err != nil {
		querySpan.RecordError(err)
		span.RecordError(err)
		slog.ErrorContext(ctx, "Row iteration error", "error", err)
		sendError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to iterate results")
		return
	}

	// レスポンス準備
	ctx, responseSpan := tracer.Start(ctx, "getUserOrderAnalytics.prepare_response")
	responseSpan.SetAttributes(
		attribute.Int("stats.count", len(stats)),
	)
	responseSpan.End()

	sendSuccess(w, http.StatusOK, map[string]interface{}{
		"stats": stats,
		"count": len(stats),
	})
}

// 複雑なクエリエンドポイント: 商品別の売上統計
func (h *handler) getProductStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "getProductStats")
	defer span.End()

	slog.InfoContext(ctx, "Computing product review statistics (heavy aggregation)")

	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	type ProductSalesStats struct {
		ProductID    uint    `json:"product_id"`
		ProductName  string  `json:"product_name"`
		Category     string  `json:"category"`
		TotalSold    int64   `json:"total_sold"`
		TotalRevenue float64 `json:"total_revenue"`
		OrderCount   int64   `json:"order_count"`
		AvgPrice     float64 `json:"avg_price"`
	}

	var stats []ProductSalesStats

	// クエリ実行
	ctx, querySpan := tracer.Start(ctx, "getProductStats.query")
	querySpan.SetAttributes(
		semconv.DBOperation("SELECT"),
		semconv.DBName(getEnv("DB_NAME", "testdb")),
	)
	defer querySpan.End()

	query := `
		SELECT 
			products.id as product_id,
			products.name as product_name,
			'' as category,
			COALESCE(SUM(order_items.quantity), 0) as total_sold,
			COALESCE(SUM(order_items.quantity * order_items.unit_price), 0) as total_revenue,
			COUNT(DISTINCT order_items.order_id) as order_count,
			COALESCE(AVG(order_items.unit_price), products.price) as avg_price
		FROM products
		LEFT JOIN order_items ON order_items.product_id = products.id
		LEFT JOIN orders ON orders.id = order_items.order_id AND orders.status = 'completed'
		GROUP BY products.id, products.name, products.price
		ORDER BY total_revenue DESC
		LIMIT 50
	`

	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		querySpan.RecordError(err)
		span.RecordError(err)
		slog.ErrorContext(ctx, "Failed to compute product stats", "error", err)
		sendError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get statistics")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var stat ProductSalesStats
		if err := rows.Scan(
			&stat.ProductID,
			&stat.ProductName,
			&stat.Category,
			&stat.TotalSold,
			&stat.TotalRevenue,
			&stat.OrderCount,
			&stat.AvgPrice,
		); err != nil {
			querySpan.RecordError(err)
			span.RecordError(err)
			slog.ErrorContext(ctx, "Failed to scan row", "error", err)
			sendError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to scan results")
			return
		}
		stats = append(stats, stat)
	}

	if err := rows.Err(); err != nil {
		querySpan.RecordError(err)
		span.RecordError(err)
		slog.ErrorContext(ctx, "Row iteration error", "error", err)
		sendError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to iterate results")
		return
	}

	// レスポンス準備
	ctx, responseSpan := tracer.Start(ctx, "getProductStats.prepare_response")
	responseSpan.SetAttributes(
		attribute.Int("stats.count", len(stats)),
	)
	responseSpan.End()

	sendSuccess(w, http.StatusOK, map[string]interface{}{
		"stats": stats,
		"count": len(stats),
	})
}

// 複雑なクエリエンドポイント: カテゴリ別の売上分析
func (h *handler) getCategoryStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "getCategoryStats")
	defer span.End()

	slog.InfoContext(ctx, "Fetching category statistics")

	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	type ProductStats struct {
		ProductCount int64   `json:"product_count"`
		TotalSold    int64   `json:"total_sold"`
		TotalRevenue float64 `json:"total_revenue"`
		AvgPrice     float64 `json:"avg_price"`
	}

	var stats ProductStats

	// クエリ実行
	ctx, querySpan := tracer.Start(ctx, "getCategoryStats.query")
	querySpan.SetAttributes(
		semconv.DBOperation("SELECT"),
		semconv.DBName(getEnv("DB_NAME", "testdb")),
	)
	defer querySpan.End()

	query := `
		SELECT 
			COUNT(DISTINCT products.id) as product_count,
			COALESCE(SUM(order_items.quantity), 0) as total_sold,
			COALESCE(SUM(order_items.quantity * order_items.unit_price), 0) as total_revenue,
			COALESCE(AVG(order_items.unit_price), 0) as avg_price
		FROM products
		LEFT JOIN order_items ON order_items.product_id = products.id
		LEFT JOIN orders ON orders.id = order_items.order_id
	`

	err := h.db.QueryRowContext(ctx, query).Scan(
		&stats.ProductCount,
		&stats.TotalSold,
		&stats.TotalRevenue,
		&stats.AvgPrice,
	)
	if err != nil {
		querySpan.RecordError(err)
		span.RecordError(err)
		slog.ErrorContext(ctx, "Failed to get category stats", "error", err)
		sendError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get statistics")
		return
	}

	// レスポンス準備
	ctx, responseSpan := tracer.Start(ctx, "getCategoryStats.prepare_response")
	responseSpan.SetAttributes(
		attribute.Int64("product_count", stats.ProductCount),
	)
	responseSpan.End()

	sendSuccess(w, http.StatusOK, map[string]interface{}{
		"stats": stats,
	})
}

// 複雑なクエリエンドポイント: 注文詳細（複数テーブルJOIN）
func (h *handler) getOrderDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "getOrderDetails")
	defer span.End()

	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}

	// パラメータ検証
	ctx, validateSpan := tracer.Start(ctx, "getOrderDetails.validate_params")
	orderIDStr := r.URL.Query().Get("order_id")
	if orderIDStr == "" {
		validateSpan.End()
		sendError(w, http.StatusBadRequest, "MISSING_ORDER_ID", "Order ID is required")
		return
	}

	orderID, err := strconv.ParseUint(orderIDStr, 10, 32)
	if err != nil {
		validateSpan.RecordError(err)
		validateSpan.End()
		span.RecordError(err)
		sendError(w, http.StatusBadRequest, "INVALID_ORDER_ID", "Invalid order ID")
		return
	}
	validateSpan.SetAttributes(
		attribute.Int64("order_id", int64(orderID)),
	)
	validateSpan.End()

	type OrderDetail struct {
		OrderID      uint      `json:"order_id"`
		OrderStatus  string    `json:"order_status"`
		OrderDate    time.Time `json:"order_date"`
		TotalAmount  float64   `json:"total_amount"`
		UserID       uint      `json:"user_id"`
		UserName     string    `json:"user_name"`
		UserEmail    string    `json:"user_email"`
		ItemID       uint      `json:"item_id"`
		ProductID    uint      `json:"product_id"`
		ProductName  string    `json:"product_name"`
		ProductPrice float64   `json:"product_price"`
		Quantity     int       `json:"quantity"`
		ItemTotal    float64   `json:"item_total"`
	}

	var details []OrderDetail

	// クエリ実行
	ctx, querySpan := tracer.Start(ctx, "getOrderDetails.query")
	querySpan.SetAttributes(
		attribute.Int64("order_id", int64(orderID)),
		semconv.DBOperation("SELECT"),
		semconv.DBName(getEnv("DB_NAME", "testdb")),
	)
	defer querySpan.End()

	query := `
		SELECT 
			orders.id as order_id,
			orders.status as order_status,
			orders.order_date,
			orders.total_amount,
			users.id as user_id,
			users.name as user_name,
			users.email as user_email,
			order_items.id as item_id,
			products.id as product_id,
			products.name as product_name,
			order_items.unit_price as product_price,
			order_items.quantity,
			(order_items.unit_price * order_items.quantity) as item_total
		FROM orders
		INNER JOIN users ON users.id = orders.user_id
		LEFT JOIN order_items ON order_items.order_id = orders.id
		LEFT JOIN products ON products.id = order_items.product_id
		WHERE orders.id = $1
	`

	rows, err := h.db.QueryContext(ctx, query, orderID)
	if err != nil {
		querySpan.RecordError(err)
		span.RecordError(err)
		slog.ErrorContext(ctx, "Failed to fetch order details", "error", err)
		sendError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get order details")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var detail OrderDetail
		if err := rows.Scan(
			&detail.OrderID,
			&detail.OrderStatus,
			&detail.OrderDate,
			&detail.TotalAmount,
			&detail.UserID,
			&detail.UserName,
			&detail.UserEmail,
			&detail.ItemID,
			&detail.ProductID,
			&detail.ProductName,
			&detail.ProductPrice,
			&detail.Quantity,
			&detail.ItemTotal,
		); err != nil {
			querySpan.RecordError(err)
			span.RecordError(err)
			slog.ErrorContext(ctx, "Failed to scan row", "error", err)
			sendError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to scan results")
			return
		}
		details = append(details, detail)
	}

	if err := rows.Err(); err != nil {
		querySpan.RecordError(err)
		span.RecordError(err)
		slog.ErrorContext(ctx, "Row iteration error", "error", err)
		sendError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to iterate results")
		return
	}

	querySpan.SetAttributes(
		attribute.Int("details.count", len(details)),
	)

	if len(details) == 0 {
		sendError(w, http.StatusNotFound, "ORDER_NOT_FOUND", "Order not found")
		return
	}

	// レスポンス準備
	ctx, responseSpan := tracer.Start(ctx, "getOrderDetails.prepare_response")
	responseSpan.SetAttributes(
		attribute.Int("details.count", len(details)),
	)
	responseSpan.End()

	sendSuccess(w, http.StatusOK, map[string]interface{}{
		"order_details": details,
		"order_id":      orderID,
	})
}

func main() {
	// ロガーの初期化（最初に実行）
	initLogger()

	// OpenTelemetryトレーサーの初期化
	shutdown := initTracer()
	defer shutdown()

	// DB初期化
	db, err := initDB()
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}

	// ハンドラー作成
	h := &handler{db: db}

	// ルーティング設定
	mux := http.NewServeMux()

	mux.Handle("/health", http.HandlerFunc(h.health))

	// 複雑なクエリエンドポイント（参考サンプルアプリと同じ構造）
	mux.Handle("/api/v1/analytics/user-orders", http.HandlerFunc(h.getUserOrderAnalytics))
	mux.Handle("/api/v1/analytics/product-sales", http.HandlerFunc(h.getProductStats))
	mux.Handle("/api/v1/analytics/category", http.HandlerFunc(h.getCategoryStats))
	mux.Handle("/api/v1/orders/details", http.HandlerFunc(h.getOrderDetails))

	// 参考: 他のエンドポイントは後で追加可能
	// mux.Handle("/api/v1/users", http.HandlerFunc(h.getUsers))
	// mux.Handle("/api/v1/products", http.HandlerFunc(h.getProducts))

	// OpenTelemetry HTTPミドルウェアを適用
	handler := otelhttp.NewHandler(mux, "server")

	port := getEnv("PORT", "8080")
	slog.Info("Server starting", "port", port)

	// シグナルハンドリング
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := http.ListenAndServe(":"+port, handler); err != nil {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-sigChan
	slog.Info("Shutting down server...")
}
