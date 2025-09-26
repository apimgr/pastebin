const express = require('express');
const mime = require('mime-types');

const router = express.Router();

router.get('/:id', async (req, res) => {
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

    const languageExtMap = {
      'javascript': 'js',
      'typescript': 'ts',
      'python': 'py',
      'ruby': 'rb',
      'java': 'java',
      'c': 'c',
      'cpp': 'cpp',
      'csharp': 'cs',
      'php': 'php',
      'go': 'go',
      'rust': 'rs',
      'bash': 'sh',
      'shell': 'sh',
      'powershell': 'ps1',
      'sql': 'sql',
      'html': 'html',
      'css': 'css',
      'scss': 'scss',
      'sass': 'sass',
      'less': 'less',
      'xml': 'xml',
      'json': 'json',
      'yaml': 'yml',
      'toml': 'toml',
      'ini': 'ini',
      'markdown': 'md',
      'latex': 'tex',
      'r': 'r',
      'swift': 'swift',
      'kotlin': 'kt',
      'scala': 'scala',
      'clojure': 'clj',
      'haskell': 'hs',
      'ocaml': 'ml',
      'fsharp': 'fs',
      'erlang': 'erl',
      'elixir': 'ex',
      'lua': 'lua',
      'perl': 'pl',
      'vim': 'vim',
      'dockerfile': 'dockerfile'
    };

    // Handle different content types
    if (paste.content.startsWith('data:')) {
      // This is a data URL (base64 encoded file)
      const [header, base64Data] = paste.content.split(',');
      const mimeType = header.split(':')[1].split(';')[0];
      const buffer = Buffer.from(base64Data, 'base64');
      
      // Get extension from MIME type or use original filename
      let extension = mime.extension(mimeType) || 'bin';
      const filename = paste.title.replace(/[^a-zA-Z0-9.-]/g, '_') + '.' + extension;
      
      res.set({
        'Content-Type': mimeType,
        'Content-Disposition': `attachment; filename="${filename}"`,
        'Content-Length': buffer.length,
        'Cache-Control': 'public, max-age=3600'
      });

      res.send(buffer);
    } else {
      // Regular text content
      const extension = languageExtMap[paste.language] || 'txt';
      const filename = paste.title.replace(/[^a-zA-Z0-9.-]/g, '_') + '.' + extension;
      
      const mimeType = mime.lookup(extension) || 'text/plain';

      res.set({
        'Content-Type': mimeType,
        'Content-Disposition': `attachment; filename="${filename}"`,
        'Content-Length': Buffer.byteLength(paste.content, 'utf8'),
        'Cache-Control': 'public, max-age=3600'
      });

      res.send(paste.content);
    }
  } catch (error) {
    console.error('Download paste error:', error);
    res.status(500).send('Internal server error');
  }
});

module.exports = router;