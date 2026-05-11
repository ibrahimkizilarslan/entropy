# Entropy - Distributed E-Commerce Demo

Welcome to the **Polyglot Distributed E-Commerce Sandbox**. This environment is specifically designed to showcase the power of **Entropy Chaos Engineering CLI** in a realistic, senior-level microservices architecture.

## 🏗️ Architecture

This demo spins up 7 containers across 4 different programming languages and 3 different databases, demonstrating Entropy's technology-agnostic capabilities.

1.  🚪 **`api-gateway` (Go):** The entry point. Implements a Circuit Breaker / Timeout pattern.
2.  📦 **`catalog-service` (.NET 8):** The product catalog. Uses **PostgreSQL** and implements Graceful Degradation.
3.  🛒 **`cart-service` (Python/FastAPI):** Manages user carts. Uses **Redis** and handles connection failures securely.
4.  🔐 **`auth-service` (Node.js/Express):** Handles user sessions. Uses **MongoDB** and is built to withstand CPU starvation.

---

## 🚀 Quick Start

### 1. Start the Environment

Navigate to this folder and use Docker Compose to build and start the entire ecosystem in the background:

```bash
docker-compose up -d --build
```
*Wait a few seconds for PostgreSQL and MongoDB to initialize.*

### 2. Verify Everything is Running

Test the Go API Gateway (which proxies to the .NET Catalog Service):
```bash
curl http://localhost:8085/api/catalog
```
*You should see a 200 OK response originating from the PostgreSQL database.*

### 3. Run Chaos Scenarios

We have prepared 3 specific hypothesis-driven chaos scenarios in the `scenarios/` folder. Run them using Entropy from your host machine!

#### Scenario 1: Redis Crash (Testing Python Cart Resilience)
The Cart Service uses Redis. What happens if Redis crashes? A bad system will return a 500 Internal Server Error. A resilient system will return a 503 Service Unavailable gracefully.
```bash
entropy scenario run scenarios/1-redis-crash.yaml
```

#### Scenario 2: Gateway Timeout (Testing Go Circuit Breaker)
What happens if the .NET Catalog Service becomes extremely slow (CPU Starved)? The Go Gateway shouldn't hang forever. It should trip a timeout and return cached fallback data.
```bash
entropy scenario run scenarios/2-gateway-timeout.yaml
```

#### Scenario 3: Auth CPU Starvation (Testing Node.js Load)
Limits the Node.js Auth Service CPU to 10% and verifies that it can still handle basic health checks, simulating heavy production load.
```bash
entropy scenario run scenarios/3-auth-cpu.yaml
```

---

## 🧹 Cleanup

When you are done playing with the chaos playground, clean up the environment:

```bash
docker-compose down -v
```
