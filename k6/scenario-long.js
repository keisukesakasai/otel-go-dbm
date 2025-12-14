import http from 'k6/http';
import { check, sleep } from 'k6';

// 長時間実行シナリオ: 1時間継続、低頻度（1-3req/s）
export const options = {
  stages: [
    { duration: '30s', target: 2 },      // 30秒で2ユーザーまで増加
    { duration: '1h', target: 2 },       // 1時間2ユーザーを維持（低頻度）
    { duration: '30s', target: 0 },      // 30秒で0ユーザーまで減少
  ],
  thresholds: {
    http_req_duration: ['p(95)<1000'],
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

  sleep(0.5 + Math.random() * 0.5); // 0.5-1.0秒待機（約1-2req/s）

  // 複雑なクエリ: ユーザー別注文統計（JOIN、集約）
  let analyticsRes = http.get(`${BASE_URL}/api/v1/analytics/user-orders`, {
    tags: { name: 'analytics_user_orders' }
  });
  check(analyticsRes, {
    'analytics user orders status is 200': (r) => r.status === 200,
  });

  sleep(0.5 + Math.random() * 0.5);

  // 複雑なクエリ: 商品別売上統計（JOIN、集約）
  let productSalesRes = http.get(`${BASE_URL}/api/v1/analytics/product-sales`, {
    tags: { name: 'analytics_product_sales' }
  });
  check(productSalesRes, {
    'analytics product sales status is 200': (r) => r.status === 200,
  });

  sleep(0.5 + Math.random() * 0.5);

  // 複雑なクエリ: カテゴリ別統計（GROUP BY、HAVING）
  let categoryRes = http.get(`${BASE_URL}/api/v1/analytics/category`, {
    tags: { name: 'analytics_category' }
  });
  check(categoryRes, {
    'analytics category status is 200': (r) => r.status === 200,
  });

  sleep(0.5 + Math.random() * 0.5);

  // 注文詳細取得（複雑な3テーブルJOIN）
  // 注: 実際のorder_idが必要なため、ランダムなIDを試す（404が返る可能性がある）
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

