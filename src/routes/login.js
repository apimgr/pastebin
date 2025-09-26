const express = require('express');

const router = express.Router();

router.get('/', (req, res) => {
  res.render('login', {
    siteTitle: process.env.SITE_TITLE || 'Pastebin',
    error: req.query.error || null
  });
});

router.get('/old', (req, res) => {
  const html = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Sign In - ${process.env.SITE_TITLE || 'Pastebin'}</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', sans-serif;
            background-color: #0d1117;
            color: #f0f6fc;
            line-height: 1.6;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        
        .container {
            max-width: 400px;
            width: 100%;
            padding: 20px;
        }
        
        .card {
            background-color: #161b22;
            border: 1px solid #30363d;
            border-radius: 12px;
            padding: 40px;
        }
        
        .logo {
            text-align: center;
            font-size: 36px;
            font-weight: 700;
            margin-bottom: 10px;
            background: linear-gradient(135deg, #2ea043, #238636);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .title {
            text-align: center;
            font-size: 24px;
            font-weight: 600;
            margin-bottom: 30px;
            color: #f0f6fc;
        }
        
        .form-group {
            margin-bottom: 20px;
        }
        
        label {
            display: block;
            margin-bottom: 8px;
            font-weight: 500;
            color: #f0f6fc;
        }
        
        input[type="text"], input[type="email"], input[type="password"] {
            width: 100%;
            padding: 12px;
            background-color: #21262d;
            border: 1px solid #30363d;
            border-radius: 6px;
            color: #f0f6fc;
            font-size: 14px;
        }
        
        input:focus {
            outline: none;
            border-color: #0969da;
            box-shadow: 0 0 0 3px rgba(9, 105, 218, 0.3);
        }
        
        .btn {
            width: 100%;
            background-color: #238636;
            color: white;
            border: none;
            padding: 12px;
            border-radius: 6px;
            font-size: 16px;
            font-weight: 500;
            cursor: pointer;
            transition: background-color 0.2s;
            margin-bottom: 15px;
        }
        
        .btn:hover {
            background-color: #2ea043;
        }
        
        .btn:disabled {
            background-color: #30363d;
            color: #8b949e;
            cursor: not-allowed;
        }
        
        .btn-secondary {
            background-color: #21262d;
            border: 1px solid #30363d;
        }
        
        .btn-secondary:hover {
            background-color: #30363d;
        }
        
        .links {
            text-align: center;
            font-size: 14px;
        }
        
        .links a {
            color: #58a6ff;
            text-decoration: none;
            margin: 0 10px;
        }
        
        .links a:hover {
            text-decoration: underline;
        }
        
        .error {
            background-color: #f85149;
            color: white;
            padding: 12px;
            border-radius: 6px;
            margin-bottom: 20px;
            font-size: 14px;
        }
        
        .success {
            background-color: #238636;
            color: white;
            padding: 12px;
            border-radius: 6px;
            margin-bottom: 20px;
            font-size: 14px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="card">
            <div class="logo">📎 ${process.env.SITE_TITLE || 'Pastebin'}</div>
            <h1 class="title">Sign In</h1>
            
            <div id="message"></div>
            
            <form id="loginForm" method="POST" action="/auth/login">
                <div class="form-group">
                    <label for="username">Username or Email</label>
                    <input type="text" id="username" name="username" required>
                </div>
                
                <div class="form-group">
                    <label for="password">Password</label>
                    <input type="password" id="password" name="password" required>
                </div>
                
                <button type="submit" class="btn" id="loginBtn">Sign In</button>
                <a href="/auth/register" class="btn btn-secondary" style="text-decoration: none; text-align: center; display: block;">Create Account</a>
            </form>
            
            <div class="links">
                <a href="/">← Back to ${process.env.SITE_TITLE || 'Pastebin'}</a>
            </div>
        </div>
    </div>
    
    <script>
        // Check if already logged in
        if (localStorage.getItem('token')) {
            window.location.href = '/auth/dashboard';
        }
        
        document.getElementById('loginForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            
            const btn = document.getElementById('loginBtn');
            const messageDiv = document.getElementById('message');
            
            btn.disabled = true;
            btn.textContent = 'Signing in...';
            messageDiv.innerHTML = '';
            
            const formData = {
                username: document.getElementById('username').value,
                password: document.getElementById('password').value
            };
            
            try {
                const response = await fetch('/auth/login', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(formData)
                });
                
                const result = await response.json();
                
                if (response.ok) {
                    localStorage.setItem('token', result.token);
                    localStorage.setItem('user', JSON.stringify(result.user));
                    window.location.href = '/auth/dashboard';
                } else {
                    messageDiv.innerHTML = \`<div class="error">\${result.error}</div>\`;
                }
            } catch (error) {
                messageDiv.innerHTML = '<div class="error">Login failed. Please try again.</div>';
            }
            
            btn.disabled = false;
            btn.textContent = 'Sign In';
        });
    </script>
</body>
</html>`;

  res.send(html);
});

router.post('/', async (req, res) => {
  try {
    const { username, password } = req.body;

    if (!username || !password) {
      const isApiRequest = req.get('Content-Type')?.includes('application/json');
      if (isApiRequest) {
        return res.status(400).json({ error: 'Username and password are required' });
      } else {
        return res.render('login', { 
          siteTitle: process.env.SITE_TITLE || 'Pastebin',
          error: 'Username and password are required'
        });
      }
    }

    const { getModels } = require('../models');
    const models = getModels();

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
      const isApiRequest = req.get('Content-Type')?.includes('application/json');
      if (isApiRequest) {
        return res.status(401).json({ error: 'Invalid credentials' });
      } else {
        return res.render('login', { 
          siteTitle: process.env.SITE_TITLE || 'Pastebin',
          error: 'Invalid credentials'
        });
      }
    }

    const jwt = require('jsonwebtoken');
    const token = jwt.sign(
      { userId: user.id || user._id },
      process.env.JWT_SECRET,
      { expiresIn: process.env.JWT_EXPIRES_IN || '7d' }
    );

    const isApiRequest = req.get('Content-Type')?.includes('application/json');
    
    if (isApiRequest) {
      res.json({
        message: 'Login successful',
        user: {
          id: user.id || user._id,
          username: user.username,
          email: user.email
        },
        token
      });
    } else {
      // Web form - store user in session and redirect to dashboard
      req.session.user = {
        id: user.id || user._id,
        username: user.username,
        email: user.email
      };
      req.session.token = token;
      res.redirect('/auth/dashboard');
    }
  } catch (error) {
    console.error('Login error:', error);
    const isApiRequest = req.get('Content-Type')?.includes('application/json');
    if (isApiRequest) {
      res.status(500).json({ error: 'Internal server error' });
    } else {
      res.render('login', { 
        siteTitle: process.env.SITE_TITLE || 'Pastebin',
        error: 'Login failed. Please try again.'
      });
    }
  }
});

module.exports = router;