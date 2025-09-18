const express = require('express');
const cors = require('cors');
const helmet = require('helmet');
const morgan = require('morgan');
const session = require('express-session');
require('dotenv').config();

const dbManager = require('./config/database');
const { initializeModels } = require('./models');
const { rateLimitWithTokenCheck } = require('./middleware/rateLimiter');
const { addUserContext } = require('./middleware/sessionAuth');

const authRoutes = require('./routes/auth');
const pasteRoutes = require('./routes/pastes');
const uploadRoutes = require('./routes/upload');
const rawRoutes = require('./routes/raw');
const highlightRoutes = require('./routes/highlight');
const downloadRoutes = require('./routes/download');
const webRoutes = require('./routes/web');
const recentRoutes = require('./routes/recent');
const registerRoutes = require('./routes/register');
const createRoutes = require('./routes/create');
const apiV1Routes = require('./api/v1');

const app = express();

app.set('trust proxy', 1);
app.set('view engine', 'ejs');
app.set('views', __dirname + '/../views');

app.use(helmet({
  crossOriginEmbedderPolicy: false,
  contentSecurityPolicy: {
    directives: {
      defaultSrc: ["'self'"],
      styleSrc: ["'self'", "'unsafe-inline'"],
      scriptSrc: ["'self'"],
      imgSrc: ["'self'", "data:", "https:"],
    },
  },
}));

app.use(cors({
  origin: '*',
  methods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS'],
  allowedHeaders: ['Content-Type', 'Authorization'],
  credentials: false
}));

app.use(morgan(process.env.NODE_ENV === 'production' ? 'combined' : 'dev'));

app.use(express.json({ limit: '10mb' }));
app.use(express.urlencoded({ extended: true, limit: '10mb' }));

// Session support for web authentication
app.use(session({
  secret: process.env.JWT_SECRET || 'session-secret-change-in-production',
  resave: false,
  saveUninitialized: false,
  cookie: {
    secure: process.env.NODE_ENV === 'production',
    httpOnly: true,
    maxAge: 7 * 24 * 60 * 60 * 1000 // 7 days
  }
}));

app.use(rateLimitWithTokenCheck);

// Add user context to all requests
app.use(addUserContext);

app.get('/api', (req, res) => {
  res.redirect('/api/v1');
});

app.get('/healthz', (req, res) => {
  res.json({ 
    status: 'ok', 
    timestamp: new Date().toISOString(),
    database: dbManager.getType()
  });
});

app.use('/raw', rawRoutes);
app.use('/r', rawRoutes);
app.use('/download', downloadRoutes);
app.get('/auth', (req, res) => {
  res.redirect('/auth/login');
});

app.use('/api/v1', apiV1Routes);
app.use('/auth', authRoutes);
app.use('/pastes', pasteRoutes);
app.use('/recent', recentRoutes);
app.use('/create', createRoutes);
app.use('/auth/register', registerRoutes);
app.use('/auth/login', require('./routes/login'));
app.use('/auth/dashboard', require('./routes/dashboard'));
app.use('/auth', require('./routes/dashboard'));
app.use('/', webRoutes);
app.use('/', uploadRoutes);

app.get('/:id', async (req, res) => {
  try {
    const { getModels } = require('./models');
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
      return res.redirect('/create');
    }

    if (paste.expiresAt && paste.expiresAt <= now) {
      return res.status(410).send(`
        <!DOCTYPE html>
        <html>
        <head>
            <title>Paste Expired - Pastebin</title>
            <style>
              body { font-family: monospace; padding: 40px; background: #0d1117; color: #f0f6fc; text-align: center; }
              h1 { color: #f85149; }
              a { color: #58a6ff; text-decoration: none; }
            </style>
        </head>
        <body>
            <h1>410 - Paste Expired</h1>
            <p>This paste has expired and is no longer available.</p>
            <a href="/">← Create a new paste</a>
        </body>
        </html>
      `);
    }

    if (!paste.isPublic) {
      return res.status(403).send(`
        <!DOCTYPE html>
        <html>
        <head>
            <title>Private Paste - Pastebin</title>
            <style>
              body { font-family: monospace; padding: 40px; background: #0d1117; color: #f0f6fc; text-align: center; }
              h1 { color: #f85149; }
              a { color: #58a6ff; text-decoration: none; }
            </style>
        </head>
        <body>
            <h1>403 - Access Denied</h1>
            <p>This paste is private.</p>
            <a href="/">← Create a new paste</a>
        </body>
        </html>
      `);
    }

    if (models.Paste.increment) {
      await models.Paste.increment('views', { where: { id } });
    } else {
      await models.Paste.findOneAndUpdate({ id }, { $inc: { views: 1 } });
    }

    if (models.Paste.increment) {
      await models.Paste.increment('views', { where: { id } });
    } else {
      await models.Paste.findOneAndUpdate({ id }, { $inc: { views: 1 } });
    }

    res.render('paste', {
      siteTitle: process.env.SITE_TITLE || 'Pastebin',
      paste,
      id
    });
  } catch (error) {
    console.error('View paste error:', error);
    res.status(500).send(`
      <!DOCTYPE html>
      <html>
      <head>
          <title>Server Error - Pastebin</title>
          <style>
            body { font-family: monospace; padding: 40px; background: #0d1117; color: #f0f6fc; text-align: center; }
            h1 { color: #f85149; }
            a { color: #58a6ff; text-decoration: none; }
          </style>
      </head>
      <body>
          <h1>500 - Internal Server Error</h1>
          <p>Something went wrong while loading this paste.</p>
          <a href="/">← Go back</a>
      </body>
      </html>
    `);
  }
});

