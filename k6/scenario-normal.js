import http from 'k6/http';
import { check, sleep } from 'k6';

// 正常系シナリオ: 短時間で約100リクエストを生成
// 参考サンプルアプリと同じ構造のエンドポイントを呼び出す
export const options = {
  stages: [
    { duration: '10s', target: 5 },     // 10秒で5ユーザーまで増加
    { duration: '30s', target: 5 },     // 30秒間5ユーザーを維持（約100リクエスト）
    { duration: '5s', target: 0 },      // 5秒で0ユーザーまで減少
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'],
    http_req_failed: ['rate<0.20'],    // エラー率20%未満（404は正常なレスポンスとして扱う）
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://app:8080';

export default function () {
  // ヘルスチェック
  let healthRes = http.get(`${BASE_URL}/health`);
  check(healthRes, {
    'health check status is 200': (r) => r.status === 200,
  });

  sleep(0.3 + Math.random() * 0.2); // 0.3-0.5秒待機（約2-3req/s）

  // 複雑なクエリ: ユーザー別注文統計（JOIN、集約）
  let analyticsRes = http.get(`${BASE_URL}/api/v1/analytics/user-orders`, {
    tags: { name: 'analytics_user_orders' }
  });
  check(analyticsRes, {
    'analytics user orders status is 200': (r) => r.status === 200,
  });

  sleep(0.3 + Math.random() * 0.2);

  // 複雑なクエリ: 商品別売上統計（JOIN、集約）
  let productSalesRes = http.get(`${BASE_URL}/api/v1/analytics/product-sales`, {
    tags: { name: 'analytics_product_sales' }
  });
  check(productSalesRes, {
    'analytics product sales status is 200': (r) => r.status === 200,
  });

  sleep(0.3 + Math.random() * 0.2);

  // 複雑なクエリ: カテゴリ別統計（GROUP BY、HAVING）
  let categoryRes = http.get(`${BASE_URL}/api/v1/analytics/category`, {
    tags: { name: 'analytics_category' }
  });
  check(categoryRes, {
    'analytics category status is 200': (r) => r.status === 200,
  });

  sleep(0.3 + Math.random() * 0.2);

  // 注文詳細取得（複雑な3テーブルJOIN）
  // 注: 実際のorder_idが必要なため、ランダムなIDを試す（404が返る可能性がある）
  // エラー率を下げるため、より広い範囲のIDを試す
  const randomOrderId = Math.floor(Math.random() * 10000) + 1;
  let orderDetailsRes = http.get(
    `${BASE_URL}/api/v1/orders/details?order_id=${randomOrderId}`,
    { tags: { name: 'get_order_details' } }
  );
  // 404も正常なレスポンスとして扱う（存在しないIDを試すため）
  check(orderDetailsRes, {
    'get order details status is 200 or 404': (r) => r.status === 200 || r.status === 404,
  });
}
