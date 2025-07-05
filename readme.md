# AXCommutator  

**AXCommutator** is a modular communication microservice written in Go, designed to receive, validate, and dispatch event-driven notifications to email and Telegram, using templated messages and multilingual support.

This service runs inside Docker and exposes an HTTP API on a user-defined port (default `8600`). It is recommended to use a domain reverse-proxy like **NGINX** for production environments.

## ✨ Features

* 📨 Email and Telegram notifications with language-specific templates  
* 🔐 CSRF protection, IP whitelisting, and anti-flood mechanisms (DDoS/TDoS protection)  
* 🌍 Multilingual notification support (English, Russian, Spanish, French, etc.)  
* 🛠️ Unlimited services configured via `.env`  
* 📤 Auto-cleaning of uploaded files (e.g., PDFs)  
* � SQLite for logging and consent storage  
* 🔄 Dual-mode Docker setup: production & hot-reload development  
* 🔌 Extensible architecture, Redis integration possible for scale  


### 🔧 Key Environment Variables

1. **`PORT`**  
   The port your service listens on (default: `8600`).

2. **`CSRF_KEY`**  
   Required if you use CSRF protection. This should be a base64-encoded 32-byte key.

3. **Service-specific prefix (`WS_`, `DH_`, etc.)**  
   The variable `*_SERVICE_NAME` must match exactly the name of a folder inside `app/templates/`.  
   For example:  
   ```ini
   WS_SERVICE_NAME=codcl
   → must correspond to folder app/templates/codcl/
````

4. **Templates inside the service folder must include (per language):**

   * `subject_en.txt` — email subject
   * `email_en.txt` — email body
   * `tg_en.txt` — Telegram notification to admin/operator

5. **Unlimited language support**
   Language suffixes like `_en`, `_ru`, `_fr` are fully dynamic.
   To add a new language, simply add all required variables and template files with the new suffix.

6. **Adding a new service**
   Add a new `*_SERVICE_NAME` and define all associated parameters with a custom prefix.
   Example:

   ```ini
   DH_SERVICE_NAME=stdgi
   DH_LANGS=en,fr,es
   DH_EMAIL_SUBJECT_EN_PATH=app/templates/stdgi/subject_en.txt
   ...
   ```

7. **Other general settings**
   These include:

   * `ENV`
   * `ALLOWED_IPS`
   * `DB_PATH`
   * SMTP credentials
   * Telegram bot token & chat ID

---

## 🛠️ Quick Setup (Dev & Prod)

```bash
# Clone the repo
git clone https://github.com/yourusername/axcommutator.git
cd axcommutator

# Create and edit .env file
cp .env.example .env
nano .env

# Build and start (dev or prod)
docker compose up -d            # production
docker compose -f docker-compose.dev.yml up --build  # dev with hot reload

# Logs
docker compose logs -f
```

---

## 🌐 Recommended Deployment (Ubuntu Server)

```bash
# Install dependencies
sudo apt update && sudo apt install -y \
    git docker.io docker-compose nginx certbot python3-certbot-nginx ufw

# Firewall
sudo ufw allow OpenSSH
sudo ufw allow http
sudo ufw allow https
sudo ufw enable

