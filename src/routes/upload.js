const express = require('express');
const { optionalAuth } = require('../middleware/auth');
const { upload, detectLanguageFromFile } = require('../middleware/upload');
const { generatePasteUrl } = require('../utils/urlHelper');
const { generatePasteId } = require('../utils/pasteId');

const router = express.Router();

const createPasteFromContent = async (content, filename, language, req, isPublic = true, title = null) => {
  const { getModels } = require('../models');
  const models = getModels();
  if (!content || content.trim().length === 0) {
    throw new Error('Content is required');
  }

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
    throw new Error('Failed to generate unique paste ID');
  }

  const pasteData = {
    id: pasteId,
    title: title || filename || 'Untitled',
    content: content.trim(),
    language: language || detectLanguageFromFile(filename, null, content),
    isPublic,
    userId: req.user ? (req.user.id || req.user._id) : null
  };

  let paste;
  if (models.Paste.create) {
    paste = await models.Paste.create(pasteData);
  } else {
    paste = new models.Paste(pasteData);
    await paste.save();
  }

  return paste;
};

router.post('/', optionalAuth, (req, res, next) => {
  const contentType = req.get('Content-Type') || '';
  
  if (contentType.startsWith('text/plain') || contentType.startsWith('application/x-www-form-urlencoded')) {
    let body = '';
    req.on('data', chunk => {
      body += chunk.toString();
    });
    
    req.on('end', async () => {
      try {
        const content = body;
        const filename = req.get('X-Filename') || null;
        const language = req.get('X-Language') || null;
        const title = req.get('X-Title') || null;
        
        const paste = await createPasteFromContent(content, filename, language, req, true, title);
        const pasteUrl = generatePasteUrl(req, paste.id);
        
        if (req.get('Accept') === 'application/json') {
          res.status(201).json({
            id: paste.id,
            title: paste.title,
            language: paste.language,
            link: pasteUrl
          });
        } else {
          res.set('Content-Type', 'text/plain');
          res.status(201).send(pasteUrl);
        }
      } catch (error) {
        console.error('Upload error:', error);
        if (req.get('Accept') === 'application/json') {
          res.status(400).json({ error: error.message });
        } else {
          res.status(400).send(`Error: ${error.message}`);
        }
      }
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
    const files = req.files || [];
    const { title, language, private: isPrivate } = req.body;
    
    if (files.length === 0) {
      return res.status(400).json({ error: 'No files uploaded' });
    }

    if (files.length === 1) {
      const file = files[0];
      const content = file.buffer.toString('utf8');
      const detectedLang = language || detectLanguageFromFile(file.originalname, file.mimetype, content);
      
      const paste = await createPasteFromContent(
        content, 
        file.originalname, 
        detectedLang, 
        req, 
        !isPrivate,
        title
      );
      
      const pasteUrl = generatePasteUrl(req, paste.id);
      
      res.status(201).json({
        id: paste.id,
        title: paste.title,
        language: paste.language,
        link: pasteUrl
      });
    } else {
      const results = [];
      
      for (const file of files) {
        const content = file.buffer.toString('utf8');
        const detectedLang = detectLanguageFromFile(file.originalname, file.mimetype, content);
        
        const paste = await createPasteFromContent(
          content,
          file.originalname,
          detectedLang,
          req,
          !isPrivate,
          file.originalname
        );
        
        const pasteUrl = generatePasteUrl(req, paste.id);
        
        results.push({
          filename: file.originalname,
          id: paste.id,
          title: paste.title,
          language: paste.language,
          link: pasteUrl
        });
      }
      
      res.status(201).json({
        message: `${files.length} files uploaded successfully`,
        pastes: results
      });
    }
  } catch (error) {
    console.error('Multipart upload error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});

module.exports = router;