// Production-shaped MongoDB data: nested order aggregates, heterogeneous
// reviews/events, validation, indexes, and aggregation views. Runs once on
// an empty compose volume.

const plans = ["free", "free", "pro", "pro", "enterprise"];
const customerStatuses = ["active", "active", "active", "trial", "suspended", "churned"];
const orderStatuses = ["pending", "processing", "shipped", "delivered", "delivered", "delivered", "cancelled", "returned"];
const channels = ["web", "mobile", "marketplace", "sales"];
const categories = ["laptops", "components", "storage", "furniture", "peripherals", "audio-video", "travel", "wellness", "smart-home"];
const colors = ["black", "white", "blue", "green", "natural"];

function batches(collection, documents, size = 1000) {
  for (let start = 0; start < documents.length; start += size) {
    collection.insertMany(documents.slice(start, start + size), { ordered: false });
  }
}

db.createCollection("customers", {
  validator: { $jsonSchema: {
    bsonType: "object", required: ["customerId", "name", "email", "plan", "status", "signupAt"],
    properties: {
      customerId: { bsonType: "int" }, email: { bsonType: "string" },
      plan: { enum: ["free", "pro", "enterprise"] },
      status: { enum: ["active", "trial", "suspended", "churned"] },
    },
  } },
});
db.createCollection("products");
db.createCollection("orders");
db.createCollection("reviews");
db.createCollection("supportTickets");
db.createCollection("auditEvents");

const customers = [];
for (let i = 1; i <= 2000; i++) {
  const country = ["US", "US", "CA", "DE", "SG", "BR"][(i - 1) % 6];
  const customer = {
    customerId: NumberInt(i),
    name: `Customer ${String(i).padStart(4, "0")}`,
    email: `customer${i}@example.test`,
    plan: plans[(i - 1) % plans.length],
    status: customerStatuses[(i - 1) % customerStatuses.length],
    signupAt: new Date(Date.UTC(2022, 0, 1 + (i % 900))),
    marketing: { optedIn: i % 3 === 0, channels: i % 2 ? ["email"] : ["email", "product"] },
    segment: ["startup", "agency", "education", "retail"][(i - 1) % 4],
    score: (i * 37) % 100,
    addresses: [{
      kind: "shipping", line1: `${100 + i} Market Street`,
      city: ["Richmond", "Portland", "Toronto", "Berlin", "Singapore", "Sao Paulo"][(i - 1) % 6],
      country, postalCode: String(10000 + (i % 89999)).padStart(5, "0"), isDefault: true,
    }],
  };
  if (i <= 1000) customer.addresses.push({ kind: "billing", line1: `${500 + i} Billing Avenue`, city: "New York", country: "US", postalCode: String(20000 + i), isDefault: false });
  if (i % 9 !== 0) customer.lastSeenAt = new Date(Date.UTC(2025, 0, 1 + (i % 365)));
  customers.push(customer);
}
batches(db.customers, customers);

const products = [];
for (let i = 1; i <= 250; i++) {
  const inventory = [];
  for (let warehouse = 1; warehouse <= 8; warehouse++) {
    const quantity = (i * 19 + warehouse * 31) % 420;
    inventory.push({ warehouse: `WH-${warehouse}`, quantity, reserved: Math.min((i * 7 + warehouse) % 35, quantity), reorderPoint: 20 + (i % 35) });
  }
  products.push({
    productId: NumberInt(i), sku: `SKU-${String(i).padStart(5, "0")}`,
    name: `Product ${String(i).padStart(3, "0")}`,
    category: categories[(i - 1) % categories.length], supplierId: NumberInt(1 + ((i - 1) % 25)),
    price: NumberDecimal((19 + ((i * 791) % 98000) / 100).toFixed(2)),
    cost: NumberDecimal((8 + ((i * 433) % 42000) / 100).toFixed(2)),
    active: i % 17 !== 0,
    attributes: { color: colors[(i - 1) % colors.length], weightGrams: 100 + ((i * 43) % 4900), warrantyYears: 1 + (i % 4), variants: i % 4 === 0 ? ["standard", "premium"] : ["standard"] },
    inventory,
    createdAt: new Date(Date.UTC(2022, 0, 1 + (i % 700))),
  });
}
batches(db.products, products);

const orders = [];
for (let i = 1; i <= 5000; i++) {
  const customerId = 1 + (((i * 37) - 1) % 2000);
  const status = orderStatuses[(i - 1) % orderStatuses.length];
  const orderedAt = new Date(Date.UTC(2024, 0, 1 + (i % 700), i % 24));
  const items = [];
  let total = 0;
  for (let line = 1; line <= 3; line++) {
    const productId = 1 + (((i * 13) + (line * 17) - 1) % 250);
    const product = products[productId - 1];
    const quantity = 1 + ((i + line) % 4);
    const discountPercent = (i + line) % 9 === 0 ? 15 : (i + line) % 5 === 0 ? 5 : 0;
    const unitPrice = parseFloat(product.price.toString());
    total += quantity * unitPrice * (1 - discountPercent / 100);
    items.push({ line, productId: NumberInt(productId), sku: product.sku, name: product.name, quantity, unitPrice: product.price, discountPercent });
  }
  const order = {
    orderId: NumberLong(i), customer: { customerId: NumberInt(customerId), email: `customer${customerId}@example.test` },
    status, channel: channels[(i - 1) % channels.length], currency: ["USD", "USD", "USD", "CAD", "EUR"][(i - 1) % 5],
    orderedAt, items, totals: { subtotal: NumberDecimal(total.toFixed(2)), tax: NumberDecimal((total * 0.0725).toFixed(2)), grandTotal: NumberDecimal((total * 1.0725).toFixed(2)) },
    shippingAddress: customers[customerId - 1].addresses[0],
    timeline: [{ status: "created", at: orderedAt }, { status, at: new Date(orderedAt.getTime() + 7200000) }],
  };
  if (i % 7 === 0) order.promotion = { code: `SAVE${10 + (i % 20)}`, source: "campaign" };
  if (!["pending", "cancelled"].includes(status)) order.payment = { provider: ["stripe", "adyen", "paypal"][(i - 1) % 3], reference: `pay_${String(i).padStart(8, "0")}`, status: status === "returned" ? "refunded" : i % 29 === 0 ? "failed" : "captured" };
  if (["shipped", "delivered", "returned"].includes(status)) order.shipment = { warehouse: `WH-${1 + ((i - 1) % 8)}`, carrier: ["UPS", "FedEx", "DHL", "USPS"][(i - 1) % 4], trackingNumber: `TRK${String(i).padStart(10, "0")}`, status: status === "shipped" ? "in_transit" : status };
  orders.push(order);
}
batches(db.orders, orders, 500);

