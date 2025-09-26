const express = require('express');
const jwt = require('jsonwebtoken');
const { v4: uuidv4 } = require('uuid');
const { authenticateToken } = require('../middleware/auth');

const router = express.Router();

router.post('/register', async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const { username, email, password } = req.body;

    if (!username || !email || !password) {
      return res.status(400).json({ 
        error: 'Username, email, and password are required' 
      });
    }

    if (password.length < 6) {
      return res.status(400).json({ 
        error: 'Password must be at least 6 characters long' 
      });
    }

    let existingUser;
    if (models.User.findOne) {
      try {
        if (models.sequelize && models.sequelize.Sequelize) {
          const { Op } = models.sequelize.Sequelize;
          existingUser = await models.User.findOne({
            where: {
              [Op.or]: [
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
      } catch (error) {
        // Try simple checks
        existingUser = await models.User.findOne({ where: { username } }) ||
                      await models.User.findOne({ where: { email } });
      }
    }

    if (existingUser) {
      return res.status(409).json({ 
        error: 'User already exists' 
      });
    }

    let user;
    if (models.User.create) {
      user = await models.User.create({ username, email, password });
    } else {
      user = new models.User({ username, email, password });
      await user.save();
    }

    const token = jwt.sign(
      { userId: user.id || user._id },
      process.env.JWT_SECRET,
      { expiresIn: process.env.JWT_EXPIRES_IN || '7d' }
    );

    res.status(201).json({
      message: 'User registered successfully',
      user: {
        id: user.id || user._id,
        username: user.username,
        email: user.email
      },
      token
    });
  } catch (error) {
    console.error('Registration error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.post('/login', async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const { username, password } = req.body;

    if (!username || !password) {
      return res.status(400).json({ 
        error: 'Username and password are required' 
      });
    }

    let user;
    if (models.User.findOne) {
      if (models.User.findOne.length > 1) {
        user = await models.User.findOne({
          where: {
            [models.sequelize.Sequelize.Op.or]: [
              { username },
              { email: username }
            ]
          }
        });
      } else {
        user = await models.User.findOne({
          $or: [{ username }, { email: username }]
        });
      }
    }

    if (!user || !(await user.comparePassword(password))) {
      return res.status(401).json({ error: 'Invalid credentials' });
    }

    const token = jwt.sign(
      { userId: user.id || user._id },
      process.env.JWT_SECRET,
      { expiresIn: process.env.JWT_EXPIRES_IN || '7d' }
    );

    res.json({
      message: 'Login successful',
      user: {
        id: user.id || user._id,
        username: user.username,
        email: user.email
      },
      token
    });
  } catch (error) {
    console.error('Login error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.post('/tokens', authenticateToken, async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const { name } = req.body;
    const tokenValue = uuidv4();

    let apiToken;
    if (models.Token.create) {
      apiToken = await models.Token.create({
        name: name || 'Default Token',
        token: tokenValue,
        userId: req.user.id || req.user._id
      });
    } else {
      apiToken = new models.Token({
        name: name || 'Default Token',
        token: tokenValue,
        userId: req.user._id || req.user.id
      });
      await apiToken.save();
    }

    res.status(201).json({
      message: 'API token created successfully',
      token: {
        id: apiToken.id || apiToken._id,
        name: apiToken.name,
        token: apiToken.token,
        createdAt: apiToken.createdAt
      }
    });
  } catch (error) {
    console.error('Token creation error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.get('/tokens', authenticateToken, async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    let tokens;
    if (models.Token.findAll) {
      tokens = await models.Token.findAll({
        where: { userId: req.user.id },
        attributes: ['id', 'name', 'token', 'isActive', 'createdAt']
      });
    } else {
      tokens = await models.Token.find({ userId: req.user._id })
        .select('_id name token isActive createdAt');
    }

    res.json({ tokens });
  } catch (error) {
    console.error('Get tokens error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.delete('/tokens/:tokenId', authenticateToken, async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const { tokenId } = req.params;

    let result;
    if (models.Token.destroy) {
      result = await models.Token.destroy({
        where: { 
          id: tokenId, 
          userId: req.user.id 
        }
      });
    } else {
      result = await models.Token.findOneAndDelete({
        _id: tokenId,
        userId: req.user._id
      });
    }

    if (!result || (typeof result === 'number' && result === 0)) {
      return res.status(404).json({ error: 'Token not found' });
    }

    res.json({ message: 'Token deleted successfully' });
  } catch (error) {
    console.error('Delete token error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.get('/me', authenticateToken, async (req, res) => {
  try {
    res.json({
      user: {
        id: req.user.id || req.user._id,
        username: req.user.username,
        email: req.user.email,
        createdAt: req.user.createdAt
      }
    });
  } catch (error) {
    console.error('Get user error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

module.exports = router;