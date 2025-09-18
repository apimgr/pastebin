const express = require('express');
const { optionalAuth } = require('../middleware/auth');
const { generatePasteUrl } = require('../utils/urlHelper');
const { generatePasteId } = require('../utils/pasteId');

const router = express.Router();

router.post('/', optionalAuth, async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const { title, content, language, isPublic, expiresIn } = req.body;

    if (!content || content.trim().length === 0) {
      return res.status(400).json({ error: 'Content is required' });
    }

    let pasteId;
    let attempts = 0;
    const maxAttempts = 10;

    do {
      pasteId = generatePasteId();
      attempts++;
      
      const { getModels } = require('../models');
      const models = getModels();
      let existingPaste;
      if (models.Paste.findByPk) {
        existingPaste = await models.Paste.findByPk(pasteId);
      } else {
        existingPaste = await models.Paste.findOne({ id: pasteId });
      }
      
      if (!existingPaste) break;
    } while (attempts < maxAttempts);

    if (attempts >= maxAttempts) {
      return res.status(500).json({ error: 'Failed to generate unique paste ID' });
    }

    let expiresAt = null;
    if (expiresIn) {
      const now = new Date();
      const expirationMs = parseInt(expiresIn) * 60 * 1000;
      expiresAt = new Date(now.getTime() + expirationMs);
    }

    const pasteData = {
      id: pasteId,
      title: title || 'Untitled',
      content: content.trim(),
      language: language || 'text',
      isPublic: isPublic !== undefined ? isPublic : true,
      expiresAt,
      userId: req.user ? (req.user.id || req.user._id) : null
    };

    let paste;
    if (models.Paste.create) {
      paste = await models.Paste.create(pasteData);
    } else {
      paste = new models.Paste(pasteData);
      await paste.save();
    }

    const pasteUrl = generatePasteUrl(req, pasteId);

    res.status(201).json({
      id: paste.id,
      title: paste.title,
      language: paste.language,
      isPublic: paste.isPublic,
      expiresAt: paste.expiresAt,
      createdAt: paste.createdAt,
      link: pasteUrl
    });
  } catch (error) {
    console.error('Create paste error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.get('/', async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const page = parseInt(req.query.page) || 1;
    const limit = Math.min(parseInt(req.query.limit) || 20, 100);
    const offset = (page - 1) * limit;

    const now = new Date();

    let pastes, total;
    if (models.Paste.findAndCountAll) {
      const result = await models.Paste.findAndCountAll({
        where: {
          isPublic: true,
          [models.sequelize.Sequelize.Op.or]: [
            { expiresAt: null },
            { expiresAt: { [models.sequelize.Sequelize.Op.gt]: now } }
          ]
        },
        attributes: ['id', 'title', 'language', 'createdAt', 'views'],
        order: [['createdAt', 'DESC']],
        limit,
        offset
      });
      pastes = result.rows;
      total = result.count;
    } else {
      const query = {
        isPublic: true,
        $or: [
          { expiresAt: null },
          { expiresAt: { $gt: now } }
        ]
      };
      
      total = await models.Paste.countDocuments(query);
      pastes = await models.Paste.find(query)
        .select('id title language createdAt views')
        .sort({ createdAt: -1 })
        .limit(limit)
        .skip(offset);
    }

    const totalPages = Math.ceil(total / limit);

    res.json({
      pastes,
      pagination: {
        page,
        limit,
        total,
        totalPages,
        hasNext: page < totalPages,
        hasPrev: page > 1
      }
    });
  } catch (error) {
    console.error('Get pastes error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.get('/:id', optionalAuth, async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const { id } = req.params;
    const now = new Date();

    let paste;
    if (models.Paste.findByPk) {
      paste = await models.Paste.findByPk(id, {
        include: req.user ? [] : undefined
      });
    } else {
      paste = await models.Paste.findOne({ id });
    }

    if (!paste) {
      return res.status(404).json({ error: 'Paste not found' });
    }

    if (paste.expiresAt && paste.expiresAt <= now) {
      return res.status(410).json({ error: 'Paste has expired' });
    }

    if (!paste.isPublic) {
      if (!req.user || (req.user.id || req.user._id).toString() !== (paste.userId || '').toString()) {
        return res.status(403).json({ error: 'Access denied' });
      }
    }

    if (models.Paste.increment) {
      await models.Paste.increment('views', { where: { id } });
    } else {
      await models.Paste.findOneAndUpdate({ id }, { $inc: { views: 1 } });
    }

    res.json({
      id: paste.id,
      title: paste.title,
      content: paste.content,
      language: paste.language,
      isPublic: paste.isPublic,
      views: paste.views + 1,
      expiresAt: paste.expiresAt,
      createdAt: paste.createdAt,
      updatedAt: paste.updatedAt
    });
  } catch (error) {
    console.error('Get paste error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.put('/:id', optionalAuth, async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const { id } = req.params;
    const { title, content, language, isPublic } = req.body;

    if (!req.user) {
      return res.status(401).json({ error: 'Authentication required' });
    }

    let paste;
    if (models.Paste.findByPk) {
      paste = await models.Paste.findByPk(id);
    } else {
      paste = await models.Paste.findOne({ id });
    }

    if (!paste) {
      return res.status(404).json({ error: 'Paste not found' });
    }

    if ((req.user.id || req.user._id).toString() !== (paste.userId || '').toString()) {
      return res.status(403).json({ error: 'Access denied' });
    }

    const updateData = {};
    if (title !== undefined) updateData.title = title;
    if (content !== undefined) updateData.content = content;
    if (language !== undefined) updateData.language = language;
    if (isPublic !== undefined) updateData.isPublic = isPublic;

    if (models.Paste.update) {
      await models.Paste.update(updateData, { where: { id } });
      paste = await models.Paste.findByPk(id);
    } else {
      paste = await models.Paste.findOneAndUpdate({ id }, updateData, { new: true });
    }

    res.json({
      id: paste.id,
      title: paste.title,
      content: paste.content,
      language: paste.language,
      isPublic: paste.isPublic,
      views: paste.views,
      expiresAt: paste.expiresAt,
      createdAt: paste.createdAt,
      updatedAt: paste.updatedAt
    });
  } catch (error) {
    console.error('Update paste error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.delete('/:id', optionalAuth, async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const { id } = req.params;

    if (!req.user) {
      return res.status(401).json({ error: 'Authentication required' });
    }

    let paste;
    if (models.Paste.findByPk) {
      paste = await models.Paste.findByPk(id);
    } else {
      paste = await models.Paste.findOne({ id });
    }

    if (!paste) {
      return res.status(404).json({ error: 'Paste not found' });
    }

    if ((req.user.id || req.user._id).toString() !== (paste.userId || '').toString()) {
      return res.status(403).json({ error: 'Access denied' });
    }

    if (models.Paste.destroy) {
      await models.Paste.destroy({ where: { id } });
    } else {
      await models.Paste.findOneAndDelete({ id });
    }

    res.json({ message: 'Paste deleted successfully' });
  } catch (error) {
    console.error('Delete paste error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.get('/user/:userId', optionalAuth, async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const { userId } = req.params;
    const page = parseInt(req.query.page) || 1;
    const limit = Math.min(parseInt(req.query.limit) || 20, 100);
    const offset = (page - 1) * limit;

    if (!req.user || (req.user.id || req.user._id).toString() !== userId) {
      return res.status(403).json({ error: 'Access denied' });
    }

    let pastes, total;
    if (models.Paste.findAndCountAll) {
      const result = await models.Paste.findAndCountAll({
        where: { userId },
        attributes: ['id', 'title', 'language', 'isPublic', 'views', 'createdAt'],
        order: [['createdAt', 'DESC']],
        limit,
        offset
      });
      pastes = result.rows;
      total = result.count;
    } else {
      total = await models.Paste.countDocuments({ userId });
      pastes = await models.Paste.find({ userId })
        .select('id title language isPublic views createdAt')
        .sort({ createdAt: -1 })
        .limit(limit)
        .skip(offset);
    }

    const totalPages = Math.ceil(total / limit);

    res.json({
      pastes,
      pagination: {
        page,
        limit,
        total,
        totalPages,
        hasNext: page < totalPages,
        hasPrev: page > 1
      }
    });
  } catch (error) {
    console.error('Get user pastes error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

module.exports = router;