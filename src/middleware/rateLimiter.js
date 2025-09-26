const rateLimit = require('express-rate-limit');
const { checkApiToken } = require('./auth');

const createRateLimiter = () => {
  return rateLimit({
    windowMs: parseInt(process.env.RATE_LIMIT_WINDOW_MS) || 60 * 60 * 1000,
    max: (req) => {
      if (req.hasValidApiToken) {
        return 0;
      }
      return parseInt(process.env.RATE_LIMIT_MAX) || 900;
    },
    message: {
      error: 'Too many requests, please try again later',
      resetTime: new Date(Date.now() + (parseInt(process.env.RATE_LIMIT_WINDOW_MS) || 60 * 60 * 1000))
    },
    standardHeaders: true,
    legacyHeaders: false,
    trustProxy: false,
    skip: (req) => {
      return req.hasValidApiToken;
    }
  });
};

const rateLimitWithTokenCheck = [checkApiToken, createRateLimiter()];

module.exports = {
  createRateLimiter,
  rateLimitWithTokenCheck
};