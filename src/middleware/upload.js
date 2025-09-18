const multer = require('multer');
const mime = require('mime-types');

const storage = multer.memoryStorage();

const upload = multer({
  storage,
  limits: {
    fileSize: 10 * 1024 * 1024,
    files: 10
  },
  fileFilter: (req, file, cb) => {
    // Accept all files - this is a pastebin, accept any content
    cb(null, true);
  }
});

const detectLanguageFromFile = (filename, mimetype, content) => {
  if (!filename) return 'text';
  
  const ext = filename.split('.').pop()?.toLowerCase();
  
  const langMap = {
    'js': 'javascript',
    'ts': 'typescript',
    'py': 'python',
    'rb': 'ruby',
    'java': 'java',
    'c': 'c',
    'cpp': 'cpp',
    'cc': 'cpp',
    'cxx': 'cpp',
    'h': 'c',
    'hpp': 'cpp',
    'cs': 'csharp',
    'php': 'php',
    'go': 'go',
    'rs': 'rust',
    'sh': 'bash',
    'bash': 'bash',
    'zsh': 'bash',
    'fish': 'bash',
    'ps1': 'powershell',
    'sql': 'sql',
    'html': 'html',
    'htm': 'html',
    'css': 'css',
    'scss': 'scss',
    'sass': 'sass',
    'less': 'less',
    'xml': 'xml',
    'json': 'json',
    'yaml': 'yaml',
    'yml': 'yaml',
    'toml': 'toml',
    'ini': 'ini',
    'cfg': 'ini',
    'conf': 'ini',
    'md': 'markdown',
    'markdown': 'markdown',
    'tex': 'latex',
    'r': 'r',
    'swift': 'swift',
    'kt': 'kotlin',
    'scala': 'scala',
    'clj': 'clojure',
    'hs': 'haskell',
    'ml': 'ocaml',
    'fs': 'fsharp',
    'erl': 'erlang',
    'ex': 'elixir',
    'lua': 'lua',
    'pl': 'perl',
    'vim': 'vim',
    'dockerfile': 'dockerfile'
  };

  return langMap[ext] || 'text';
};

module.exports = {
  upload,
  detectLanguageFromFile
};