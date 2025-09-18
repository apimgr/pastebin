const express = require('express');

const router = express.Router();

// Dashboard - check session auth and show proper dashboard
router.get('/', (req, res) => {
  // Check if user is authenticated via session
  if (!req.session.user || !req.session.token) {
    return res.redirect('/auth/login');
  }
  res.send(`
<!DOCTYPE html>
<html>
<head>
    <title>Dashboard - ${process.env.SITE_TITLE || 'Pastebin'}</title>
    <style>
        body { font-family: monospace; background: #0d1117; color: #f0f6fc; padding: 20px; text-align: center; }
        .loading { margin-top: 50px; font-size: 18px; color: #8b949e; }
    </style>
</head>
<body>
    <div class="loading">Loading your dashboard...</div>
    <script>
        const token = localStorage.getItem('token');
        const user = JSON.parse(localStorage.getItem('user') || '{}');
        
        if (!token || !user.id) {
            window.location.href = '/auth/login';
        } else {
            // Verify token is still valid
            fetch('/auth/me', {
                headers: { 'Authorization': 'Bearer ' + token }
            }).then(response => {
                if (response.ok) {
                    return response.json();
                } else {
                    localStorage.removeItem('token');
                    localStorage.removeItem('user');
                    window.location.href = '/auth/login';
                }
            }).then(userData => {
                if (userData) {
                    // Load actual dashboard with user data
                    loadDashboard(userData.user, token);
                }
            }).catch(() => {
                localStorage.removeItem('token');
                localStorage.removeItem('user');
                window.location.href = '/auth/login';
            });
        }
        
        async function loadDashboard(user, token) {
            try {
                // Show a functional dashboard with instructions
                const tokensData = { tokens: [] };
                const pastesData = { pastes: [] };
                
                // Render dashboard HTML
                document.body.innerHTML = \`
<!DOCTYPE html>
<html>
<head>
    <title>Dashboard - ${process.env.SITE_TITLE || 'Pastebin'}</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #0d1117; color: #f0f6fc; margin: 0; padding: 20px; }
        .container { max-width: 1200px; margin: 0 auto; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 40px; padding-bottom: 20px; border-bottom: 1px solid #30363d; }
        .logo { font-size: 24px; font-weight: 700; background: linear-gradient(135deg, #2ea043, #238636); -webkit-background-clip: text; -webkit-text-fill-color: transparent; }
        .nav { display: flex; gap: 20px; margin-bottom: 30px; justify-content: center; }
        .nav a { color: #58a6ff; text-decoration: none; padding: 8px 16px; border-radius: 6px; }
        .nav a:hover { background-color: #21262d; }
        .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 30px; }
        .card { background-color: #161b22; border: 1px solid #30363d; border-radius: 12px; padding: 30px; }
        .btn { background-color: #238636; color: white; border: none; padding: 8px 16px; border-radius: 6px; cursor: pointer; }
        .btn-danger { background-color: #f85149; }
        .token-item, .paste-item { background-color: #21262d; border: 1px solid #30363d; border-radius: 8px; padding: 15px; margin-bottom: 15px; }
        .form-group { margin-bottom: 15px; }
        input[type="text"] { width: 100%; padding: 8px; background-color: #0d1117; border: 1px solid #30363d; border-radius: 4px; color: #f0f6fc; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="logo">📎 ${process.env.SITE_TITLE || 'Pastebin'}</div>
            <div>
                <span>Welcome, \${user.username}!</span>
                <button onclick="logout()" class="btn btn-danger" style="margin-left: 15px;">Logout</button>
            </div>
        </div>
        
        <div class="nav">
            <a href="/">🏠 Home</a>
            <a href="/recent">📋 Recent Pastes</a>
            <a href="/auth/dashboard">👤 Dashboard</a>
        </div>
        
        <div class="grid">
            <div class="card">
                <h3>API Token Management</h3>
                <div style="text-align: center; color: #8b949e; padding: 20px;">
                    <p>To manage API tokens, use the REST API:</p>
                    <div style="background: #0d1117; border-radius: 6px; padding: 15px; margin: 15px 0; font-family: monospace; font-size: 13px; text-align: left;">
                        <div style="margin: 5px 0;">curl -H "Authorization: Bearer YOUR_TOKEN" ${req.protocol || 'http'}://${req.get ? req.get('host') : 'localhost:3010'}/auth/tokens</div>
                        <div style="margin: 5px 0;">curl -X POST -H "Authorization: Bearer YOUR_TOKEN" -d '{"name":"My Token"}' ${req.protocol || 'http'}://${req.get ? req.get('host') : 'localhost:3010'}/auth/tokens</div>
                    </div>
                    <p style="font-size: 12px;">Your token: \${token.substring(0, 20)}...</p>
                </div>
            </div>
            
            <div class="card">
                <h3>Your Pastes</h3>
                <div>
                    \${pastesData.pastes.map(paste => \`
                        <div class="paste-item">
                            <div style="display: flex; justify-content: space-between; margin-bottom: 8px;">
                                <a href="/\${paste.id}" style="color: #f0f6fc; text-decoration: none; font-weight: 600;">\${paste.title || 'Untitled'}</a>
                                <div>
                                    <a href="/\${paste.id}" style="background-color: #21262d; border: 1px solid #30363d; color: #f0f6fc; padding: 4px 8px; border-radius: 4px; text-decoration: none; font-size: 12px; margin-right: 5px;">View</a>
                                    <button onclick="deletePaste('\${paste.id}')" class="btn btn-danger" style="padding: 4px 8px; font-size: 12px;">Delete</button>
                                </div>
                            </div>
                            <div style="font-size: 12px; color: #8b949e;">
                                \${paste.language || 'text'} • \${paste.isPublic ? 'Public' : 'Private'} • \${paste.views || 0} views • \${new Date(paste.createdAt).toLocaleDateString()}
                            </div>
                        </div>
                    \`).join('') || '<div style="text-align: center; color: #8b949e; padding: 40px;"><p>No pastes yet</p><a href="/" style="color: #58a6ff;">Create your first paste →</a></div>'}
                </div>
            </div>
        </div>
        
        <div style="text-align: center; padding: 20px 0; border-top: 1px solid #30363d; color: #8b949e; margin-top: 40px;">
            <p>Powered by Node.js & Express • <a href="/recent" style="color: #58a6ff; text-decoration: none;">View Recent Pastes</a> • <a href="/api" style="color: #58a6ff; text-decoration: none;">API Documentation</a></p>
        </div>
    </div>
    
    <script>
        async function createToken() {
            const tokenName = document.getElementById('tokenName').value || 'Default Token';
            
            try {
                const response = await fetch('/auth/tokens', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + token
                    },
                    body: JSON.stringify({ name: tokenName })
                });
                
                if (response.ok) {
                    location.reload();
                } else {
                    alert('Failed to create token');
                }
            } catch (error) {
                alert('Failed to create token');
            }
        }
        
        async function deleteToken(tokenId) {
            if (!confirm('Delete this token?')) return;
            
            try {
                const response = await fetch(\`/auth/tokens/\${tokenId}\`, {
                    method: 'DELETE',
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                
                if (response.ok) location.reload();
            } catch (error) {
                alert('Failed to delete token');
            }
        }
        
        async function deletePaste(pasteId) {
            if (!confirm('Delete this paste?')) return;
            
            try {
                const response = await fetch(\`/pastes/\${pasteId}\`, {
                    method: 'DELETE',
                    headers: { 'Authorization': 'Bearer ' + token }
                });
                
                if (response.ok) location.reload();
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
</html>
                \`;
            } catch (error) {
                console.error('Dashboard load error:', error);
                document.body.innerHTML = '<div style="padding: 40px; text-align: center;"><h3>Error loading dashboard</h3><p><a href="/auth/login" style="color: #58a6ff;">Please login again</a></p></div>';
            }
        }
    </script>
</body>
</html>`);
});

// Handle logout  
router.get('/logout', (req, res) => {
  req.session.destroy((err) => {
    res.redirect('/?message=Logged out successfully');
  });
});

module.exports = router;