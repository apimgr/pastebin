const express = require('express');
const { authenticateToken, optionalAuth } = require('../middleware/auth');

const router = express.Router();

router.get('/', (req, res) => {
  const html = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Register - Pastebin</title>
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
        }
        
        .btn:hover {
            background-color: #2ea043;
        }
        
        .btn:disabled {
            background-color: #30363d;
            color: #8b949e;
            cursor: not-allowed;
        }
        
        .links {
            text-align: center;
            margin-top: 20px;
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
            <div class="logo">📎 Pastebin</div>
            <h1 class="title">Create Account</h1>
            
            <div id="message"></div>
            
            <form id="registerForm">
                <div class="form-group">
                    <label for="username">Username</label>
                    <input type="text" id="username" name="username" required minlength="3" maxlength="30">
                </div>
                
                <div class="form-group">
                    <label for="email">Email</label>
                    <input type="email" id="email" name="email" required>
                </div>
                
                <div class="form-group">
                    <label for="password">Password</label>
                    <input type="password" id="password" name="password" required minlength="6">
                </div>
                
                <button type="submit" class="btn" id="registerBtn">Create Account</button>
            </form>
            
            <div class="links">
                <a href="/login">Already have an account? Sign in</a>
                <a href="/">← Back to Pastebin</a>
            </div>
        </div>
    </div>
    
    <script>
        document.getElementById('registerForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            
            const btn = document.getElementById('registerBtn');
            const messageDiv = document.getElementById('message');
            
            btn.disabled = true;
            btn.textContent = 'Creating account...';
            messageDiv.innerHTML = '';
            
            const formData = {
                username: document.getElementById('username').value,
                email: document.getElementById('email').value,
                password: document.getElementById('password').value
            };
            
            try {
                const response = await fetch('/auth/register', {
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
                    messageDiv.innerHTML = '<div class="success">Account created successfully! Redirecting...</div>';
                    setTimeout(() => {
                        window.location.href = '/dashboard';
                    }, 1500);
                } else {
                    messageDiv.innerHTML = \`<div class="error">\${result.error}</div>\`;
                }
            } catch (error) {
                messageDiv.innerHTML = '<div class="error">Registration failed. Please try again.</div>';
            }
            
            btn.disabled = false;
            btn.textContent = 'Create Account';
        });
    </script>
</body>
</html>`;

  res.send(html);
});

router.get('/login', (req, res) => {
  const html = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Sign In - Pastebin</title>
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
        }
        
        .btn:hover {
            background-color: #2ea043;
        }
        
        .btn:disabled {
            background-color: #30363d;
            color: #8b949e;
            cursor: not-allowed;
        }
        
        .links {
            text-align: center;
            margin-top: 20px;
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
            <div class="logo">📎 Pastebin</div>
            <h1 class="title">Sign In</h1>
            
            <div id="message"></div>
            
            <form id="loginForm">
                <div class="form-group">
                    <label for="username">Username or Email</label>
                    <input type="text" id="username" name="username" required>
                </div>
                
                <div class="form-group">
                    <label for="password">Password</label>
                    <input type="password" id="password" name="password" required>
                </div>
                
                <button type="submit" class="btn" id="loginBtn">Sign In</button>
            </form>
            
            <div class="links">
                <a href="/register">Don't have an account? Sign up</a>
                <a href="/">← Back to Pastebin</a>
            </div>
        </div>
    </div>
    
    <script>
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
                    messageDiv.innerHTML = '<div class="success">Signed in successfully! Redirecting...</div>';
                    setTimeout(() => {
                        window.location.href = '/dashboard';
                    }, 1500);
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

router.get('/dashboard', authenticateToken, async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const userId = req.user.id || req.user._id;
    
    let userTokens = [];
    let userPastes = [];
    
    try {
      if (models.Token.findAll) {
        userTokens = await models.Token.findAll({
          where: { userId, isActive: true },
          attributes: ['id', 'name', 'token', 'createdAt'],
          order: [['createdAt', 'DESC']]
        });
      } else {
        userTokens = await models.Token.find({ userId, isActive: true })
          .select('_id name token createdAt')
          .sort({ createdAt: -1 });
      }
      
      if (models.Paste.findAll) {
        userPastes = await models.Paste.findAll({
          where: { userId },
          attributes: ['id', 'title', 'language', 'isPublic', 'views', 'createdAt'],
          order: [['createdAt', 'DESC']],
          limit: 20
        });
      } else {
        userPastes = await models.Paste.find({ userId })
          .select('id title language isPublic views createdAt')
          .sort({ createdAt: -1 })
          .limit(20);
      }
    } catch (error) {
      console.error('Error fetching user data:', error);
    }

    const html = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Dashboard - Pastebin</title>
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
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        
        .header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 40px;
            padding-bottom: 20px;
            border-bottom: 1px solid #30363d;
        }
        
        .logo {
            font-size: 24px;
            font-weight: 700;
            background: linear-gradient(135deg, #2ea043, #238636);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .user-info {
            display: flex;
            align-items: center;
            gap: 15px;
        }
        
        .nav {
            display: flex;
            gap: 20px;
            margin-bottom: 30px;
        }
        
        .nav a {
            color: #58a6ff;
            text-decoration: none;
            padding: 8px 16px;
            border-radius: 6px;
            transition: background-color 0.2s;
        }
        
        .nav a:hover {
            background-color: #21262d;
        }
        
        .grid {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 30px;
        }
        
        .section {
            background-color: #161b22;
            border: 1px solid #30363d;
            border-radius: 12px;
            padding: 30px;
        }
        
        .section-title {
            font-size: 20px;
            font-weight: 600;
            margin-bottom: 20px;
            color: #f0f6fc;
        }
        
        .btn {
            background-color: #238636;
            color: white;
            border: none;
            padding: 8px 16px;
            border-radius: 6px;
            font-size: 14px;
            cursor: pointer;
            transition: background-color 0.2s;
        }
        
        .btn:hover {
            background-color: #2ea043;
        }
        
        .btn-danger {
            background-color: #f85149;
        }
        
        .btn-danger:hover {
            background-color: #ff6b6b;
        }
        
        .btn-secondary {
            background-color: #21262d;
            border: 1px solid #30363d;
        }
        
        .btn-secondary:hover {
            background-color: #30363d;
        }
        
        .token-item, .paste-item {
            background-color: #21262d;
            border: 1px solid #30363d;
            border-radius: 8px;
            padding: 15px;
            margin-bottom: 15px;
        }
        
        .token-header, .paste-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 8px;
        }
        
        .token-name, .paste-title {
            font-weight: 600;
            color: #f0f6fc;
        }
        
        .token-value {
            font-family: monospace;
            font-size: 12px;
            color: #8b949e;
            word-break: break-all;
            margin: 8px 0;
        }
        
        .meta {
            font-size: 12px;
            color: #8b949e;
        }
        
        .empty-state {
            text-align: center;
            color: #8b949e;
            padding: 40px;
        }
        
        .form-group {
            margin-bottom: 15px;
        }
        
        label {
            display: block;
            margin-bottom: 5px;
            font-weight: 500;
            color: #f0f6fc;
        }
        
        input[type="text"] {
            width: 100%;
            padding: 8px;
            background-color: #0d1117;
            border: 1px solid #30363d;
            border-radius: 4px;
            color: #f0f6fc;
            font-size: 14px;
        }
        
        @media (max-width: 768px) {
            .grid {
                grid-template-columns: 1fr;
            }
            
            .header {
                flex-direction: column;
                gap: 15px;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="logo">📎 Pastebin</div>
            <div class="user-info">
                <span>Welcome, ${req.user.username}!</span>
                <button class="btn btn-danger" onclick="logout()">Logout</button>
            </div>
        </div>
        
        <div class="nav">
            <a href="/">🏠 Home</a>
            <a href="/recent">📋 Recent Pastes</a>
            <a href="/dashboard">👤 Dashboard</a>
            <a href="/api">📚 API</a>
        </div>
        
        <div class="grid">
            <div class="section">
                <div class="section-title">API Tokens</div>
                
                <form id="tokenForm" style="margin-bottom: 20px;">
                    <div class="form-group">
                        <label for="tokenName">Token Name (optional)</label>
                        <input type="text" id="tokenName" name="tokenName" placeholder="My API Token">
                    </div>
                    <button type="submit" class="btn">Create New Token</button>
                </form>
                
                <div id="tokens">
                    ${userTokens.length > 0 ? userTokens.map(token => `
                        <div class="token-item">
                            <div class="token-header">
                                <div class="token-name">${token.name}</div>
                                <button class="btn btn-danger" onclick="deleteToken('${token.id || token._id}')">Delete</button>
                            </div>
                            <div class="token-value">${token.token}</div>
                            <div class="meta">Created: ${new Date(token.createdAt).toLocaleDateString()}</div>
                        </div>
                    `).join('') : '<div class="empty-state">No API tokens yet</div>'}
                </div>
            </div>
            
            <div class="section">
                <div class="section-title">Your Pastes</div>
                
                <div id="pastes">
                    ${userPastes.length > 0 ? userPastes.map(paste => `
                        <div class="paste-item">
                            <div class="paste-header">
                                <div class="paste-title">
                                    <a href="/${paste.id}" style="color: #f0f6fc; text-decoration: none;">${paste.title || 'Untitled'}</a>
                                </div>
                                <div>
                                    <a href="/${paste.id}" class="btn btn-secondary" style="margin-right: 5px;">View</a>
                                    <button class="btn btn-danger" onclick="deletePaste('${paste.id}')">Delete</button>
                                </div>
                            </div>
                            <div class="meta">
                                ${paste.language || 'text'} • 
                                ${paste.isPublic ? 'Public' : 'Private'} • 
                                ${paste.views || 0} views • 
                                ${new Date(paste.createdAt).toLocaleDateString()}
                            </div>
                        </div>
                    `).join('') : '<div class="empty-state">No pastes yet</div>'}
                </div>
            </div>
        </div>
    </div>
    
    <script>
        const token = localStorage.getItem('token');
        
        if (!token) {
            window.location.href = '/login';
        }
        
        document.getElementById('tokenForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            
            const tokenName = document.getElementById('tokenName').value || 'Default Token';
            
            try {
                const response = await fetch('/auth/tokens', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': \`Bearer \${token}\`
                    },
                    body: JSON.stringify({ name: tokenName })
                });
                
                const result = await response.json();
                
                if (response.ok) {
                    location.reload();
                } else {
                    alert('Error: ' + result.error);
                }
            } catch (error) {
                alert('Failed to create token');
            }
        });
        
        async function deleteToken(tokenId) {
            if (!confirm('Are you sure you want to delete this token?')) return;
            
            try {
                const response = await fetch(\`/auth/tokens/\${tokenId}\`, {
                    method: 'DELETE',
                    headers: {
                        'Authorization': \`Bearer \${token}\`
                    }
                });
                
                if (response.ok) {
                    location.reload();
                } else {
                    alert('Failed to delete token');
                }
            } catch (error) {
                alert('Failed to delete token');
            }
        }
        
        async function deletePaste(pasteId) {
            if (!confirm('Are you sure you want to delete this paste?')) return;
            
            try {
                const response = await fetch(\`/pastes/\${pasteId}\`, {
                    method: 'DELETE',
                    headers: {
                        'Authorization': \`Bearer \${token}\`
                    }
                });
                
                if (response.ok) {
                    location.reload();
                } else {
                    alert('Failed to delete paste');
                }
            } catch (error) {
                alert('Failed to delete paste');
            }
        }
        
        function logout() {
            localStorage.removeItem('token');
            localStorage.removeItem('user');
            window.location.href = '/';
        }
    </script>
</body>
</html>`;

    res.send(html);
  } catch (error) {
    console.error('Dashboard error:', error);
    res.status(500).send('Internal server error');
  }
});

module.exports = router;