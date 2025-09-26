const dbManager = require('../config/database');

let models = {};

const getModels = () => {
  return models;
};

const initializeModels = async () => {
  const dbType = dbManager.getType();
  
  if (dbType === 'mongodb') {
    const User = require('./user');
    const Token = require('./token');
    const Paste = require('./paste');
    
    models = { User, Token, Paste };
  } else {
    const connection = dbManager.getConnection();
    
    const User = require('./user').initUserModel(connection);
    const Token = require('./token').initTokenModel(connection);
    const Paste = require('./paste').initPasteModel(connection);
    
    User.hasMany(Token, { foreignKey: 'userId', as: 'tokens' });
    Token.belongsTo(User, { foreignKey: 'userId', as: 'user' });
    
    User.hasMany(Paste, { foreignKey: 'userId', as: 'pastes' });
    Paste.belongsTo(User, { foreignKey: 'userId', as: 'user' });
    
    await connection.sync({ 
      alter: process.env.NODE_ENV === 'development' && process.env.DB_SYNC_ALTER !== 'false',
      force: false 
    });
    
    models = { User, Token, Paste, sequelize: connection };
  }
  
  return models;
};

module.exports = { initializeModels, models, getModels };