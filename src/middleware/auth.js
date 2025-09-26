const jwt = require('jsonwebtoken');

const authenticateToken = async (req, res, next) => {
  try {
    const { models } = require('../models');
    const authHeader = req.headers['authorization'];
    const token = authHeader && authHeader.split(' ')[1];

    if (!token) {
      return res.status(401).json({ error: 'Access token required' });
    }

    const decoded = jwt.verify(token, process.env.JWT_SECRET);
    
    let user;
    if (models.User.findByPk) {
      user = await models.User.findByPk(decoded.userId);
    } else {
      user = await models.User.findById(decoded.userId);
    }
    
    if (!user) {
      return res.status(401).json({ error: 'Invalid token' });
    }

    req.user = user;
    req.isAuthenticated = true;
    next();
  } catch (error) {
    if (error.name === 'TokenExpiredError') {
      return res.status(401).json({ error: 'Token expired' });
    } else if (error.name === 'JsonWebTokenError') {
      return res.status(401).json({ error: 'Invalid token' });
    }
    
    console.error('Auth middleware error:', error);
    return res.status(500).json({ error: 'Internal server error' });
  }
};

const optionalAuth = async (req, res, next) => {
  try {
    const { models } = require('../models');
    const authHeader = req.headers['authorization'];
    const token = authHeader && authHeader.split(' ')[1];

    if (!token) {
      req.user = null;
      req.isAuthenticated = false;
      return next();
    }

    const decoded = jwt.verify(token, process.env.JWT_SECRET);
    
    let user;
    if (models.User.findByPk) {
      user = await models.User.findByPk(decoded.userId);
    } else {
      user = await models.User.findById(decoded.userId);
    }
    
    req.user = user || null;
    req.isAuthenticated = !!user;
    next();
  } catch (error) {
    req.user = null;
    req.isAuthenticated = false;
    next();
  }
};

const checkApiToken = async (req, res, next) => {
  try {
    const { models } = require('../models');
    const authHeader = req.headers['authorization'];
    const token = authHeader && authHeader.split(' ')[1];

    if (!token) {
      req.hasValidApiToken = false;
      return next();
    }

    let apiToken;
    if (models.Token.findOne) {
      if (models.Token.findOne.length > 1) {
        apiToken = await models.Token.findOne({ 
          where: { token, isActive: true },
          include: [{ model: models.User, as: 'user' }]
        });
      } else {
        apiToken = await models.Token.findOne({ token, isActive: true }).populate('userId');
      }
    }
    
    if (apiToken) {
      req.hasValidApiToken = true;
      req.user = apiToken.user || apiToken.userId;
      req.isAuthenticated = true;
    } else {
      req.hasValidApiToken = false;
    }
    
    next();
  } catch (error) {
    console.error('API token check error:', error);
    req.hasValidApiToken = false;
    next();
  }
};

module.exports = {
  authenticateToken,
  optionalAuth,
  checkApiToken
};