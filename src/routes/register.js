const express = require('express');

const router = express.Router();

router.get('/', (req, res) => {
  res.render('register', { 
    title: 'Register',
    siteTitle: process.env.SITE_TITLE || 'Pastebin',
    error: null,
    username: '',
    email: ''
  });
});

router.post('/', async (req, res) => {
  try {
    const { username, email, password } = req.body;

    if (!username || !email || !password) {
      return res.render('register', { 
        title: 'Register',
        siteTitle: process.env.SITE_TITLE || 'Pastebin',
        error: 'Username, email, and password are required',
        username: username || '',
        email: email || ''
      });
    }

    if (password.length < 6) {
      return res.render('register', { 
        title: 'Register',
        siteTitle: process.env.SITE_TITLE || 'Pastebin',
        error: 'Password must be at least 6 characters long',
        username: username || '',
        email: email || ''
      });
    }

    const { getModels } = require('../models');
    const models = getModels();

    let existingUser;
    if (models.User.findOne) {
      if (models.User.findOne.length > 1) {
        existingUser = await models.User.findOne({
          where: {
            [models.sequelize.Sequelize.Op.or]: [
              { username },
              { email }
            ]
          }
        });
      } else {
        existingUser = await models.User.findOne({
          $or: [{ username }, { email }]
        });
      }
    }

    if (existingUser) {
      return res.render('register', { 
        title: 'Register',
        siteTitle: process.env.SITE_TITLE || 'Pastebin',
        error: 'User already exists',
        username: username || '',
        email: email || ''
      });
    }

    let user;
    if (models.User.create) {
      user = await models.User.create({ username, email, password });
    } else {
      user = new models.User({ username, email, password });
      await user.save();
    }

    const jwt = require('jsonwebtoken');
    const token = jwt.sign(
      { userId: user.id || user._id },
      process.env.JWT_SECRET,
      { expiresIn: process.env.JWT_EXPIRES_IN || '7d' }
    );

    const isApiRequest = req.get('Content-Type')?.includes('application/json') || req.get('Accept')?.includes('application/json');
    
    if (isApiRequest) {
      res.status(201).json({
        message: 'User registered successfully',
        user: {
          id: user.id || user._id,
          username: user.username,
          email: user.email
        },
        token
      });
    } else {
      // Web form - store user in session and redirect to dashboard  
      req.session.user = {
        id: user.id || user._id,
        username: user.username,
        email: user.email
      };
      req.session.token = token;
      res.redirect('/auth/dashboard');
    }
  } catch (error) {
    console.error('Registration error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

module.exports = router;