const reviews = [];
for (let i = 1; i <= 2000; i++) {
  const review = {
    reviewId: NumberInt(i), productId: NumberInt(1 + (((i * 17) - 1) % 250)), customerId: NumberInt(i),
    rating: NumberInt(1 + ((i * 7) % 5)), title: ["Exceeded expectations", "Solid daily driver", "Good value", "Mixed experience", "Would buy again"][(i - 1) % 5],
    verifiedPurchase: i % 4 !== 0, createdAt: new Date(Date.UTC(2024, 1, 1 + (i % 600))),
  };
  if (i % 6 !== 0) review.body = `Review ${i}: details about fit, delivery, and long-term use.`;
  if (i % 13 === 0) review.sellerResponse = { team: "care", body: "Thanks for the detailed feedback.", respondedAt: new Date(Date.UTC(2025, 1, 1 + (i % 200))) };
  reviews.push(review);
}
batches(db.reviews, reviews);

const tickets = [];
for (let i = 1; i <= 600; i++) {
  const status = ["open", "waiting", "resolved", "closed"][(i - 1) % 4];
  const openedAt = new Date(Date.UTC(2025, 0, 1 + (i % 300)));
  const ticket = {
    ticketId: NumberInt(i), customerId: NumberInt(1 + (((i * 29) - 1) % 2000)), orderId: NumberLong(1 + (((i * 41) - 1) % 5000)),
    priority: ["low", "normal", "normal", "high", "urgent"][(i - 1) % 5], status,
    subject: ["Delivery question", "Return request", "Billing discrepancy", "Product setup help", "Account access"][(i - 1) % 5],
    assignedTeam: ["logistics", "returns", "billing", "technical", "accounts"][(i - 1) % 5], openedAt,
    messages: [{ author: "customer", body: "Please help with this request.", at: openedAt }],
  };
  if (["resolved", "closed"].includes(status)) ticket.resolvedAt = new Date(openedAt.getTime() + 86400000);
  tickets.push(ticket);
}
batches(db.supportTickets, tickets);

const events = [];
for (let i = 1; i <= 10000; i++) {
  events.push({
    eventId: NumberLong(i), actor: i % 8 === 0 ? { type: "system" } : { type: "customer", id: NumberInt(1 + (((i * 37) - 1) % 2000)) },
    entity: { type: ["order", "customer", "product", "ticket"][(i - 1) % 4], id: NumberLong(1 + ((i * 19) % 5000)) },
    action: ["created", "updated", "viewed", "exported", "status_changed"][(i - 1) % 5],
    context: { requestId: `req-${String(i).padStart(7, "0")}`, source: i % 3 === 0 ? "api" : "web", ip: `10.${i % 255}.${(i * 7) % 255}.${(i * 13) % 255}` },
    occurredAt: new Date(Date.UTC(2025, 0, 1 + (i % 300), 0, 0, i % 86400)),
  });
}
batches(db.auditEvents, events);

db.customers.createIndex({ customerId: 1 }, { unique: true });
db.customers.createIndex({ email: 1 }, { unique: true });
db.customers.createIndex({ plan: 1, status: 1, signupAt: -1 });
db.products.createIndex({ productId: 1 }, { unique: true });
db.products.createIndex({ sku: 1 }, { unique: true });
db.products.createIndex({ category: 1, active: 1 });
db.products.createIndex({ name: "text", description: "text" });
db.orders.createIndex({ orderId: 1 }, { unique: true });
db.orders.createIndex({ "customer.customerId": 1, orderedAt: -1 });
db.orders.createIndex({ status: 1, orderedAt: -1 });
db.orders.createIndex({ "items.productId": 1 });
db.reviews.createIndex({ productId: 1, rating: -1 });
db.supportTickets.createIndex({ status: 1, priority: 1, openedAt: 1 });
db.auditEvents.createIndex({ "entity.type": 1, "entity.id": 1, occurredAt: -1 });

db.createView("deliveredOrders", "orders", [{ $match: { status: "delivered" } }, { $project: { orderId: 1, customer: 1, orderedAt: 1, totals: 1, itemCount: { $size: "$items" } } }]);
db.createView("customerOrderSummary", "orders", [
  { $match: { status: { $ne: "cancelled" } } },
  { $group: { _id: "$customer.customerId", orderCount: { $sum: 1 }, lifetimeValue: { $sum: "$totals.grandTotal" }, lastOrderAt: { $max: "$orderedAt" } } },
  { $sort: { lifetimeValue: -1 } },
]);
