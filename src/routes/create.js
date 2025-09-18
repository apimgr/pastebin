const express = require('express');
const { optionalAuth } = require('../middleware/auth');
const { upload, detectLanguageFromFile } = require('../middleware/upload');
const { generatePasteUrl } = require('../utils/urlHelper');
const { generatePasteId } = require('../utils/pasteId');

const router = express.Router();

// GET /create - List pastes (JSON only, API endpoint)
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
      const { Op } = models.sequelize.Sequelize;
      const result = await models.Paste.findAndCountAll({
        where: {
          isPublic: true,
          [Op.or]: [
            { expiresAt: null },
            { expiresAt: { [Op.gt]: now } }
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

// POST /create - Create paste (supports multiple content types and Accept headers)
router.post('/', optionalAuth, (req, res, next) => {
  const contentType = req.get('Content-Type') || 'text/plain';
  
  if (contentType.startsWith('text/plain') || (!req.get('Content-Type') && !contentType.includes('multipart'))) {
    // Handle raw text upload
    let body = '';
    req.on('data', chunk => {
      body += chunk.toString();
    });
    
    req.on('end', async () => {
      req.rawBody = body;
      next();
    });
    
    req.on('error', (error) => {
      console.error('Upload stream error:', error);
      res.status(400).send('Upload failed');
    });
  } else {
    next();
  }
}, upload.array('files', 10), async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    
    let content, title, language;
    const contentType = req.get('Content-Type') || 'text/plain';
    const files = req.files || [];
    
    // Handle different content types
    if (files.length > 0) {
      // File upload via multipart form
      const file = files[0];
      const { detectLanguageFromFile } = require('../middleware/upload');
      
      if (file.mimetype.startsWith('image/') || file.mimetype.startsWith('video/') || file.mimetype.startsWith('audio/')) {
        content = `data:${file.mimetype};base64,${file.buffer.toString('base64')}`;
      } else {
        content = file.buffer.toString('utf8');
      }
      
      title = file.originalname;
      language = detectLanguageFromFile(file.originalname, file.mimetype, content);
    } else if (contentType.includes('application/json')) {
      // JSON API request
      const body = req.body;
      content = body.content;
      title = body.title;
      language = body.language;
    } else if (req.rawBody) {
      // Raw text upload (curl --data-binary)
      content = req.rawBody;
      title = req.get('X-Title') || null;
      language = req.get('X-Language') || null;
    } else {
      // Handle form data or other content types
      if (req.body && typeof req.body === 'object') {
        // If it's form data, extract the actual content from the keys
        const keys = Object.keys(req.body);
        if (keys.length > 0) {
          // Find the key that contains the actual content (longest key that's not a form field)
          const contentKey = keys.find(key => key.includes('\n') || key.length > 50) || keys[0];
          content = contentKey;
          title = req.body.title || req.get('X-Title') || null;
          language = req.body.language || req.get('X-Language') || null;
        } else {
          content = JSON.stringify(req.body);
        }
      } else {
        content = typeof req.body === 'string' ? req.body : String(req.body || '');
        title = req.get('X-Title') || null;
        language = req.get('X-Language') || null;
      }
    }

    if (!content || content.trim().length === 0) {
      const accept = req.get('Accept') || 'text/plain';
      if (accept.includes('application/json')) {
        return res.status(400).json({ error: 'Content is required' });
      } else {
        return res.status(400).send('Error: Content is required');
      }
    }

    // Generate unique paste ID
    let pasteId;
    let attempts = 0;
    const maxAttempts = 10;

    do {
      pasteId = generatePasteId();
      attempts++;
      
      let existingPaste;
      if (models.Paste.findByPk) {
        existingPaste = await models.Paste.findByPk(pasteId);
      } else {
        existingPaste = await models.Paste.findOne({ id: pasteId });
      }
      
      if (!existingPaste) break;
    } while (attempts < maxAttempts);

    if (attempts >= maxAttempts) {
      const accept = req.get('Accept') || 'text/plain';
      if (accept.includes('application/json')) {
        return res.status(500).json({ error: 'Failed to generate unique paste ID' });
      } else {
        return res.status(500).send('Error: Failed to generate unique paste ID');
      }
    }

    // Create paste
    const pasteData = {
      id: pasteId,
      title: title || 'Untitled',
      content: typeof content === 'string' ? content.trim() : String(content),
      language: language || 'text',
      isPublic: true,
      userId: req.user ? (req.user.id || req.user._id) : null
    };

    let paste;
    if (models.Paste.create) {
      paste = await models.Paste.create(pasteData);
    } else {
      paste = new models.Paste(pasteData);
      await paste.save();
    }

    // Return response based on Accept header
    const accept = req.get('Accept') || 'text/plain';
    const pasteUrl = `${req.protocol}://${req.get('host')}/${paste.id}`;
    
    if (accept.includes('application/json')) {
      res.status(201).json({
        id: paste.id,
        title: paste.title,
        language: paste.language,
        isPublic: paste.isPublic,
        createdAt: paste.createdAt,
        link: pasteUrl
      });
    } else {
      res.set('Content-Type', 'text/plain');
      res.status(201).send(pasteUrl);
    }

  } catch (error) {
    console.error('Create paste error:', error);
    const accept = req.get('Accept') || 'text/plain';
    if (accept.includes('application/json')) {
      res.status(500).json({ error: 'Internal server error' });
    } else {
      res.status(500).send('Error: Internal server error');
    }
  }
});

module.exports = router;