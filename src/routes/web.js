const express = require('express');
const { upload } = require('../middleware/upload');
const { generatePasteId } = require('../utils/pasteId');

const router = express.Router();

router.get('/', async (req, res) => {
  try {
    res.render('home', {
      title: 'Home',
      siteTitle: process.env.SITE_TITLE || 'Pastebin',
      tagline: process.env.SITE_TAGLINE || 'Share text and code snippets instantly',
      baseUrl: req.protocol + '://' + req.get('host'),
      error: req.query.error || null,
      titleValue: req.query.title || '',
      content: req.query.content || '',
      selectedLanguage: req.query.language || '',
      isPrivate: req.query.private === 'on'
    });
  } catch (error) {
    console.error('Web interface error:', error);
    res.status(500).send('Internal server error');
  }
});

// Handle form submissions from home page
router.post('/', upload.array('files', 10), async (req, res) => {
  try {
    const { title, content, language, expiration, private: isPrivate } = req.body;
    const files = req.files || [];
    
    // Check if we have either content or files
    if ((!content || content.trim().length === 0) && files.length === 0) {
      return res.redirect(`/?error=Please provide content or upload files&title=${encodeURIComponent(title || '')}&language=${encodeURIComponent(language || '')}&private=${isPrivate ? 'on' : ''}`);
    }
    
    // Handle file uploads
    if (files.length > 0) {
      try {
        const { detectLanguageFromFile } = require('../middleware/upload');
        
        if (files.length === 1) {
          // Single file upload
          const file = files[0];
          let fileContent;
          let mimeType = file.mimetype || 'application/octet-stream';
          
          // Handle different file types
          if (mimeType.startsWith('image/') || mimeType.startsWith('video/') || mimeType.startsWith('audio/')) {
            // For media files, store as base64
            fileContent = `data:${mimeType};base64,${file.buffer.toString('base64')}`;
          } else if (mimeType.startsWith('text/') || 
                     mimeType.includes('json') || 
                     mimeType.includes('xml') || 
                     mimeType.includes('javascript') ||
                     !mimeType ||
                     mimeType === 'application/octet-stream') {
            // For text files, store as text
            fileContent = file.buffer.toString('utf8');
          } else {
            // For other binary files, store as base64 with MIME info
            fileContent = `data:${mimeType};base64,${file.buffer.toString('base64')}`;
          }
          
          const detectedLang = language || detectLanguageFromFile(file.originalname, file.mimetype, fileContent);
          
          const paste = await createPaste(fileContent, file.originalname, detectedLang, req, !isPrivate, title || file.originalname, mimeType);
          return res.redirect(`/${paste.id}`);
        } else {
          // Multiple files - redirect to a success page showing all created pastes
          const results = [];
          for (const file of files) {
            const fileContent = file.buffer.toString('utf8');
            const detectedLang = detectLanguageFromFile(file.originalname, file.mimetype, fileContent);
            const paste = await createPaste(fileContent, file.originalname, detectedLang, req, !isPrivate, file.originalname);
            results.push({ filename: file.originalname, id: paste.id });
          }
          const successMessage = `${files.length} files uploaded: ${results.map(r => `${r.filename} (${r.id})`).join(', ')}`;
          return res.redirect(`/?success=${encodeURIComponent(successMessage)}`);
        }
      } catch (error) {
        console.error('File upload error:', error);
        return res.redirect(`/?error=Failed to upload files&title=${encodeURIComponent(title || '')}&language=${encodeURIComponent(language || '')}&private=${isPrivate ? 'on' : ''}`);
      }
    }

    // Use the same logic as create route
    const { getModels } = require('../models');
    const models = getModels();
    const { generatePasteId } = require('../utils/pasteId');
    
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
      return res.redirect('/?error=Failed to generate unique paste ID');
    }

    const pasteData = {
      id: pasteId,
      title: title || 'Untitled',
      content: content.trim(),
      language: language || 'text',
      isPublic: !isPrivate,
      userId: req.user ? (req.user.id || req.user._id) : null
    };

    let paste;
    if (models.Paste.create) {
      paste = await models.Paste.create(pasteData);
    } else {
      paste = new models.Paste(pasteData);
      await paste.save();
    }

    res.redirect(`/${paste.id}`);
  } catch (error) {
    console.error('Web form error:', error);
    res.redirect('/?error=Failed to create paste');
  }
});

async function createPaste(content, filename, language, req, isPublic = true, title = null, mimeType = 'text/plain') {
  const { getModels } = require('../models');
  const models = getModels();
  
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
    content: content,
    language: language || 'text',
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
}

module.exports = router;