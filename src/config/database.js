const { Sequelize } = require('sequelize');
const mongoose = require('mongoose');
require('dotenv').config();

class DatabaseManager {
  constructor() {
    this.sequelize = null;
    this.mongoose = null;
    this.dbType = process.env.DB_TYPE || 'sqlite';
  }

  async connect() {
    if (this.dbType === 'mongodb') {
      return this.connectMongoDB();
    } else {
      return this.connectSQL();
    }
  }

  async connectMongoDB() {
    try {
      const mongoUri = process.env.MONGO_URI || 'mongodb://localhost:27017/pastebin';
      await mongoose.connect(mongoUri);
      console.log('Connected to MongoDB');
      this.mongoose = mongoose;
      return { type: 'mongodb', connection: mongoose };
    } catch (error) {
      console.error('MongoDB connection failed:', error);
      throw error;
    }
  }

  async connectSQL() {
    try {
      let sequelizeConfig;

      switch (this.dbType) {
        case 'postgresql':
        case 'postgres':
          sequelizeConfig = {
            dialect: 'postgres',
            host: process.env.DB_HOST || 'localhost',
            port: process.env.DB_PORT || 5432,
            database: process.env.DB_NAME || 'pastebin',
            username: process.env.DB_USER,
            password: process.env.DB_PASSWORD,
            logging: process.env.NODE_ENV === 'development' ? console.log : false,
          };
          break;

        case 'mariadb':
        case 'mysql':
          sequelizeConfig = {
            dialect: 'mysql',
            host: process.env.DB_HOST || 'localhost',
            port: process.env.DB_PORT || 3306,
            database: process.env.DB_NAME || 'pastebin',
            username: process.env.DB_USER,
            password: process.env.DB_PASSWORD,
            logging: process.env.NODE_ENV === 'development' ? console.log : false,
          };
          break;

        case 'sqlite':
        default:
          sequelizeConfig = {
            dialect: 'sqlite',
            storage: process.env.DB_PATH || './data/pastebin.db',
            logging: process.env.NODE_ENV === 'development' ? console.log : false,
          };
          break;
      }

      this.sequelize = new Sequelize(sequelizeConfig);
      
      await this.sequelize.authenticate();
      console.log(`Connected to ${this.dbType} database`);
      
      return { type: 'sql', connection: this.sequelize };
    } catch (error) {
      console.error('SQL database connection failed:', error);
      throw error;
    }
  }

  async disconnect() {
    if (this.sequelize) {
      await this.sequelize.close();
    }
    if (this.mongoose) {
      await mongoose.disconnect();
    }
  }

  getConnection() {
    return this.dbType === 'mongodb' ? this.mongoose : this.sequelize;
  }

  getType() {
    return this.dbType;
  }
}

module.exports = new DatabaseManager();