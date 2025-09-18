const crypto = require('crypto');

const generatePasteId = () => {
  const characters = 'abcdefghijklmnopqrstuvwxyz0123456789';
  const length = 6;
  let result = '';
  
  for (let i = 0; i < length; i++) {
    const randomIndex = crypto.randomInt(0, characters.length);
    result += characters[randomIndex];
  }
  
  return result;
};

module.exports = { generatePasteId };