# Clone & prepare
git clone https://github.com/yourusername/axcommutator.git
cd axcommutator
cp .env.example .env
nano .env
```

### Reverse Proxy via NGINX

```nginx
server {
    server_name yourdomain.com;

    location / {
        proxy_pass http://localhost:8600;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    listen 443 ssl;
    ssl_certificate /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;
}

server {
    listen 80;
    server_name yourdomain.com;
    return 301 https://$host$request_uri;
}
```

```bash
sudo certbot --nginx -d yourdomain.com
```

---

## 🧬 .env Configuration

```ini
PORT=8600
ENV=production
DB_PATH=database/comms.db
ALLOWED_IPS=127.0.0.1,::1,::ffff:127.0.0.1,192.168.1.0/24
CSRF_KEY=base64-encoded-key

# Service: codcl
WS_SERVICE_NAME=codcl
WS_LANGS=en,ru,es
WS_EMAIL_SUBJECT_EN_PATH=app/templates/codcl/subject_en.txt
WS_EMAIL_BODY_EN_PATH=app/templates/codcl/email_en.txt
WS_TG_MSG_EN_PATH=app/templates/codcl/tg_en.txt
WS_SMTP_USER=your@email
WS_SMTP_PASS=your-password
WS_SMTP_HOST=smtp.example.com
WS_SMTP_PORT=587
WS_FROM_EMAIL=sender@example.com
WS_ADMIN_EMAIL=admin@example.com
WS_TG_BOT_TOKEN=your_telegram_bot_token
WS_TG_CHAT_ID=telegram_chat_id

# Service: stdgi
DH_SERVICE_NAME=stdgi
DH_LANGS=en,fr,es
...
```

You can define any number of services this way.

---

## 📄 Template Structure

Each service has its own folder inside `app/templates/<service_name>`:

```
app/templates/codcl/
├── subject_en.txt
├── email_en.txt
├── tg_en.txt
├── subject_ru.txt
├── email_ru.txt
├── tg_ru.txt
...
```

Supported template types:

* `subject_<lang>.txt` — Email subject line  
* `email_<lang>.txt` — Email HTML/text body  
* `tg_<lang>.txt` — Telegram message  

Template engine supports multiline, and variables like `{{.fullName}}`, `{{.startDate}}`, etc.

---

## ✅ API Testing (via curl)

### 1. Get CSRF Token

```bash
curl -v http://localhost:8600/api/v1/csrf-token -c cookies.txt
```

### 2. Log Cookie Consent

```bash
curl -v -X POST http://localhost:8600/api/v1/cookie-consent \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: <your-token>" \
  -b cookies.txt \
  -d '{
    "serviceName": "codcl",
    "fingerprint": "abc123",
    "userAgent": "curl",
    "ipAddress": "127.0.0.1",
    "accepted": true,
    "timestamp": "2025-07-04T11:00:00Z"
  }'
```

### 3. Submit Order

```bash
curl -v -X POST http://localhost:8600/api/v1/order \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: <your-token>" \
  -H "X-Service-Name: codcl" \
  -b cookies.txt \
  -d '{
    "fullName": "John",
    "contactInfo": "john@example.com",
    "paymentMethod": "card",
    "language": "en",
    "startDate": "2025-07-10",
    "specificationPdf": "<base64-pdf>",
    "invoicePdf": "<base64-pdf>",
    "contractPdf": "<base64-pdf>"
  }'
```

### 4. Health Check

```bash
curl http://localhost:8600/api/v1/health
```

See full testing suite in [`docs/API_TESTS.md`](./docs/API_TESTS.md).

---

## 🧩 Folder Structure

```
axcommutator/
├── app/
│   ├── config/             # App-wide configuration
│   ├── db/                 # SQLite DB connection
│   ├── handlers/           # API handlers
│   ├── storage/temp/       # Temporary files
│   ├── templates/          # Multilingual templates per service
│   └── utils/              # Helper logic (email, Telegram, file utils)
├── database/               # Default DB
├── logs/                   # Logs
├── migrations/             # SQL schema setup
├── Dockerfile
├── docker-compose.yml
├── main.go
├── go.mod / go.sum
├── .env / .env.example
```

---

## 🔒 Security Practices

* CSRF protection using token system  
* IP whitelisting per `.env`  
* DDoS/TDoS mitigation via rate limiting  
* Validations for file type/size/content  
* Consent logging (GDPR-ready)  
* Auto-cleaning of uploaded files from temp directory  
* Use secure SMTP and Telegram tokens  

---

## 🧠 Future Improvements

* Redis-based rate limiter and job queue  
* Admin dashboard UI for viewing logs  
* i18n fallback for undefined languages  
* SMTP failover  

---

## 🧾 License

MIT License.