-- MySQL twin of the Postgres development dataset. The generated rows are
-- deterministic and this file runs once when the data volume is empty.

CREATE TABLE regions (id SMALLINT AUTO_INCREMENT PRIMARY KEY, code VARCHAR(10) NOT NULL UNIQUE, name VARCHAR(80) NOT NULL, tax_rate DECIMAL(5,4) NOT NULL CHECK (tax_rate >= 0)) ENGINE=InnoDB;
CREATE TABLE warehouses (id SMALLINT AUTO_INCREMENT PRIMARY KEY, region_id SMALLINT NOT NULL, code VARCHAR(12) NOT NULL UNIQUE, name VARCHAR(100) NOT NULL, capacity INT NOT NULL CHECK (capacity > 0), opened_at DATE NOT NULL, FOREIGN KEY(region_id) REFERENCES regions(id)) ENGINE=InnoDB;
CREATE TABLE suppliers (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL, country_code CHAR(2) NOT NULL, rating DECIMAL(3,2) NOT NULL CHECK (rating BETWEEN 0 AND 5), active BOOLEAN NOT NULL DEFAULT true) ENGINE=InnoDB;
CREATE TABLE categories (id INT AUTO_INCREMENT PRIMARY KEY, parent_id INT NULL, slug VARCHAR(80) NOT NULL UNIQUE, name VARCHAR(100) NOT NULL, FOREIGN KEY(parent_id) REFERENCES categories(id)) ENGINE=InnoDB;
CREATE TABLE customers (
 id BIGINT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(120) NOT NULL, email VARCHAR(180) NOT NULL UNIQUE,
 plan ENUM('free','pro','enterprise') NOT NULL, status ENUM('active','trial','suspended','churned') NOT NULL,
 signup_at DATETIME NOT NULL, last_seen_at DATETIME NULL, marketing_opt_in BOOLEAN NOT NULL DEFAULT false,
 metadata JSON NOT NULL
) ENGINE=InnoDB;
CREATE TABLE addresses (
 id BIGINT AUTO_INCREMENT PRIMARY KEY, customer_id BIGINT NOT NULL, kind ENUM('billing','shipping') NOT NULL,
 line1 VARCHAR(180) NOT NULL, city VARCHAR(100) NOT NULL, region_code VARCHAR(20) NOT NULL,
 postal_code VARCHAR(20) NOT NULL, country_code CHAR(2) NOT NULL, is_default BOOLEAN NOT NULL DEFAULT false,
 INDEX addresses_customer_idx(customer_id), FOREIGN KEY(customer_id) REFERENCES customers(id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE products (
 id BIGINT AUTO_INCREMENT PRIMARY KEY, supplier_id INT NOT NULL, category_id INT NOT NULL,
 sku VARCHAR(40) NOT NULL UNIQUE, name VARCHAR(160) NOT NULL, description TEXT,
 price DECIMAL(12,2) NOT NULL CHECK (price >= 0), cost DECIMAL(12,2) NOT NULL CHECK (cost >= 0),
 active BOOLEAN NOT NULL DEFAULT true, attributes JSON NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
 INDEX products_category_active_idx(category_id,active), FOREIGN KEY(supplier_id) REFERENCES suppliers(id), FOREIGN KEY(category_id) REFERENCES categories(id)
) ENGINE=InnoDB;
CREATE TABLE inventory (
 product_id BIGINT NOT NULL, warehouse_id SMALLINT NOT NULL, quantity INT NOT NULL CHECK(quantity >= 0),
 reserved INT NOT NULL DEFAULT 0 CHECK(reserved >= 0), reorder_point INT NOT NULL DEFAULT 20 CHECK(reorder_point >= 0),
 updated_at DATETIME NOT NULL, PRIMARY KEY(product_id,warehouse_id), CHECK(reserved <= quantity),
 INDEX inventory_warehouse_quantity_idx(warehouse_id,quantity), FOREIGN KEY(product_id) REFERENCES products(id) ON DELETE CASCADE,
 FOREIGN KEY(warehouse_id) REFERENCES warehouses(id)
) ENGINE=InnoDB;
CREATE TABLE orders (
 id BIGINT AUTO_INCREMENT PRIMARY KEY, customer_id BIGINT NOT NULL, billing_address_id BIGINT NOT NULL, shipping_address_id BIGINT NOT NULL,
 status ENUM('pending','processing','shipped','delivered','cancelled','returned') NOT NULL,
 channel ENUM('web','mobile','marketplace','sales') NOT NULL, currency CHAR(3) NOT NULL DEFAULT 'USD',
 promo_code VARCHAR(40), notes TEXT, ordered_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
 INDEX orders_customer_date_idx(customer_id,ordered_at DESC), INDEX orders_status_date_idx(status,ordered_at DESC),
 FOREIGN KEY(customer_id) REFERENCES customers(id), FOREIGN KEY(billing_address_id) REFERENCES addresses(id), FOREIGN KEY(shipping_address_id) REFERENCES addresses(id)
) ENGINE=InnoDB;
CREATE TABLE order_items (
 order_id BIGINT NOT NULL, line_number SMALLINT NOT NULL, product_id BIGINT NOT NULL,
 quantity SMALLINT NOT NULL CHECK(quantity > 0), unit_price DECIMAL(12,2) NOT NULL CHECK(unit_price >= 0),
 discount_percent DECIMAL(5,2) NOT NULL DEFAULT 0 CHECK(discount_percent BETWEEN 0 AND 100),
 PRIMARY KEY(order_id,line_number), INDEX order_items_product_idx(product_id),
 FOREIGN KEY(order_id) REFERENCES orders(id) ON DELETE CASCADE, FOREIGN KEY(product_id) REFERENCES products(id)
) ENGINE=InnoDB;
CREATE TABLE payments (
 id BIGINT AUTO_INCREMENT PRIMARY KEY, order_id BIGINT NOT NULL, provider VARCHAR(30) NOT NULL,
 provider_reference VARCHAR(80) NOT NULL UNIQUE, amount DECIMAL(14,2) NOT NULL,
 status ENUM('authorized','captured','failed','refunded') NOT NULL, paid_at DATETIME,
 INDEX payments_order_idx(order_id), FOREIGN KEY(order_id) REFERENCES orders(id)
) ENGINE=InnoDB;
CREATE TABLE shipments (
 id BIGINT AUTO_INCREMENT PRIMARY KEY, order_id BIGINT NOT NULL, warehouse_id SMALLINT NOT NULL,
 carrier VARCHAR(30) NOT NULL, tracking_number VARCHAR(80) NOT NULL UNIQUE,
 status ENUM('label_created','in_transit','delivered','returned') NOT NULL, shipped_at DATETIME, delivered_at DATETIME,
 INDEX shipments_order_idx(order_id), FOREIGN KEY(order_id) REFERENCES orders(id), FOREIGN KEY(warehouse_id) REFERENCES warehouses(id)
) ENGINE=InnoDB;
CREATE TABLE reviews (
 id BIGINT AUTO_INCREMENT PRIMARY KEY, product_id BIGINT NOT NULL, customer_id BIGINT NOT NULL,
 rating SMALLINT NOT NULL CHECK(rating BETWEEN 1 AND 5), title VARCHAR(160) NOT NULL, body TEXT,
 verified_purchase BOOLEAN NOT NULL DEFAULT false, created_at DATETIME NOT NULL,
 UNIQUE KEY reviews_product_customer_uk(product_id,customer_id), INDEX reviews_product_rating_idx(product_id,rating),
 FOREIGN KEY(product_id) REFERENCES products(id), FOREIGN KEY(customer_id) REFERENCES customers(id)
) ENGINE=InnoDB;
CREATE TABLE support_tickets (
 id BIGINT AUTO_INCREMENT PRIMARY KEY, customer_id BIGINT NOT NULL, order_id BIGINT,
 priority ENUM('low','normal','high','urgent') NOT NULL, status ENUM('open','waiting','resolved','closed') NOT NULL,
 subject VARCHAR(200) NOT NULL, assigned_team VARCHAR(50), opened_at DATETIME NOT NULL, resolved_at DATETIME,
 INDEX support_tickets_queue_idx(status,priority,opened_at), FOREIGN KEY(customer_id) REFERENCES customers(id), FOREIGN KEY(order_id) REFERENCES orders(id)
) ENGINE=InnoDB;
CREATE TABLE audit_events (
 id BIGINT AUTO_INCREMENT PRIMARY KEY, actor_type VARCHAR(30) NOT NULL, actor_id BIGINT,
 entity_type VARCHAR(30) NOT NULL, entity_id BIGINT NOT NULL, action VARCHAR(50) NOT NULL,
 payload JSON NOT NULL, occurred_at DATETIME NOT NULL,
 INDEX audit_events_entity_idx(entity_type,entity_id,occurred_at DESC)
) ENGINE=InnoDB;

INSERT INTO regions(code,name,tax_rate) VALUES
 ('US-E','US East',0.0625),('US-W','US West',0.0725),('CA','Canada',0.0500),
 ('EU','European Union',0.2000),('APAC','Asia Pacific',0.1000),('LATAM','Latin America',0.0850);
INSERT INTO warehouses(region_id,code,name,capacity,opened_at) VALUES
 (1,'IAD-1','Virginia Fulfillment',180000,'2019-04-01'),(1,'EWR-1','Newark Fulfillment',120000,'2021-02-15'),
 (2,'PDX-1','Portland Fulfillment',150000,'2020-08-10'),(2,'LAX-1','Los Angeles Fulfillment',220000,'2018-11-20'),
 (3,'YYZ-1','Toronto Fulfillment',90000,'2022-03-01'),(4,'FRA-1','Frankfurt Fulfillment',175000,'2019-09-12'),
 (5,'SIN-1','Singapore Fulfillment',140000,'2021-06-17'),(6,'GRU-1','Sao Paulo Fulfillment',80000,'2023-01-09');
INSERT INTO categories(parent_id,slug,name) VALUES
 (NULL,'computing','Computing'),(NULL,'office','Office'),(NULL,'lifestyle','Lifestyle'),
 (1,'laptops','Laptops'),(1,'components','Components'),(1,'storage','Storage'),
 (2,'furniture','Furniture'),(2,'peripherals','Peripherals'),(2,'audio-video','Audio & Video'),
 (3,'travel','Travel'),(3,'wellness','Wellness'),(3,'smart-home','Smart Home');

DELIMITER $$
CREATE PROCEDURE seed_complex_demo()
BEGIN
 DECLARE i INT DEFAULT 1;
 DECLARE j INT;
 DECLARE customer_ref BIGINT;
 DECLARE product_ref BIGINT;
 DECLARE order_status VARCHAR(20);
 DECLARE order_date DATETIME;
 DECLARE product_price DECIMAL(12,2);

 START TRANSACTION;

 WHILE i<=25 DO
  INSERT INTO suppliers(name,country_code,rating,active) VALUES(
   CONCAT('Supplier ',LPAD(i,2,'0')),ELT(1+MOD(i-1,7),'US','CA','DE','JP','GB','BR','SG'),
   3.20+MOD(i*17,18)/10,MOD(i,11)<>0);
  SET i=i+1;
 END WHILE;

 SET i=1;
 WHILE i<=2000 DO
  INSERT INTO customers(name,email,plan,status,signup_at,last_seen_at,marketing_opt_in,metadata) VALUES(
   CONCAT('Customer ',LPAD(i,4,'0')),CONCAT('customer',i,'@example.test'),
   ELT(1+MOD(i-1,5),'free','free','pro','pro','enterprise'),
   ELT(1+MOD(i-1,6),'active','active','active','trial','suspended','churned'),
   DATE_ADD('2022-01-01 09:00:00',INTERVAL MOD(i,900) DAY),
   IF(MOD(i,9)=0,NULL,DATE_ADD('2025-01-01 08:00:00',INTERVAL MOD(i,365) DAY)),MOD(i,3)=0,
   JSON_OBJECT('segment',ELT(1+MOD(i-1,4),'startup','agency','education','retail'),'score',MOD(i*37,100)));
  INSERT INTO addresses(customer_id,kind,line1,city,region_code,postal_code,country_code,is_default) VALUES(
   i,'shipping',CONCAT(100+i,' Market Street'),ELT(1+MOD(i-1,6),'Richmond','Portland','Toronto','Berlin','Singapore','Sao Paulo'),
   ELT(1+MOD(i-1,6),'VA','OR','ON','BE','SG','SP'),LPAD(10000+MOD(i,89999),5,'0'),
   ELT(1+MOD(i-1,6),'US','US','CA','DE','SG','BR'),true);
  SET i=i+1;
 END WHILE;
 SET i=1;
 WHILE i<=1000 DO
  INSERT INTO addresses(customer_id,kind,line1,city,region_code,postal_code,country_code,is_default)
  VALUES(i,'billing',CONCAT(500+i,' Billing Avenue'),'New York','NY',LPAD(20000+MOD(i,70000),5,'0'),'US',false);
  SET i=i+1;
 END WHILE;

 SET i=1;
 WHILE i<=250 DO
  INSERT INTO products(supplier_id,category_id,sku,name,description,price,cost,active,attributes,created_at,updated_at) VALUES(
   1+MOD(i-1,25),1+MOD(i-1,12),CONCAT('SKU-',LPAD(i,5,'0')),CONCAT('Product ',LPAD(i,3,'0')),
   CONCAT('Production-quality demo product, variant ',MOD(i,8)),19+MOD(i*791,98000)/100,
   8+MOD(i*433,42000)/100,MOD(i,17)<>0,
   JSON_OBJECT('color',ELT(1+MOD(i-1,5),'black','white','blue','green','natural'),'weightGrams',100+MOD(i*43,4900),'warrantyYears',1+MOD(i,4)),
   DATE_ADD('2022-01-01',INTERVAL MOD(i,700) DAY),DATE_ADD('2025-01-01',INTERVAL MOD(i,300) DAY));
  SET j=1;
  WHILE j<=8 DO
   INSERT INTO inventory(product_id,warehouse_id,quantity,reserved,reorder_point,updated_at)
   VALUES(i,j,MOD(i*19+j*31,420),LEAST(MOD(i*7+j,35),MOD(i*19+j*31,420)),20+MOD(i,35),DATE_ADD('2025-09-01',INTERVAL MOD(i+j,30) DAY));
   SET j=j+1;
  END WHILE;
  SET i=i+1;
 END WHILE;

 SET i=1;
 WHILE i<=5000 DO
  SET customer_ref=1+MOD(i*37-1,2000);
  SET order_status=ELT(1+MOD(i-1,8),'pending','processing','shipped','delivered','delivered','delivered','cancelled','returned');
  SET order_date=DATE_ADD(DATE_ADD('2024-01-01 10:00:00',INTERVAL MOD(i,700) DAY),INTERVAL MOD(i,24) HOUR);
  INSERT INTO orders(customer_id,billing_address_id,shipping_address_id,status,channel,currency,promo_code,notes,ordered_at,updated_at) VALUES(
   customer_ref,customer_ref,customer_ref,order_status,ELT(1+MOD(i-1,4),'web','mobile','marketplace','sales'),
   ELT(1+MOD(i-1,5),'USD','USD','USD','CAD','EUR'),IF(MOD(i,7)=0,CONCAT('SAVE',10+MOD(i,20)),NULL),
   IF(MOD(i,23)=0,'Gift wrap; leave at reception.',NULL),order_date,DATE_ADD(order_date,INTERVAL 2 HOUR));
  SET j=1;
  WHILE j<=3 DO
   SET product_ref=1+MOD(i*13+j*17-1,250);
   SELECT price INTO product_price FROM products WHERE id=product_ref;
   INSERT INTO order_items(order_id,line_number,product_id,quantity,unit_price,discount_percent)
   VALUES(i,j,product_ref,1+MOD(i+j,4),product_price,IF(MOD(i+j,9)=0,15,IF(MOD(i+j,5)=0,5,0)));
   SET j=j+1;
  END WHILE;
  IF order_status NOT IN ('pending','cancelled') THEN
   INSERT INTO payments(order_id,provider,provider_reference,amount,status,paid_at)
   SELECT i,ELT(1+MOD(i-1,3),'stripe','adyen','paypal'),CONCAT('pay_',LPAD(i,8,'0')),
    ROUND(SUM(quantity*unit_price*(1-discount_percent/100)),2),
    IF(order_status='returned','refunded',IF(MOD(i,29)=0,'failed','captured')),
    IF(MOD(i,29)=0,NULL,DATE_ADD(order_date,INTERVAL 3 MINUTE)) FROM order_items WHERE order_id=i;
  END IF;
  IF order_status IN ('shipped','delivered','returned') THEN
   INSERT INTO shipments(order_id,warehouse_id,carrier,tracking_number,status,shipped_at,delivered_at) VALUES(
    i,1+MOD(i-1,8),ELT(1+MOD(i-1,4),'UPS','FedEx','DHL','USPS'),CONCAT('TRK',LPAD(i,10,'0')),
    IF(order_status='delivered','delivered',IF(order_status='returned','returned','in_transit')),
    DATE_ADD(order_date,INTERVAL 1 DAY),IF(order_status IN ('delivered','returned'),DATE_ADD(order_date,INTERVAL 2+MOD(i,6) DAY),NULL));
  END IF;
  SET i=i+1;
 END WHILE;

 SET i=1;
 WHILE i<=2000 DO
  INSERT INTO reviews(product_id,customer_id,rating,title,body,verified_purchase,created_at) VALUES(
   1+MOD(i*17-1,250),i,1+MOD(i*7,5),
   ELT(1+MOD(i-1,5),'Exceeded expectations','Solid daily driver','Good value','Mixed experience','Would buy again'),
   IF(MOD(i,6)=0,NULL,CONCAT('Review ',i,': details about fit, delivery, and long-term use.')),MOD(i,4)<>0,
   DATE_ADD('2024-02-01',INTERVAL MOD(i,600) DAY));
  SET i=i+1;
 END WHILE;
 SET i=1;
 WHILE i<=600 DO
  INSERT INTO support_tickets(customer_id,order_id,priority,status,subject,assigned_team,opened_at,resolved_at) VALUES(
   1+MOD(i*29-1,2000),1+MOD(i*41-1,5000),ELT(1+MOD(i-1,5),'low','normal','normal','high','urgent'),
   ELT(1+MOD(i-1,4),'open','waiting','resolved','closed'),
   ELT(1+MOD(i-1,5),'Delivery question','Return request','Billing discrepancy','Product setup help','Account access'),
   ELT(1+MOD(i-1,5),'logistics','returns','billing','technical','accounts'),DATE_ADD('2025-01-01',INTERVAL MOD(i,300) DAY),
   IF(MOD(i,4) IN (2,3),DATE_ADD('2025-01-02',INTERVAL MOD(i,300) DAY),NULL));
  SET i=i+1;
 END WHILE;
 SET i=1;
 WHILE i<=10000 DO
  INSERT INTO audit_events(actor_type,actor_id,entity_type,entity_id,action,payload,occurred_at) VALUES(
   IF(MOD(i,8)=0,'system','customer'),IF(MOD(i,8)=0,NULL,1+MOD(i*37-1,2000)),
   ELT(1+MOD(i-1,4),'order','customer','product','ticket'),1+MOD(i*19,5000),
   ELT(1+MOD(i-1,5),'created','updated','viewed','exported','status_changed'),
   JSON_OBJECT('requestId',CONCAT('req-',LPAD(i,7,'0')),'source',IF(MOD(i,3)=0,'api','web')),
   DATE_ADD(DATE_ADD('2025-01-01',INTERVAL MOD(i,300) DAY),INTERVAL MOD(i,86400) SECOND));
  SET i=i+1;
 END WHILE;
 COMMIT;
END$$
DELIMITER ;

CALL seed_complex_demo();
DROP PROCEDURE seed_complex_demo;

CREATE VIEW order_totals AS
SELECT o.id order_id,o.customer_id,o.status,o.currency,o.ordered_at,COUNT(oi.line_number) line_count,
 ROUND(SUM(oi.quantity*oi.unit_price*(1-oi.discount_percent/100)),2) total
FROM orders o JOIN order_items oi ON oi.order_id=o.id GROUP BY o.id,o.customer_id,o.status,o.currency,o.ordered_at;
CREATE VIEW customer_lifetime_value AS
SELECT c.id customer_id,c.name,c.plan,c.status,
 SUM(CASE WHEN ot.status<>'cancelled' THEN 1 ELSE 0 END) order_count,
 COALESCE(ROUND(SUM(CASE WHEN ot.status<>'cancelled' THEN ot.total ELSE 0 END),2),0) lifetime_value,
 MAX(ot.ordered_at) last_order_at
FROM customers c LEFT JOIN order_totals ot ON ot.customer_id=c.id GROUP BY c.id,c.name,c.plan,c.status;
CREATE VIEW product_performance AS
SELECT p.id product_id,p.sku,p.name,c.name category,COALESCE(SUM(oi.quantity),0) units_sold,
 COALESCE(ROUND(SUM(oi.quantity*oi.unit_price*(1-oi.discount_percent/100)),2),0) gross_revenue,
 ROUND(AVG(r.rating),2) average_rating,COUNT(DISTINCT r.id) review_count
FROM products p JOIN categories c ON c.id=p.category_id
LEFT JOIN order_items oi ON oi.product_id=p.id LEFT JOIN reviews r ON r.product_id=p.id
GROUP BY p.id,p.sku,p.name,c.name;
CREATE VIEW order_fulfillment AS
SELECT o.id order_id,c.name customer,o.status order_status,s.status shipment_status,s.carrier,s.tracking_number,
 o.ordered_at,s.shipped_at,s.delivered_at,TIMESTAMPDIFF(HOUR,o.ordered_at,s.delivered_at)/24 fulfillment_days
FROM orders o JOIN customers c ON c.id=o.customer_id LEFT JOIN shipments s ON s.order_id=o.id;
CREATE VIEW low_stock_inventory AS
SELECT p.sku,p.name product,w.code warehouse,i.quantity,i.reserved,i.quantity-i.reserved available,i.reorder_point
FROM inventory i JOIN products p ON p.id=i.product_id JOIN warehouses w ON w.id=i.warehouse_id
WHERE i.quantity-i.reserved<=i.reorder_point;
CREATE VIEW daily_sales AS
SELECT DATE(ordered_at) sales_date,currency,COUNT(*) orders,ROUND(SUM(total),2) revenue,ROUND(AVG(total),2) average_order_value
FROM order_totals WHERE status NOT IN ('cancelled','returned') GROUP BY DATE(ordered_at),currency;

DELIMITER $$
CREATE FUNCTION calculate_order_total(p_order_id BIGINT) RETURNS DECIMAL(14,2)
DETERMINISTIC READS SQL DATA
BEGIN
 DECLARE result DECIMAL(14,2);
 SELECT COALESCE(ROUND(SUM(quantity*unit_price*(1-discount_percent/100)),2),0) INTO result FROM order_items WHERE order_id=p_order_id;
 RETURN result;
END$$
CREATE FUNCTION customer_order_count(p_customer_id BIGINT) RETURNS BIGINT
DETERMINISTIC READS SQL DATA
BEGIN
 DECLARE result BIGINT;
 SELECT COUNT(*) INTO result FROM orders WHERE customer_id=p_customer_id AND status<>'cancelled';
 RETURN result;
END$$
CREATE PROCEDURE restock_product(IN p_product_id BIGINT,IN p_warehouse_id SMALLINT,IN p_quantity INT)
BEGIN
 IF p_quantity<=0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='restock quantity must be positive'; END IF;
 INSERT INTO inventory(product_id,warehouse_id,quantity,reserved,reorder_point,updated_at)
 VALUES(p_product_id,p_warehouse_id,p_quantity,0,20,NOW())
 ON DUPLICATE KEY UPDATE quantity=inventory.quantity+p_quantity,updated_at=NOW();
END$$
CREATE PROCEDURE set_customer_plan(IN p_customer_id BIGINT,IN p_plan VARCHAR(20))
BEGIN
 IF p_plan NOT IN ('free','pro','enterprise') THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='invalid plan'; END IF;
 UPDATE customers SET plan=p_plan WHERE id=p_customer_id;
END$$
CREATE TRIGGER products_touch_updated_at BEFORE UPDATE ON products FOR EACH ROW SET NEW.updated_at=NOW()$$
CREATE TRIGGER orders_touch_updated_at BEFORE UPDATE ON orders FOR EACH ROW SET NEW.updated_at=NOW()$$
CREATE TRIGGER orders_audit_status AFTER UPDATE ON orders FOR EACH ROW
BEGIN
 IF NOT (OLD.status <=> NEW.status) THEN
  INSERT INTO audit_events(actor_type,entity_type,entity_id,action,payload,occurred_at)
  VALUES('system','order',NEW.id,'status_changed',JSON_OBJECT('from',OLD.status,'to',NEW.status),NOW());
 END IF;
END$$
DELIMITER ;

ANALYZE TABLE customers,products,orders,order_items,inventory,audit_events;
