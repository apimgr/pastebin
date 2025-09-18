const express = require('express');
const authRoutes = require('../../routes/auth');
const pasteRoutes = require('../../routes/pastes');
const uploadRoutes = require('../../routes/upload');

const router = express.Router();

// API Info endpoint
router.get('/', (req, res) => {
  res.json({
    name: 'Pastebin API',
    version: '1.0.0',
    description: 'A pastebin service like pastebin.com with multi-database support',
    endpoints: {
      unified: {
        description: 'Unified create/list endpoints',
        'GET /create': 'List all public pastes (web form)',
        'POST /create': 'Create paste (curl, form, or JSON)',
        'GET /api/v1/create': 'List all public pastes (JSON)',
        'POST /api/v1/create': 'Create paste (JSON)',
        'GET /api/v1/pastes': 'List all public pastes (JSON)',
        'POST /api/v1/pastes': 'Create paste (JSON)'
      },
      auth: {
        register: 'POST /api/v1/auth/register',
        login: 'POST /api/v1/auth/login',
        tokens: 'GET/POST/DELETE /api/v1/auth/tokens',
        me: 'GET /api/v1/auth/me'
      },
      web: {
        home: 'GET /',
        create: 'GET /create (form), POST /create (submit)',
        paste: 'GET /:id',
        raw: 'GET /raw/:id or /r/:id',
        download: 'GET /download/:id'
      }
    },
    examples: {
      curl_text: `curl -X POST --data-binary @file.txt ${req.protocol}://${req.get('host')}/create`,
      curl_file: `curl -X POST -F "files=@file.txt" ${req.protocol}://${req.get('host')}/create`,
      curl_json: `curl -X POST -H "Content-Type: application/json" -d '{"content":"hello world"}' ${req.protocol}://${req.get('host')}/api/v1/create`,
      list_pastes: `curl -H "Accept: application/json" ${req.protocol}://${req.get('host')}/create`
    }
  });
});

// Mount API routes with unified pattern
router.use('/auth', authRoutes);
router.use('/create', require('../../routes/create'));
router.use('/pastes', require('../../routes/create'));

module.exports = router;