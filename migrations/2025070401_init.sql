-- migrations/2025070401_init.sql
CREATE TABLE IF NOT EXISTS project_orders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_name TEXT NOT NULL,
    full_name TEXT NOT NULL,
    company_name TEXT,
    contact_info TEXT NOT NULL,
    project_link TEXT,
    payment_method TEXT NOT NULL,
    start_date TEXT NOT NULL,
    languages INTEGER NOT NULL,
    ip_address TEXT NOT NULL,
    user_agent TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS cookie_consents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_name TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    user_agent TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    accepted BOOLEAN NOT NULL,
    timestamp DATETIME NOT NULL
);