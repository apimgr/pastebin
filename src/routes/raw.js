const express = require('express');

const router = express.Router();

const getRawPaste = async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const { id } = req.params;
    const now = new Date();

    let paste;
    if (models.Paste.findByPk) {
      paste = await models.Paste.findByPk(id);
    } else {
      paste = await models.Paste.findOne({ id });
    }

    if (!paste) {
      return res.status(404).send('Paste not found');
    }

    if (paste.expiresAt && paste.expiresAt <= now) {
      return res.status(410).send('Paste has expired');
    }

    if (!paste.isPublic) {
      return res.status(403).send('This paste is private');
    }

    if (models.Paste.increment) {
      await models.Paste.increment('views', { where: { id } });
    } else {
      await models.Paste.findOneAndUpdate({ id }, { $inc: { views: 1 } });
    }

    // Handle different content types
    if (paste.content.startsWith('data:')) {
      // This is a data URL (base64 encoded file)
      const [header, base64Data] = paste.content.split(',');
      const mimeType = header.split(':')[1].split(';')[0];
      const buffer = Buffer.from(base64Data, 'base64');
      
      res.set('Content-Type', mimeType);
      res.send(buffer);
    } else {
      // Regular text content
      res.set('Content-Type', 'text/plain; charset=utf-8');
      res.send(paste.content);
    }
  } catch (error) {
    console.error('Get raw paste error:', error);
    res.status(500).send('Internal server error');
  }
};

router.get('/:id', getRawPaste);

module.exports = router;