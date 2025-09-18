const express = require('express');
const hljs = require('highlight.js');

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
      return res.status(404).send(`
        <!DOCTYPE html>
        <html>
        <head>
            <title>Paste Not Found</title>
            <style>body { font-family: monospace; padding: 20px; }</style>
        </head>
        <body>
            <h1>404 - Paste Not Found</h1>
            <p>The paste you're looking for doesn't exist.</p>
        </body>
        </html>
      `);
    }

    if (paste.expiresAt && paste.expiresAt <= now) {
      return res.status(410).send(`
        <!DOCTYPE html>
        <html>
        <head>
            <title>Paste Expired</title>
            <style>body { font-family: monospace; padding: 20px; }</style>
        </head>
        <body>
            <h1>410 - Paste Expired</h1>
            <p>This paste has expired and is no longer available.</p>
        </body>
        </html>
      `);
    }

    if (!paste.isPublic) {
      return res.status(403).send(`
        <!DOCTYPE html>
        <html>
        <head>
            <title>Private Paste</title>
            <style>body { font-family: monospace; padding: 20px; }</style>
        </head>
        <body>
            <h1>403 - Access Denied</h1>
            <p>This paste is private.</p>
        </body>
        </html>
      `);
    }

    if (models.Paste.increment) {
      await models.Paste.increment('views', { where: { id } });
    } else {
      await models.Paste.findOneAndUpdate({ id }, { $inc: { views: 1 } });
    }

    let highlightedCode;
    let detectedLanguage = paste.language || 'text';

    try {
      if (paste.language && paste.language !== 'text' && hljs.getLanguage(paste.language)) {
        highlightedCode = hljs.highlight(paste.content, { language: paste.language }).value;
      } else {
        const result = hljs.highlightAuto(paste.content);
        highlightedCode = result.value;
        detectedLanguage = result.language || 'text';
      }
    } catch (error) {
      highlightedCode = paste.content.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
      detectedLanguage = 'text';
    }

    const html = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>${paste.title} - Pastebin</title>
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github-dark.min.css">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', sans-serif;
            margin: 0;
            padding: 20px;
            background-color: #0d1117;
            color: #f0f6fc;
            line-height: 1.6;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        .header {
            border-bottom: 1px solid #30363d;
            padding-bottom: 20px;
            margin-bottom: 20px;
        }
        .title {
            font-size: 24px;
            font-weight: 600;
            margin: 0 0 10px 0;
            color: #f0f6fc;
        }
        .meta {
            font-size: 14px;
            color: #8b949e;
            display: flex;
            gap: 20px;
            flex-wrap: wrap;
        }
        .meta-item {
            display: flex;
            align-items: center;
            gap: 5px;
        }
        .code-container {
            background-color: #161b22;
            border: 1px solid #30363d;
            border-radius: 6px;
            overflow: hidden;
        }
        .code-header {
            background-color: #21262d;
            padding: 12px 16px;
            border-bottom: 1px solid #30363d;
            display: flex;
            justify-content: space-between;
            align-items: center;
            font-size: 14px;
            color: #8b949e;
        }
        .language-tag {
            background-color: #0969da;
            color: white;
            padding: 2px 8px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 500;
        }
        .code-content {
            padding: 16px;
            overflow-x: auto;
        }
        .hljs {
            background-color: transparent !important;
            padding: 0 !important;
        }
        .line-numbers {
            border-right: 1px solid #30363d;
            padding-right: 12px;
            margin-right: 12px;
            color: #6e7681;
            user-select: none;
            min-width: 30px;
            text-align: right;
        }
        .actions {
            margin-top: 20px;
            display: flex;
            gap: 10px;
            flex-wrap: wrap;
        }
        .btn {
            background-color: #21262d;
            border: 1px solid #30363d;
            color: #f0f6fc;
            padding: 8px 16px;
            border-radius: 6px;
            text-decoration: none;
            font-size: 14px;
            transition: background-color 0.2s;
        }
        .btn:hover {
            background-color: #30363d;
        }
        .btn-primary {
            background-color: #238636;
            border-color: #238636;
        }
        .btn-primary:hover {
            background-color: #2ea043;
        }
        pre {
            margin: 0;
            font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
            font-size: 14px;
            line-height: 1.45;
        }
        @media (max-width: 768px) {
            body { padding: 10px; }
            .meta { flex-direction: column; gap: 10px; }
            .actions { flex-direction: column; }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1 class="title">${paste.title}</h1>
            <div class="meta">
                <div class="meta-item">
                    <span>📅</span>
                    <span>${new Date(paste.createdAt).toLocaleString()}</span>
                </div>
                <div class="meta-item">
                    <span>👁️</span>
                    <span>${paste.views + 1} views</span>
                </div>
                <div class="meta-item">
                    <span>🔗</span>
                    <span>${id}</span>
                </div>
                ${paste.expiresAt ? `
                <div class="meta-item">
                    <span>⏰</span>
                    <span>Expires: ${new Date(paste.expiresAt).toLocaleString()}</span>
                </div>
                ` : ''}
            </div>
        </div>
        
        <div class="code-container">
            <div class="code-header">
                <span>${paste.content.split('\\n').length} lines</span>
                <span class="language-tag">${detectedLanguage}</span>
            </div>
            <div class="code-content">
                <pre><code class="hljs">${highlightedCode}</code></pre>
            </div>
        </div>
        
        <div class="actions">
            <a href="/raw/${id}" class="btn btn-primary">Raw</a>
            <a href="/download/${id}" class="btn">Download</a>
            <a href="/" class="btn">New Paste</a>
        </div>
    </div>
</body>
</html>`;

    res.send(html);
  } catch (error) {
    console.error('Highlight paste error:', error);
    res.status(500).send(`
      <!DOCTYPE html>
      <html>
      <head>
          <title>Server Error</title>
          <style>body { font-family: monospace; padding: 20px; }</style>
      </head>
      <body>
          <h1>500 - Internal Server Error</h1>
          <p>Something went wrong while loading this paste.</p>
      </body>
      </html>
    `);
  }
});

module.exports = router;