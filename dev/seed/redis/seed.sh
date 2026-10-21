#!/bin/sh
# A varied Redis workload generated server-side in one Lua call: hashes,
# strings containing JSON, sets, sorted sets, lists, streams, counters,
# HyperLogLog, geospatial data, and expiring sessions.
set -eu
R="redis-cli -h redis"

$R FLUSHDB >/dev/null
$R EVAL '
local plans = {"free", "free", "pro", "pro", "enterprise"}
local customer_statuses = {"active", "active", "active", "trial", "suspended", "churned"}
local order_statuses = {"pending", "processing", "shipped", "delivered", "delivered", "delivered", "cancelled", "returned"}
local categories = {"laptops", "components", "storage", "furniture", "peripherals", "audio-video", "travel", "wellness", "smart-home"}

for i = 1, 2000 do
  local plan = plans[((i - 1) % #plans) + 1]
  local status = customer_statuses[((i - 1) % #customer_statuses) + 1]
  local key = "customer:" .. i
  redis.call("HSET", key,
    "name", string.format("Customer %04d", i),
    "email", "customer" .. i .. "@example.test",
    "plan", plan, "status", status,
    "score", (i * 37) % 100,
    "marketing_opt_in", i % 3 == 0 and "1" or "0")
  redis.call("SADD", "customers:plan:" .. plan, key)
  redis.call("SADD", "customers:status:" .. status, key)
  redis.call("ZADD", "customers:by_score", (i * 37) % 100, key)
  redis.call("PFADD", "analytics:unique_customers", "customer-" .. i)
end

for i = 1, 250 do
  local category = categories[((i - 1) % #categories) + 1]
  local price = 19 + ((i * 791) % 98000) / 100
  local stock = (i * 19) % 420
  local key = "product:" .. i
  redis.call("HSET", key,
    "sku", string.format("SKU-%05d", i),
    "name", string.format("Product %03d", i),
    "category", category, "price", string.format("%.2f", price),
    "active", i % 17 == 0 and "0" or "1", "stock", stock)
  redis.call("SADD", "products:category:" .. category, key)
  redis.call("ZADD", "products:by_price", price, key)
  redis.call("ZADD", "inventory:by_stock", stock, key)
  redis.call("SADD", "tags:product:" .. i, "demo", category, i % 2 == 0 and "featured" or "standard")
  redis.call("GEOADD", "warehouses:locations", -77.0369 + (i % 8), 38.9072 - (i % 6), "warehouse:" .. ((i - 1) % 8 + 1))
end

for i = 1, 5000 do
  local customer_id = ((i * 37 - 1) % 2000) + 1
  local status = order_statuses[((i - 1) % #order_statuses) + 1]
  local total = 40 + ((i * 173) % 150000) / 100
  local order = cjson.encode({
    orderId = i, customerId = customer_id, status = status,
    channel = ({"web", "mobile", "marketplace", "sales"})[((i - 1) % 4) + 1],
    currency = ({"USD", "USD", "USD", "CAD", "EUR"})[((i - 1) % 5) + 1],
    total = total,
    items = {
      {productId = ((i * 13) % 250) + 1, quantity = (i % 4) + 1},
      {productId = ((i * 17) % 250) + 1, quantity = ((i + 1) % 4) + 1},
      {productId = ((i * 23) % 250) + 1, quantity = ((i + 2) % 4) + 1}
    }
  })
  local key = "order:" .. i
  redis.call("SET", key, order)
  redis.call("SADD", "orders:status:" .. status, key)
  redis.call("ZADD", "orders:by_total", total, key)
  redis.call("LPUSH", "customer:" .. customer_id .. ":recent_orders", key)
  redis.call("LTRIM", "customer:" .. customer_id .. ":recent_orders", 0, 19)
  redis.call("HINCRBY", "metrics:order_status", status, 1)
  redis.call("HINCRBYFLOAT", "metrics:revenue_by_currency", ({"USD", "USD", "USD", "CAD", "EUR"})[((i - 1) % 5) + 1], total)
end

for i = 1, 10000 do
  redis.call("XADD", "events:audit", "MAXLEN", "~", 10000, "*",
    "event_id", i,
    "entity_type", ({"order", "customer", "product", "ticket"})[((i - 1) % 4) + 1],
    "entity_id", ((i * 19) % 5000) + 1,
    "action", ({"created", "updated", "viewed", "exported", "status_changed"})[((i - 1) % 5) + 1],
    "source", i % 3 == 0 and "api" or "web")
end

for i = 1, 600 do
  local priority = ({"low", "normal", "normal", "high", "urgent"})[((i - 1) % 5) + 1]
  local status = ({"open", "waiting", "resolved", "closed"})[((i - 1) % 4) + 1]
  redis.call("HSET", "ticket:" .. i,
    "customer_id", ((i * 29 - 1) % 2000) + 1,
    "order_id", ((i * 41 - 1) % 5000) + 1,
    "priority", priority, "status", status,
    "subject", ({"Delivery question", "Return request", "Billing discrepancy", "Product setup help", "Account access"})[((i - 1) % 5) + 1])
  redis.call("ZADD", "support:queue:" .. status, ({low=1, normal=2, high=3, urgent=4})[priority] * 100000 + i, "ticket:" .. i)
end

for i = 1, 200 do
  local key = "session:demo:" .. i
  redis.call("SET", key, cjson.encode({customerId=i, device=i % 2 == 0 and "desktop" or "mobile"}), "EX", 3600 + i * 30)
end

redis.call("SET", "feature:recommendations:v2", "enabled")
redis.call("SET", "feature:new_checkout:percentage", "25")
redis.call("HSET", "system:seed", "version", "2", "customers", 2000, "products", 250, "orders", 5000, "events", 10000)
return redis.call("DBSIZE")
' 0 >/dev/null

echo "seeded: $($R DBSIZE) keys"
