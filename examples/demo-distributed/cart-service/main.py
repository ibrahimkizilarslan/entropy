from fastapi import FastAPI, HTTPException
import redis
import os

app = FastAPI()

# Connect to Redis
# In a real app, you'd use a connection pool and async redis
redis_client = redis.Redis(host='cart-db', port=6379, db=0, socket_timeout=1)

@app.get("/cart")
def get_cart():
    try:
        # Attempt to read from Redis
        # If cart-db is stopped via Chaos, this will raise a ConnectionError
        redis_client.ping()
        return {"status": "ok", "source": "redis", "items": ["Product A", "Product B"]}
    except redis.exceptions.ConnectionError:
        # Graceful handling: Tell the user the service is degraded instead of a raw 500
        # Gateway could interpret this 503 and show a friendly UI
        raise HTTPException(status_code=503, detail="Cart database is temporarily unavailable due to Chaos!")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=80)
