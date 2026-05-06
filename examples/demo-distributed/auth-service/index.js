const express = require('express');
const mongoose = require('mongoose');

const app = express();
const port = 80;

// Connect to MongoDB
mongoose.connect('mongodb://auth-db:27017/auth', {
  serverSelectionTimeoutMS: 2000 // Keep timeout short for chaos testing
}).then(() => console.log('Connected to MongoDB'))
  .catch(err => console.error('MongoDB connection error:', err));

app.get('/auth/verify', async (req, res) => {
  // Simulate some CPU intensive work to test `limit_cpu` chaos action
  let hash = 0;
  for (let i = 0; i < 10000000; i++) {
    hash += i;
  }

  // Check DB state
  if (mongoose.connection.readyState !== 1) {
    return res.status(503).json({ error: "Auth database unavailable" });
  }

  res.json({ status: "ok", user: "test-user", hash_computed: hash });
});

app.listen(port, () => {
  console.log(`Auth service listening on port ${port}`);
});