app.get('/paste/:id', async (req, res) => {
  try {
    const { models } = require('./models');
    const { id } = req.params;
    const now = new Date();

    let paste;
    if (models.Paste.findByPk) {
      paste = await models.Paste.findByPk(id);
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
      return res.status(403).json({ error: 'This paste is private' });
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
      views: paste.views + 1,
      createdAt: paste.createdAt
    });
  } catch (error) {
    console.error('Get paste error:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});


app.use((error, req, res, next) => {
  console.error('Unhandled error:', error);
  
  // Check if this is a web request or API request
  const isApiRequest = req.get('Accept')?.includes('application/json') || req.path.startsWith('/api/');
  
  if (error.type === 'entity.too.large') {
    if (isApiRequest) {
      return res.status(413).json({ error: 'Request entity too large' });
    } else {
      return res.redirect('/?error=File too large');
    }
  }
  
  if (error.type === 'entity.parse.failed') {
    if (isApiRequest) {
      return res.status(400).json({ error: 'Invalid JSON' });
    } else {
      return res.redirect('/?error=Invalid data');
    }
  }

  // Handle multer errors (file upload errors)
  if (error.code === 'LIMIT_FILE_SIZE') {
    if (isApiRequest) {
      return res.status(413).json({ error: 'File too large' });
    } else {
      return res.redirect('/?error=File too large');
    }
  }

  if (isApiRequest) {
    res.status(500).json({ 
      error: 'Internal server error',
      ...(process.env.NODE_ENV === 'development' && { details: error.message })
    });
  } else {
    res.redirect('/?error=Something went wrong. Please try again.');
  }
});

const PORT = process.env.PORT || 3010;

const startServer = async () => {
  try {
    console.log('Connecting to database...');
    await dbManager.connect();
    
    console.log('Initializing models...');
    await initializeModels();
    
    app.listen(PORT, '0.0.0.0', () => {
      console.log(`🚀 Pastebin API server running on port ${PORT}`);
      console.log(`📊 Database: ${dbManager.getType()}`);
      console.log(`🌍 Environment: ${process.env.NODE_ENV || 'development'}`);
      
      if (process.env.NODE_ENV === 'development') {
        const baseUrl = `http://localhost:${PORT}`;
        console.log(`\n🌐 Pastebin URLs:`);
        console.log(`   Web interface: ${baseUrl}/`);
        console.log(`   API docs: ${baseUrl}/api`);
        console.log(`   Health check: ${baseUrl}/healthz`);
        console.log(`\n📡 Ready to use:`);
        console.log(`   Upload text: curl -X POST --data-binary @file.txt ${baseUrl}/create`);
        console.log(`   Upload file: curl -X POST -F "files=@file.txt" ${baseUrl}/create`);
        console.log(`   Upload JSON: curl -H "Content-Type: application/json" -d '{"content":"hello"}' ${baseUrl}/api/v1/create`);
        console.log(`   Get raw: curl ${baseUrl}/raw/{id}`);
        console.log(`\n🔗 URLs auto-adapt when deployed (uses request headers).`);
      }
    });
  } catch (error) {
    console.error('Failed to start server:', error);
    process.exit(1);
  }
};

process.on('SIGINT', async () => {
  console.log('\nShutting down gracefully...');
  await dbManager.disconnect();
  process.exit(0);
});

process.on('SIGTERM', async () => {
  console.log('\nShutting down gracefully...');
  await dbManager.disconnect();
  process.exit(0);
});

if (require.main === module) {
  startServer();
}

module.exports = app;