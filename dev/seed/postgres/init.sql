-- Deterministic production-shaped data for Quarro's browser, diagrams,
-- editor, views, and stored-routine UI. Runs once on an empty volume.

CREATE TABLE regions (id SMALLSERIAL PRIMARY KEY, code TEXT NOT NULL UNIQUE, name TEXT NOT NULL, tax_rate NUMERIC(5,4) NOT NULL CHECK (tax_rate >= 0));
CREATE TABLE warehouses (id SMALLSERIAL PRIMARY KEY, region_id SMALLINT NOT NULL REFERENCES regions(id), code TEXT NOT NULL UNIQUE, name TEXT NOT NULL, capacity INTEGER NOT NULL CHECK (capacity > 0), opened_at DATE NOT NULL);
CREATE TABLE suppliers (id SERIAL PRIMARY KEY, name TEXT NOT NULL, country_code CHAR(2) NOT NULL, rating NUMERIC(3,2) NOT NULL CHECK (rating BETWEEN 0 AND 5), active BOOLEAN NOT NULL DEFAULT true);
CREATE TABLE categories (id SERIAL PRIMARY KEY, parent_id INTEGER REFERENCES categories(id), slug TEXT NOT NULL UNIQUE, name TEXT NOT NULL);
CREATE TABLE customers (
    id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL UNIQUE,
    plan TEXT NOT NULL CHECK (plan IN ('free','pro','enterprise')),
    status TEXT NOT NULL CHECK (status IN ('active','trial','suspended','churned')),
    signup_at TIMESTAMPTZ NOT NULL, last_seen_at TIMESTAMPTZ,
    marketing_opt_in BOOLEAN NOT NULL DEFAULT false,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE TABLE addresses (
    id BIGSERIAL PRIMARY KEY, customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('billing','shipping')), line1 TEXT NOT NULL,
    city TEXT NOT NULL, region_code TEXT NOT NULL, postal_code TEXT NOT NULL,
    country_code CHAR(2) NOT NULL, is_default BOOLEAN NOT NULL DEFAULT false
);
CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY, supplier_id INTEGER NOT NULL REFERENCES suppliers(id),
    category_id INTEGER NOT NULL REFERENCES categories(id), sku TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL, description TEXT, price NUMERIC(12,2) NOT NULL CHECK (price >= 0),
    cost NUMERIC(12,2) NOT NULL CHECK (cost >= 0), active BOOLEAN NOT NULL DEFAULT true,
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE inventory (
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    warehouse_id SMALLINT NOT NULL REFERENCES warehouses(id), quantity INTEGER NOT NULL CHECK (quantity >= 0),
    reserved INTEGER NOT NULL DEFAULT 0 CHECK (reserved >= 0), reorder_point INTEGER NOT NULL DEFAULT 20 CHECK (reorder_point >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (product_id, warehouse_id), CHECK (reserved <= quantity)
);
CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY, customer_id BIGINT NOT NULL REFERENCES customers(id),
    billing_address_id BIGINT NOT NULL REFERENCES addresses(id), shipping_address_id BIGINT NOT NULL REFERENCES addresses(id),
    status TEXT NOT NULL CHECK (status IN ('pending','processing','shipped','delivered','cancelled','returned')),
    channel TEXT NOT NULL CHECK (channel IN ('web','mobile','marketplace','sales')),
    currency CHAR(3) NOT NULL DEFAULT 'USD', promo_code TEXT, notes TEXT,
    ordered_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE order_items (
    order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE, line_number SMALLINT NOT NULL,
    product_id BIGINT NOT NULL REFERENCES products(id), quantity SMALLINT NOT NULL CHECK (quantity > 0),
    unit_price NUMERIC(12,2) NOT NULL CHECK (unit_price >= 0),
    discount_percent NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (discount_percent BETWEEN 0 AND 100),
    PRIMARY KEY (order_id, line_number)
);
CREATE TABLE payments (
    id BIGSERIAL PRIMARY KEY, order_id BIGINT NOT NULL REFERENCES orders(id), provider TEXT NOT NULL,
    provider_reference TEXT NOT NULL UNIQUE, amount NUMERIC(14,2) NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('authorized','captured','failed','refunded')), paid_at TIMESTAMPTZ
);
CREATE TABLE shipments (
    id BIGSERIAL PRIMARY KEY, order_id BIGINT NOT NULL REFERENCES orders(id), warehouse_id SMALLINT NOT NULL REFERENCES warehouses(id),
    carrier TEXT NOT NULL, tracking_number TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('label_created','in_transit','delivered','returned')),
    shipped_at TIMESTAMPTZ, delivered_at TIMESTAMPTZ
);
CREATE TABLE reviews (
    id BIGSERIAL PRIMARY KEY, product_id BIGINT NOT NULL REFERENCES products(id), customer_id BIGINT NOT NULL REFERENCES customers(id),
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5), title TEXT NOT NULL, body TEXT,
    verified_purchase BOOLEAN NOT NULL DEFAULT false, created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (product_id, customer_id)
);
CREATE TABLE support_tickets (
    id BIGSERIAL PRIMARY KEY, customer_id BIGINT NOT NULL REFERENCES customers(id), order_id BIGINT REFERENCES orders(id),
    priority TEXT NOT NULL CHECK (priority IN ('low','normal','high','urgent')),
    status TEXT NOT NULL CHECK (status IN ('open','waiting','resolved','closed')),
    subject TEXT NOT NULL, assigned_team TEXT, opened_at TIMESTAMPTZ NOT NULL, resolved_at TIMESTAMPTZ
);
CREATE TABLE audit_events (
    id BIGSERIAL PRIMARY KEY, actor_type TEXT NOT NULL, actor_id BIGINT,
    entity_type TEXT NOT NULL, entity_id BIGINT NOT NULL, action TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb, occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX customers_plan_status_idx ON customers(plan,status);
CREATE INDEX customers_signup_at_idx ON customers(signup_at DESC);
CREATE INDEX addresses_customer_idx ON addresses(customer_id);
CREATE INDEX products_category_active_idx ON products(category_id,active);
CREATE INDEX products_attributes_gin_idx ON products USING GIN(attributes);
CREATE INDEX inventory_low_stock_idx ON inventory(warehouse_id,quantity) WHERE quantity <= reorder_point;
CREATE INDEX orders_customer_date_idx ON orders(customer_id,ordered_at DESC);
CREATE INDEX orders_status_date_idx ON orders(status,ordered_at DESC);
CREATE INDEX order_items_product_idx ON order_items(product_id);
CREATE INDEX payments_order_idx ON payments(order_id);
CREATE INDEX shipments_order_idx ON shipments(order_id);
CREATE INDEX reviews_product_rating_idx ON reviews(product_id,rating);
CREATE INDEX support_tickets_queue_idx ON support_tickets(status,priority,opened_at);
CREATE INDEX audit_events_entity_idx ON audit_events(entity_type,entity_id,occurred_at DESC);
CREATE INDEX audit_events_payload_gin_idx ON audit_events USING GIN(payload);

INSERT INTO regions(code,name,tax_rate) VALUES
 ('US-E','US East',0.0625),('US-W','US West',0.0725),('CA','Canada',0.0500),
 ('EU','European Union',0.2000),('APAC','Asia Pacific',0.1000),('LATAM','Latin America',0.0850);
INSERT INTO warehouses(region_id,code,name,capacity,opened_at) VALUES
 (1,'IAD-1','Virginia Fulfillment',180000,'2019-04-01'),(1,'EWR-1','Newark Fulfillment',120000,'2021-02-15'),
 (2,'PDX-1','Portland Fulfillment',150000,'2020-08-10'),(2,'LAX-1','Los Angeles Fulfillment',220000,'2018-11-20'),
 (3,'YYZ-1','Toronto Fulfillment',90000,'2022-03-01'),(4,'FRA-1','Frankfurt Fulfillment',175000,'2019-09-12'),
 (5,'SIN-1','Singapore Fulfillment',140000,'2021-06-17'),(6,'GRU-1','Sao Paulo Fulfillment',80000,'2023-01-09');
INSERT INTO suppliers(name,country_code,rating,active)
SELECT 'Supplier '||lpad(g::text,2,'0'),(ARRAY['US','CA','DE','JP','GB','BR','SG'])[((g-1)%7)+1],
       (3.20+((g*17)%18)/10.0)::numeric(3,2),g%11<>0 FROM generate_series(1,25) g;
INSERT INTO categories(parent_id,slug,name) VALUES
 (NULL,'computing','Computing'),(NULL,'office','Office'),(NULL,'lifestyle','Lifestyle'),
 (1,'laptops','Laptops'),(1,'components','Components'),(1,'storage','Storage'),
 (2,'furniture','Furniture'),(2,'peripherals','Peripherals'),(2,'audio-video','Audio & Video'),
 (3,'travel','Travel'),(3,'wellness','Wellness'),(3,'smart-home','Smart Home');

INSERT INTO customers(name,email,plan,status,signup_at,last_seen_at,marketing_opt_in,metadata)
SELECT 'Customer '||lpad(g::text,4,'0'),'customer'||g||'@example.test',
 (ARRAY['free','free','pro','pro','enterprise'])[((g-1)%5)+1],
 (ARRAY['active','active','active','trial','suspended','churned'])[((g-1)%6)+1],
 timestamptz '2022-01-01 09:00+00'+(g%900)*interval '1 day',
 CASE WHEN g%9=0 THEN NULL ELSE timestamptz '2025-01-01 08:00+00'+(g%365)*interval '1 day' END,
 g%3=0,jsonb_build_object('segment',(ARRAY['startup','agency','education','retail'])[((g-1)%4)+1],'score',(g*37)%100)
FROM generate_series(1,2000) g;
INSERT INTO addresses(customer_id,kind,line1,city,region_code,postal_code,country_code,is_default)
SELECT id,'shipping',(100+id)||' Market Street',
 (ARRAY['Richmond','Portland','Toronto','Berlin','Singapore','Sao Paulo'])[((id-1)%6)+1],
 (ARRAY['VA','OR','ON','BE','SG','SP'])[((id-1)%6)+1],lpad((10000+id%89999)::text,5,'0'),
 (ARRAY['US','US','CA','DE','SG','BR'])[((id-1)%6)+1],true FROM customers;
INSERT INTO addresses(customer_id,kind,line1,city,region_code,postal_code,country_code,is_default)
SELECT id,'billing',(500+id)||' Billing Avenue','New York','NY',lpad((20000+id%70000)::text,5,'0'),'US',false
FROM customers WHERE id<=1000;
INSERT INTO products(supplier_id,category_id,sku,name,description,price,cost,active,attributes,created_at,updated_at)
SELECT ((g-1)%25)+1,((g-1)%12)+1,'SKU-'||lpad(g::text,5,'0'),'Product '||lpad(g::text,3,'0'),
 'Production-quality demo product, variant '||(g%8),(19+(g*791%98000)/100.0)::numeric(12,2),
 (8+(g*433%42000)/100.0)::numeric(12,2),g%17<>0,
 jsonb_build_object('color',(ARRAY['black','white','blue','green','natural'])[((g-1)%5)+1],
                    'weightGrams',100+(g*43)%4900,'warrantyYears',1+g%4),
 timestamptz '2022-01-01'+(g%700)*interval '1 day',timestamptz '2025-01-01'+(g%300)*interval '1 day'
FROM generate_series(1,250) g;
INSERT INTO inventory(product_id,warehouse_id,quantity,reserved,reorder_point,updated_at)
SELECT p.id,w.id,(p.id*19+w.id*31)%420,
 LEAST((p.id*7+w.id)%35,(p.id*19+w.id*31)%420),20+(p.id%35),
 timestamptz '2025-09-01'+((p.id+w.id)%30)*interval '1 day' FROM products p CROSS JOIN warehouses w;
INSERT INTO orders(customer_id,billing_address_id,shipping_address_id,status,channel,currency,promo_code,notes,ordered_at,updated_at)
SELECT ((g*37-1)%2000)+1,((g*37-1)%2000)+1,((g*37-1)%2000)+1,
 (ARRAY['pending','processing','shipped','delivered','delivered','delivered','cancelled','returned'])[((g-1)%8)+1],
 (ARRAY['web','mobile','marketplace','sales'])[((g-1)%4)+1],
 (ARRAY['USD','USD','USD','CAD','EUR'])[((g-1)%5)+1],
 CASE WHEN g%7=0 THEN 'SAVE'||(10+g%20) END,CASE WHEN g%23=0 THEN 'Gift wrap; leave at reception.' END,
 timestamptz '2024-01-01 10:00+00'+(g%700)*interval '1 day'+(g%24)*interval '1 hour',
 timestamptz '2024-01-01 12:00+00'+(g%700)*interval '1 day'+(g%24)*interval '1 hour'
FROM generate_series(1,5000) g;
INSERT INTO order_items(order_id,line_number,product_id,quantity,unit_price,discount_percent)
SELECT o.id,line,p.id,1+((o.id+line)%4),p.price,
 CASE WHEN (o.id+line)%9=0 THEN 15 WHEN (o.id+line)%5=0 THEN 5 ELSE 0 END
FROM orders o CROSS JOIN generate_series(1,3) line
JOIN products p ON p.id=((o.id*13+line*17-1)%250)+1;
INSERT INTO payments(order_id,provider,provider_reference,amount,status,paid_at)
SELECT o.id,(ARRAY['stripe','adyen','paypal'])[((o.id-1)%3)+1],'pay_'||lpad(o.id::text,8,'0'),
 round(sum(oi.quantity*oi.unit_price*(1-oi.discount_percent/100)),2),
 CASE WHEN o.status='returned' THEN 'refunded' WHEN o.id%29=0 THEN 'failed' ELSE 'captured' END,
 CASE WHEN o.id%29<>0 THEN o.ordered_at+interval '3 minutes' END
FROM orders o JOIN order_items oi ON oi.order_id=o.id WHERE o.status NOT IN ('pending','cancelled') GROUP BY o.id;
INSERT INTO shipments(order_id,warehouse_id,carrier,tracking_number,status,shipped_at,delivered_at)
SELECT id,((id-1)%8)+1,(ARRAY['UPS','FedEx','DHL','USPS'])[((id-1)%4)+1],
 'TRK'||lpad(id::text,10,'0'),CASE status WHEN 'delivered' THEN 'delivered' WHEN 'returned' THEN 'returned' ELSE 'in_transit' END,
 ordered_at+interval '1 day',CASE WHEN status IN ('delivered','returned') THEN ordered_at+(2+id%6)*interval '1 day' END
FROM orders WHERE status IN ('shipped','delivered','returned');
INSERT INTO reviews(product_id,customer_id,rating,title,body,verified_purchase,created_at)
SELECT ((g*17-1)%250)+1,g,1+(g*7)%5,
 (ARRAY['Exceeded expectations','Solid daily driver','Good value','Mixed experience','Would buy again'])[((g-1)%5)+1],
 CASE WHEN g%6<>0 THEN 'Review '||g||': details about fit, delivery, and long-term use.' END,g%4<>0,
 timestamptz '2024-02-01'+(g%600)*interval '1 day' FROM generate_series(1,2000) g;
INSERT INTO support_tickets(customer_id,order_id,priority,status,subject,assigned_team,opened_at,resolved_at)
SELECT ((g*29-1)%2000)+1,((g*41-1)%5000)+1,
 (ARRAY['low','normal','normal','high','urgent'])[((g-1)%5)+1],
 (ARRAY['open','waiting','resolved','closed'])[((g-1)%4)+1],
 (ARRAY['Delivery question','Return request','Billing discrepancy','Product setup help','Account access'])[((g-1)%5)+1],
 (ARRAY['logistics','returns','billing','technical','accounts'])[((g-1)%5)+1],
 timestamptz '2025-01-01'+(g%300)*interval '1 day',
 CASE WHEN g%4 IN (2,3) THEN timestamptz '2025-01-02'+(g%300)*interval '1 day' END
FROM generate_series(1,600) g;
INSERT INTO audit_events(actor_type,actor_id,entity_type,entity_id,action,payload,occurred_at)
SELECT CASE WHEN g%8=0 THEN 'system' ELSE 'customer' END,
 CASE WHEN g%8<>0 THEN ((g*37-1)%2000)+1 END,
 (ARRAY['order','customer','product','ticket'])[((g-1)%4)+1],1+(g*19)%5000,
 (ARRAY['created','updated','viewed','exported','status_changed'])[((g-1)%5)+1],
 jsonb_build_object('requestId','req-'||lpad(g::text,7,'0'),'source',CASE WHEN g%3=0 THEN 'api' ELSE 'web' END),
 timestamptz '2025-01-01'+(g%300)*interval '1 day'+(g%86400)*interval '1 second'
FROM generate_series(1,10000) g;

CREATE VIEW order_totals AS
SELECT o.id order_id,o.customer_id,o.status,o.currency,o.ordered_at,count(oi.line_number) line_count,
 round(sum(oi.quantity*oi.unit_price*(1-oi.discount_percent/100)),2) total
FROM orders o JOIN order_items oi ON oi.order_id=o.id GROUP BY o.id;
CREATE VIEW customer_lifetime_value AS
SELECT c.id customer_id,c.name,c.plan,c.status,
 count(ot.order_id) FILTER (WHERE ot.status<>'cancelled') order_count,
 coalesce(round(sum(ot.total) FILTER (WHERE ot.status<>'cancelled'),2),0) lifetime_value,
 max(ot.ordered_at) last_order_at
FROM customers c LEFT JOIN order_totals ot ON ot.customer_id=c.id GROUP BY c.id;
CREATE VIEW product_performance AS
SELECT p.id product_id,p.sku,p.name,c.name category,coalesce(sum(oi.quantity),0) units_sold,
 coalesce(round(sum(oi.quantity*oi.unit_price*(1-oi.discount_percent/100)),2),0) gross_revenue,
 round(avg(r.rating),2) average_rating,count(DISTINCT r.id) review_count
FROM products p JOIN categories c ON c.id=p.category_id
LEFT JOIN order_items oi ON oi.product_id=p.id LEFT JOIN reviews r ON r.product_id=p.id GROUP BY p.id,c.name;
CREATE VIEW order_fulfillment AS
SELECT o.id order_id,c.name customer,o.status order_status,s.status shipment_status,s.carrier,s.tracking_number,
 o.ordered_at,s.shipped_at,s.delivered_at,extract(epoch FROM (s.delivered_at-o.ordered_at))/86400 fulfillment_days
FROM orders o JOIN customers c ON c.id=o.customer_id LEFT JOIN shipments s ON s.order_id=o.id;
CREATE VIEW low_stock_inventory AS
SELECT p.sku,p.name product,w.code warehouse,i.quantity,i.reserved,i.quantity-i.reserved available,i.reorder_point
FROM inventory i JOIN products p ON p.id=i.product_id JOIN warehouses w ON w.id=i.warehouse_id
WHERE i.quantity-i.reserved<=i.reorder_point;
CREATE VIEW daily_sales AS
SELECT ordered_at::date sales_date,currency,count(*) orders,round(sum(total),2) revenue,round(avg(total),2) average_order_value
FROM order_totals WHERE status NOT IN ('cancelled','returned') GROUP BY ordered_at::date,currency;

CREATE FUNCTION calculate_order_total(p_order_id BIGINT) RETURNS NUMERIC(14,2)
LANGUAGE sql STABLE AS $$ SELECT coalesce(round(sum(quantity*unit_price*(1-discount_percent/100)),2),0) FROM order_items WHERE order_id=p_order_id $$;
CREATE FUNCTION customer_order_count(p_customer_id BIGINT) RETURNS BIGINT
LANGUAGE sql STABLE AS $$ SELECT count(*) FROM orders WHERE customer_id=p_customer_id AND status<>'cancelled' $$;
CREATE FUNCTION search_products(p_term TEXT)
RETURNS TABLE(product_id BIGINT,sku TEXT,product_name TEXT,price NUMERIC)
LANGUAGE sql STABLE AS $$ SELECT id,sku,name,price FROM products WHERE active AND (name ILIKE '%'||p_term||'%' OR description ILIKE '%'||p_term||'%') ORDER BY name $$;
CREATE PROCEDURE restock_product(p_product_id BIGINT,p_warehouse_id SMALLINT,p_quantity INTEGER)
LANGUAGE plpgsql AS $$ BEGIN
 IF p_quantity<=0 THEN RAISE EXCEPTION 'restock quantity must be positive'; END IF;
 INSERT INTO inventory(product_id,warehouse_id,quantity,reserved,reorder_point)
 VALUES(p_product_id,p_warehouse_id,p_quantity,0,20)
 ON CONFLICT(product_id,warehouse_id) DO UPDATE SET quantity=inventory.quantity+EXCLUDED.quantity,updated_at=now();
END $$;
CREATE FUNCTION touch_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.updated_at=now(); RETURN NEW; END $$;
CREATE TRIGGER products_touch_updated_at BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER orders_touch_updated_at BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE FUNCTION audit_order_status_change() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF OLD.status IS DISTINCT FROM NEW.status THEN
  INSERT INTO audit_events(actor_type,entity_type,entity_id,action,payload)
  VALUES('system','order',NEW.id,'status_changed',jsonb_build_object('from',OLD.status,'to',NEW.status));
 END IF; RETURN NEW;
END $$;
CREATE TRIGGER orders_audit_status AFTER UPDATE OF status ON orders FOR EACH ROW EXECUTE FUNCTION audit_order_status_change();

ANALYZE;
