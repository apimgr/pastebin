const jwt = require('jsonwebtoken');

const redirectIfAuthenticated = async (req, res, next) => {
  try {
    const authHeader = req.headers['authorization'];
    const token = authHeader && authHeader.split(' ')[1];
    
    if (!token) {
      return next();
    }

    const decoded = jwt.verify(token, process.env.JWT_SECRET);
    const { getModels } = require('../models');
    const models = getModels();
    
    let user;
    if (models.User.findByPk) {
      user = await models.User.findByPk(decoded.userId);
    } else {
      user = await models.User.findById(decoded.userId);
    }
    
    if (user) {
      return res.redirect('/auth/dashboard');
    }
    
    next();
  } catch (error) {
    next();
  }
};

const redirectIfNotAuthenticated = async (req, res, next) => {
  try {
    const authHeader = req.headers['authorization'];
    const token = authHeader && authHeader.split(' ')[1];

    if (!token) {
      return res.redirect('/auth/login');
    }

    const decoded = jwt.verify(token, process.env.JWT_SECRET);
    const { getModels } = require('../models');
    const models = getModels();
    
    let user;
    if (models.User.findByPk) {
      user = await models.User.findByPk(decoded.userId);
    } else {
      user = await models.User.findById(decoded.userId);
    }
    
    if (!user) {
      return res.redirect('/auth/login');
    }

    req.user = user;
    next();
  } catch (error) {
    return res.redirect('/auth/login');
  }
};

const checkAuthStatus = async (req, res, next) => {
  try {
    const authHeader = req.headers['authorization'];
    const token = authHeader && authHeader.split(' ')[1];

    if (!token) {
      req.isAuthenticated = false;
      return next();
    }

    const decoded = jwt.verify(token, process.env.JWT_SECRET);
    const { getModels } = require('../models');
    const models = getModels();
    
    let user;
    if (models.User.findByPk) {
      user = await models.User.findByPk(decoded.userId);
    } else {
      user = await models.User.findById(decoded.userId);
    }
    
    if (user) {
      req.user = user;
      req.isAuthenticated = true;
    } else {
      req.isAuthenticated = false;
    }
    
    next();
  } catch (error) {
    req.isAuthenticated = false;
    next();
  }
};

module.exports = {
  redirectIfAuthenticated,
  redirectIfNotAuthenticated,
  checkAuthStatus
};