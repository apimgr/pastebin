const getBaseUrl = (req) => {
  const protocol = req.protocol || 'http';
  const host = req.get('host') || 'localhost:3010';
  
  return `${protocol}://${host}`;
};

const generatePasteUrl = (req, pasteId) => {
  const baseUrl = getBaseUrl(req);
  return `${baseUrl}/${pasteId}`;
};

module.exports = {
  getBaseUrl,
  generatePasteUrl
};